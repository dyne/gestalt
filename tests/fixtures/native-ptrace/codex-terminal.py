"""Actual pinned Codex TUI through a fresh PTY and the test-only tracer."""
import os,pty,termios,fcntl,struct,subprocess,pathlib,json,select,time,re,datetime,hashlib,sys
prototype,binary,home,work,directory=sys.argv[1:];out=pathlib.Path(directory);out.mkdir(parents=True,exist_ok=True)
master,slave=pty.openpty();fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',40,110,0,0));control_r,control_w=os.pipe()
command=[prototype,'--terminal',str(slave),'--control',str(control_r),'--',binary,'--no-alt-screen','--sandbox','read-only','--ask-for-approval','never','-c','check_for_update_on_startup=false']
environment={**os.environ,'CODEX_HOME':home,'TERM':'xterm-256color'}
start=datetime.datetime.now(datetime.timezone.utc).isoformat();child=subprocess.Popen(command,cwd=work,env=environment,stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=subprocess.PIPE,pass_fds=(slave,control_r));os.close(slave);os.close(control_r)
data=b'';diagnostics=b'';interactive=False;queries=0;tail=b'';deadline=time.monotonic()+20;failure=None
try:
    while time.monotonic()<deadline and child.poll() is None:
        for ready in select.select([master,child.stderr],[],[],.1)[0]:
            try:chunk=os.read(ready if isinstance(ready,int) else ready.fileno(),32768)
            except OSError as e:
                if e.errno==5:continue
                raise
            if ready==master:
                data+=chunk;query=tail+chunk
                for _ in range(query.count(b'\x1b[6n')):
                    assert queries<8;os.write(master,b'\x1b[1;1R');queries+=1
                tail=query[-3:]
            else:diagnostics+=chunk
        visible=re.sub(rb'\x1b\[[0-?]*[ -/]*[@-~]',b'',data)
        if b'Sign in with ChatGPT' in visible and (b'Device Code' in visible or b'OpenAI API key' in visible):interactive=True;break
    assert interactive,'actual interactive sign-in choices not reached'
    os.write(master,b'\x03')
except Exception as e:failure=repr(e)
finally:
    if child.poll() is None:
        try:os.write(control_w,b'S')
        except BrokenPipeError:pass
    try:child.wait(timeout=8)
    except subprocess.TimeoutExpired:child.kill();child.wait(timeout=5);failure=failure or 'tracer did not settle'
    try:
        while select.select([master],[],[],0)[0]:
            chunk=os.read(master,32768)
            if not chunk:break
            data+=chunk
    except OSError as e:
        if e.errno!=5:failure=failure or repr(e)
    diagnostics+=child.stderr.read();os.close(master);os.close(control_w)
(out/'output.bin').write_bytes(data);(out/'tracer.stderr').write_bytes(diagnostics)
record={'command':command,'cwd':work,'startedAt':start,'observedAt':datetime.datetime.now(datetime.timezone.utc).isoformat(),'helperExit':child.returncode,'interactiveSignInChoices':interactive,'cursorReplies':queries,'failure':failure,'modelRequest':False,'outputSha256':hashlib.sha256(data).hexdigest(),'tracerSha256':hashlib.sha256(diagnostics).hexdigest(),'scope':'test-only PTY/ptrace; no production backend selection'}
(out/'result.json').write_text(json.dumps(record,indent=2)+'\n');print(json.dumps(record));assert failure is None and child.returncode==0 and b'TRACE_DRAINED' in diagnostics
