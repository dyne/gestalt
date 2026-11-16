// Test-only inert loopback endpoint. Pinned Codex doctor sends HEAD /responses
// then GET /models with no body. No generation endpoint is implemented here.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';

export function providerProbeViolation(record) {
  if (!['eof', 'interrupt'].includes(record.control)) return 'request outside an active fixture case';
  if (!(record.method === 'HEAD' && record.path === '/responses' || record.method === 'GET' && record.path === '/models'))
    return 'generation, query-bearing or other request';
  if (record.bodyComplete !== true || record.bodyBytes !== 0) return 'nonempty or unconfirmed request body';
  if (record.contentLength !== null && record.contentLength !== '0') return 'nonzero declared request body';
  if (record.transferEncoding !== null) return 'streamed request body';
  if (record.blockedHeaderNames.length) return 'authentication, upgrade or prompt-bearing header';
  return null;
}

export function assertMetadataProbesOnly(records, control) {
  for (const record of records) assert.equal(providerProbeViolation(record), null, JSON.stringify(record));
  const selected = records.filter(record => record.control === control);
  assert.ok(selected.length <= 2, 'Repeated provider metadata probes exceed the pinned doctor sequence');
  if (selected.length) assert.equal(selected[0].method + ' ' + selected[0].path, 'HEAD /responses');
  if (selected.length === 2) assert.equal(selected[1].method + ' ' + selected[1].path, 'GET /models');
  return { control, metadataProbes: selected.length, generationOrOtherRequests: 0 };
}

export function captureProviderProbes(records, currentCase) {
  return (request, response) => {
    const record = { control: currentCase(), method: request.method, path: request.url, receivedAt: new Date().toISOString(),
      contentLength: request.headers['content-length'] ?? null, transferEncoding: request.headers['transfer-encoding'] ?? null,
      blockedHeaderNames: Object.keys(request.headers).filter(name => /authorization|cookie|upgrade|prompt|input|message/i.test(name)),
      bodyBytes: 0, bodyComplete: false, completedAt: null, bodySha256: null };
    records.push(record);
    const hash = createHash('sha256'); let finished = false;
    const finish = complete => {
      if (finished) return; finished = true;
      record.bodyComplete = complete; record.completedAt = new Date().toISOString(); record.bodySha256 = hash.digest('hex');
      record.violation = providerProbeViolation(record);
    };
    request.on('data', bytes => {
      if (finished) return;
      record.bodyBytes += bytes.length; hash.update(bytes);
      if (record.bodyBytes > 1024 * 1024) { finish(false); request.destroy(); }
    });
    request.on('aborted', () => { finish(false); });
    request.on('error', () => { finish(false); });
    request.on('end', () => {
      if (finished) return;
      finish(true);
      response.writeHead(503); response.end('Inert fixture: no model service');
    });
  };
}
