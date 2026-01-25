import test from 'node:test';
import assert from 'node:assert/strict';
import { decodeSymbolId, encodeSymbolId, encodeSymbolIdForOutput } from '../../src/symbol-id-codec.js';

test('symbol-id codec round-trips raw SCIP ids with base64url', () => {
  const rawSymbolId = 'scip-go gomod example v1 `internal/terminal`/Manager#';
  const encoded = encodeSymbolId(rawSymbolId);

  assert.match(encoded, /^[A-Za-z0-9_-]+$/);
  assert.ok(!encoded.includes(' '));
  assert.ok(!encoded.includes('`'));
  assert.equal(decodeSymbolId(encoded), rawSymbolId);
  assert.equal(encodeSymbolIdForOutput(encoded), encoded);
});

test('symbol-id codec leaves invalid base64url inputs unchanged', () => {
  assert.equal(decodeSymbolId('not base64url!'), 'not base64url!');
  assert.equal(decodeSymbolId('Zm9v'), 'Zm9v');
});

