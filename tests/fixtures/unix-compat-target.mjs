import { execFileSync } from 'node:child_process';
import { readFileSync, readlinkSync, statSync } from 'node:fs';
const [peer, mode, ...argv] = process.argv.slice(2);
const identity = JSON.parse(execFileSync(peer,['--identity',String(process.pid)],{encoding:'utf8'}));
const observation = { identity, pid:process.pid,ppid:process.ppid,uid:process.getuid(),gid:process.getgid(),
  rootOwner:statSync('/').uid,tmpOwner:statSync('/tmp').uid,argv,cwd:process.cwd(),
  environment:Object.fromEntries(['GESTALT_SESSION_TMP_CONTEXT','TMPDIR','TMP','TEMP'].map(k=>[k,process.env[k]??null])),
  tty:[Boolean(process.stdin.isTTY),Boolean(process.stdout.isTTY),Boolean(process.stderr.isTTY)],ttySize:{columns:process.stdout.columns,rows:process.stdout.rows} };
if(process.platform==='linux') observation.namespaces={user:readlinkSync('/proc/self/ns/user'),pid:readlinkSync('/proc/self/ns/pid'),uidMap:readFileSync('/proc/self/uid_map','utf8'),gidMap:readFileSync('/proc/self/gid_map','utf8')};
if(mode==='tty') {console.log('ACTUAL_TTY:'+JSON.stringify(observation));process.exitCode=23;}
else if(mode==='hold') {
  process.on('SIGTERM',()=>{console.log('signal-observed');process.exit(19)});
  console.log(JSON.stringify(observation));setInterval(()=>{},1000);
} else {
  const input=[];for await(const b of process.stdin)input.push(b);observation.inputHex=Buffer.concat(input).toString('hex');
  process.stdout.write(JSON.stringify(observation)+'\n');process.stderr.write(Buffer.from([255,0,13]));process.exitCode=23;
}
