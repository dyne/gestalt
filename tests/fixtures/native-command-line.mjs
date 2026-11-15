import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

export function quotingBlock(source) {
  const start = source.indexOf('/* Windows CRT quoting:');
  const legacy = source.indexOf('static wchar_t *command_line(');
  const end = source.indexOf('static int run_owned_windows(', legacy);
  assert.ok(legacy >= 0 && end > legacy);
  return source.slice(start >= 0 ? start : legacy, end);
}
export async function countProbeSource(source) {
  const base = new URL('./', import.meta.url);
  return source.slice(0, source.indexOf('#define _GNU_SOURCE')) + (await readFile(new URL('native-command-line-count.c', base), 'utf8'))
    .replace('/* QUOTING_BLOCK */', quotingBlock(source))
    .replace('/* VECTOR_IO */', await readFile(new URL('native-command-line-vector.h', base), 'utf8'));
}
export function vectorBytes(argv) {
  const pieces = [], count = Buffer.alloc(4); count.writeUInt32LE(argv.length); pieces.push(count);
  for (const arg of argv) { const b = Buffer.from(arg, 'utf16le'), n = Buffer.alloc(4); n.writeUInt32LE(b.length / 2); pieces.push(n, b); }
  return Buffer.concat(pieces);
}
export function wideReader(bytes) {
  let offset = 0;
  return { integer() { assert.ok(offset + 4 <= bytes.length); const n = bytes.readUInt32LE(offset); offset += 4; return n; },
    string() { const n = this.integer(); assert.ok(offset + n * 2 <= bytes.length); const s = bytes.subarray(offset, offset + n * 2).toString('utf16le'); offset += n * 2; return s; },
    done() { assert.equal(offset, bytes.length); } };
}
export function quoteReference(argv) {
  return argv.map(arg => '"' + arg.replace(/(\\*)"/g, '$1$1\\"').replace(/(\\+)$/, '$1$1') + '"').join(' ');
}
export function quotingCases(capture) {
  return [
    { name: 'exact captured15argv hook query', argv: [capture.target, ...capture.argv], units: 27810 },
    { name: 'empty literal', argv: [''], line: '""' },
    { name: 'nonBMP UTF16 pair', argv: ['é東京🚀'], line: '"é東京🚀"' },
    { name: 'embedded quote', argv: ['a"b'], line: '"a\\"b"' },
    { name: 'ordinary backslashes', argv: ['x\\\\y'], line: '"x\\\\y"' },
    { name: 'trailing backslash', argv: ['tail\\'], line: '"tail\\\\"' },
    { name: 'slashes before quote', argv: ['x\\\\"'], line: String.fromCharCode(34,120,92,92,92,92,92,34,34) },
    { name: 'empty Unicode metachar vector', argv: ['p', '', 'space é🚀', '&|<>^%!$()', '"', '\\'], units: null },
    { name: '32767units includingNUL', argv: ['p', 'a'.repeat(32760)], units: 32767 },
    { name: '32768units rejected', argv: ['p', 'a'.repeat(32761)], rejected: true },
    { name: '32767escaped backslash units', argv: ['p', '\\'.repeat(16380)], units: 32767 },
    { name: 'escaped backslash oversize', argv: ['p', '\\'.repeat(16381)], rejected: true },
    { name: '32767escaped quote units', argv: ['p', '"'.repeat(16380)], units: 32767 },
    { name: 'escaped quote oversize', argv: ['p', '"'.repeat(16381)], rejected: true },
    { name: '32767units with surrogate pairs', argv: ['p', '🚀'.repeat(16380)], units: 32767 },
    { name: 'many empty argument overhead rejected', argv: Array(20000).fill(''), rejected: true },
  ];
}
export function checkCountResult(item, bytes) {
  const reader = wideReader(bytes), accepted = reader.integer(), error = reader.integer(), line = reader.string(); reader.done();
  if (item.rejected) { assert.equal(accepted, 0); assert.equal(error, 206); return { accepted: false, error }; }
  assert.equal(accepted, 1); assert.equal(line, item.line ?? quoteReference(item.argv));
  assert.equal(line.length + 1, item.units ?? line.length + 1);
  return { accepted: true, unitsIncludingNul: line.length + 1 };
}
