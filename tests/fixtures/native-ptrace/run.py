"""Test-only Linux prototype. No production selection, policy changes or PID scans."""
import os,sys,json,pathlib,subprocess,datetime,hashlib,select,signal,time,platform,traceback

source=pathlib.Path(__file__).resolve().parent
output=pathlib.Path(sys.argv[1]).resolve();output.mkdir(parents=True,exist_ok=True)
rows=[];builds=[]
def stamp():return datetime.datetime.now(datetime.timezone.utc).isoformat()
def digest(file):
    b=file.read_bytes();return {'path':str(file),'bytes':len(b),'mode':oct(file.stat().st_mode&0o7777),'sha256':hashlib.sha256(b).hexdigest()}
def run_build(name):
    argv=[os.environ.get('CC','cc'),'-std=c11','-O2','-Wall','-Wextra','-Werror','-pthread',str(source/(name+'.c')),'-o',str(output/name)]
    start=stamp();r=subprocess.run(argv,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=30)
    log=output/(name+'-build.log');log.write_bytes(r.stdout);builds.append({'argv':argv,'cwd':os.getcwd(),'startedAt':start,'observedAt':stamp(),'exit':r.returncode,'log':digest(log)})
    if r.returncode:raise RuntimeError(f'build {name} failed: {r.stdout.decode()}')
def settled(fd,timeout=8):
    return bool(select.select([fd],[],[],timeout)[0])

sentinel=subprocess.Popen(['/bin/sleep','120'],stdin=subprocess.DEVNULL,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
sentinel_fd=os.pidfd_open(sentinel.pid)
try:
    for name in ['prototype','fixture-base','fixture-race','fixture-abi']:run_build(name)
    cases=[('attachment-marker','normal'),('probes','normal'),('thread-exec','normal'),('tree','stop'),('tree','eof'),('tree','tracer-kill'),('clone-parent','stop'),('fork-race','tracer-kill'),('compat','normal'),('nnp','normal'),('group-stop','normal'),('kernel-denial','normal')]
    for mode,action in cases:
        start=stamp();child=None;fds=[];data=b'';error=b'';release_r,release_w=os.pipe();token_r,token_w=os.pipe();held=False;token_eof=None;state=None;communicated=False
        env={**os.environ,'GESTALT_PTRACE_FIXTURE_RELEASE_FD':str(release_r)}
        fixture='fixture-race' if mode in ['clone-parent','fork-race'] else 'fixture-abi' if mode in ['attachment-marker','compat','nnp','group-stop','kernel-denial'] else 'fixture-base'
        command=[str(output/'prototype'),str(output/fixture),mode]
        if fixture=='fixture-race':command.append(str(token_w))
        marker=output/'must-not-start'
        if mode=='attachment-marker':command=[str(output/'prototype'),str(output/'fixture-abi'),'marker',str(output/'attached-payload-marker')]
        if mode=='kernel-denial':command=[str(output/'fixture-abi'),'deny-tracing',str(output/'prototype'),str(output/'fixture-abi'),'marker',str(marker)]
        row={'mode':mode,'action':action,'command':command,'cwd':os.getcwd(),'startedAt':start,'payloadExitOrSignal':None}
        try:
            child=subprocess.Popen(command,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,env=env,pass_fds=(release_r,token_w));os.close(release_r);release_r=None;os.close(token_w);token_w=None
            needs_tree=mode in ['tree','clone-parent','fork-race'];needs_stop=mode=='group-stop';deadline=time.monotonic()+10
            while needs_tree or needs_stop:
                if time.monotonic()>deadline:raise RuntimeError('actual held precondition not reached')
                for stream in select.select([child.stdout,child.stderr],[],[],.1)[0]:
                    block=os.read(stream.fileno(),32768)
                    if stream==child.stdout:data+=block
                    else:error+=block
                ready=(error.count(b'TRACE_CHILD')>=12) if mode=='fork-race' else (b'"ready"' in data) if needs_tree else b'LISTEN_READY' in error
                if ready:break
                if child.poll() is not None:raise RuntimeError('helper ended before held precondition')
            if needs_tree:
                events=[]
                for line in data.splitlines():
                    try:events.append(json.loads(line))
                    except json.JSONDecodeError:pass
                pids=[event['pid'] for event in events if 'pid' in event]
                expected=3 if mode=='tree' else 2 if mode=='clone-parent' else 2
                assert len(pids)>=expected,('distinct held process identities',pids)
                assert len(set(pids))==len(pids)
                for pid in pids:fds.append(os.pidfd_open(pid))
                assert not select.select(fds,[],[],0)[0],'fixtures already exited before action'
                held=True
                if action=='stop':child.stdin.write(b'S');child.stdin.flush()
                elif action=='eof':child.stdin.close();child.stdin=None
                else:child.kill()
            elif needs_stop:
                pid=int(data.splitlines()[0].split()[1]);fds.append(os.pidfd_open(pid));assert not settled(fds[0],0)
                state=pathlib.Path(f'/proc/{pid}/status').read_text();held=True;signal.pidfd_send_signal(fds[0],signal.SIGCONT)
            # Keep control open until ordinary payloads exit; communicate would
            # otherwise request an early EOF stop and hide their result.
            if not needs_tree:child.wait(timeout=10)
            tail,diagnostics=child.communicate(timeout=10);data+=tail;error+=diagnostics;communicated=True
            if mode=='kernel-denial':
                assert child.returncode==77 and b'PREREQUISITE_REFUSED errno=1' in error
                assert not marker.exists() and not data,'payload started despite real kernel ptrace denial'
            else:assert child.returncode==(-9 if action=='tracer-kill' else 0),error.decode()
            if needs_tree:
                for fd in fds:assert settled(fd),'captured pidfd still active'
                if fixture=='fixture-race':
                    assert settled(token_r),'inherited token writers remain';assert os.read(token_r,1)==b'';token_eof=True
            if mode=='probes':
                actual=[json.loads(line) for line in data.splitlines()]
                assert actual[0]=={'untraced':-1,'untracedErrno':1,'clone3':-1,'clone3Errno':38,'x32':-1,'x32Errno':38}
                assert actual[1]=={'nestedTracer':-1,'errno':1};assert b'root_status=5888' in error
            if mode=='attachment-marker':
                assert (output/'attached-payload-marker').read_text()=='PAYLOAD_STARTED'
                assert b'PAYLOAD_STARTED nnp=1' in data and f'TracerPid:\t{child.pid}'.encode() in data
                assert b'root_status=5888' in error
            if mode=='thread-exec':assert b'root_status=0' in error
            if mode=='compat':assert b'root_status=31' in error and b'COMPAT_ESCAPED' not in data
            if mode=='nnp':assert b'inherited=1 remove=-1' in data and b'root_status=5888' in error
            if mode=='group-stop':assert b'LISTEN_READY' in error and b'RESUMED' in data and b'root_status=5888' in error
            assert not settled(sentinel_fd,0),'non-owned sentinel was disturbed'
            row.update(status='passed',helperExit=child.returncode,heldPrecondition=held,observedPidfds=len(fds),exitedPidfds=sum(settled(fd,0) for fd in fds),inheritedTokenEof=token_eof,heldState=state)
        except Exception as e:
            row.update(status='failed',failure=str(e),stack=traceback.format_exc())
        finally:
            # Controlled voluntary teardown remains available even if a fixture
            # escapes or a test assertion fails. It is released after observations.
            os.close(release_w)
            if child is not None:
                if child.poll() is None:child.kill()
                try:
                    if not communicated:
                        tail,diagnostics=child.communicate(timeout=8);data+=tail;error+=diagnostics
                except Exception as e:row['cleanupFailure']=str(e)
            for fd in fds:
                try:signal.pidfd_send_signal(fd,signal.SIGKILL)
                except ProcessLookupError:pass
                os.close(fd)
            for fd in [release_r,token_r,token_w]:
                if fd is not None:os.close(fd)
            for suffix,b in [('stdout',data),('stderr',error)]:
                log=output/f'{mode}-{action}.{suffix}';log.write_bytes(b);row[suffix]=digest(log)
            row['observedAt']=stamp();rows.append(row)
finally:
    sentinel_untouched=not settled(sentinel_fd,0)
    signal.pidfd_send_signal(sentinel_fd,signal.SIGKILL);sentinel.wait(timeout=5);os.close(sentinel_fd)
    version=subprocess.run([os.environ.get('CC','cc'),'--version'],stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=5)
    refs=[digest(f) for f in sorted(source.iterdir()) if f.is_file()]+[digest(output/name) for name in ['prototype','fixture-base','fixture-race','fixture-abi'] if (output/name).exists()]
    result={'testOnly':True,'productionBackendSelected':False,'runtime':{'kernel':platform.release(),'machine':platform.machine(),'uid':os.getuid(),'gid':os.getgid(),'node':os.environ.get('NODE_VERSION'),'compiler':version.stdout.decode(),'uidMap':pathlib.Path('/proc/self/uid_map').read_text(),'gidMap':pathlib.Path('/proc/self/gid_map').read_text()},'builds':builds,'cases':rows,'counts':{'passed':sum(r['status']=='passed' for r in rows),'failed':sum(r['status']=='failed' for r in rows),'total':len(rows)},'nonOwnedSentinelUntouched':sentinel_untouched,'references':refs,'observedAt':stamp()}
    (output/'prototype-results.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps(result['counts']))
assert len(rows)==12 and result['counts']['failed']==0 and sentinel_untouched
