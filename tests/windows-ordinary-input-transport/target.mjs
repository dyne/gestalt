import assert from 'node:assert/strict';

const [exit, ...argv] = process.argv.slice(2);
assert.ok(exit === '0' || exit === '23');
const maximumBytes = 4096;
let bytes = 0;
for await (const input of process.stdin) {
  bytes += input.length;
  assert.ok(bytes <= maximumBytes);
  await new Promise((resolve, reject) => {
    process.stdout.write(input, error => error ? reject(error) : resolve());
  });
}
process.stderr.write(JSON.stringify({ eof: true, bytes, argv, cwd: process.cwd(), operator: process.env.ORDINARY_INPUT_OPERATOR }) + '\n');
process.exitCode = Number(exit);
