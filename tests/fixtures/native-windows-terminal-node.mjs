import { writeFileSync } from 'node:fs';
const [report, mode] = process.argv.slice(2), data = {
  kind: 'node-actual-stdio', pid: process.pid, node: process.version,
  tty: [Boolean(process.stdin.isTTY), Boolean(process.stdout.isTTY), Boolean(process.stderr.isTTY)],
  initialSize: { columns: process.stdout.columns, rows: process.stdout.rows }, phase: 'initial',
};
const save = () => writeFileSync(report, JSON.stringify(data));
process.on('uncaughtException', error => { data.error = error.stack; save(); process.exit(32); });
save();
if (!data.tty.every(Boolean)) { process.stderr.write('NODE_ACTUAL_TTY_FAILURE'); process.exit(31); }
process.stdin.setRawMode(true); process.stdin.resume();
data.phase = 'ready'; save();
process.stdout.write('NODE_STDOUT_READY\n'); process.stderr.write('NODE_STDERR_READY\n');
if (mode === 'hold') setInterval(() => {}, 1000);
else {
  let bytes = Buffer.alloc(0);
  const finish = () => {
    if (bytes.length < 9 || process.stdout.columns !== 103 || process.stdout.rows !== 41) return;
    data.phase = 'input-complete'; data.inputHex = bytes.toString('hex'); data.resized = { columns: process.stdout.columns, rows: process.stdout.rows }; save();
    process.exit(bytes.equals(Buffer.from('é東京\x04')) ? 23 : 33);
  };
  process.stdout.on('resize', finish); process.stdin.on('data', chunk => { bytes = Buffer.concat([bytes, chunk]); finish(); });
}
