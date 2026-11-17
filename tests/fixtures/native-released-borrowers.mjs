// Durable schema evidence, never a substitute for a matching native Job claim.
const uuid = value => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(value);
const id = value => typeof value === 'string' && /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$/.test(value);
export function releasedBorrowerInventory(primary, before, after, expectedCount) {
  const failures = [], added = [], fail = value => failures.push(value);
  if (!Array.isArray(before) || !Array.isArray(after)) return { pass: false, failures: ['borrower-inventory-missing'], added };
  const validate = (records, label) => {
    const ids = new Set(), identities = new Set();
    for (const record of records) {
      const lease = record?.lease;
      if (!lease || lease.schemaVersion !== 1 || !uuid(lease.leaseId) || !uuid(lease.processIdentity) ||
          lease.ownerId !== primary.context.ownerId || lease.workspacePath !== primary.context.workspacePath ||
          typeof record.file !== 'string' || record.file.split(/[\\/]/).at(-1) !== lease.leaseId + '.json' ||
          typeof record.sha256 !== 'string' || !/^[0-9a-f]{64}$/.test(record.sha256) ||
          !['active', 'released'].includes(lease.state) || !Array.isArray(lease.consumers) || !lease.consumers.every(id) ||
          lease.state === 'released' && lease.consumers.length !== 0 ||
          !['createdAt', 'updatedAt'].every(key => typeof lease[key] === 'string' && Number.isFinite(Date.parse(lease[key])))) {
        fail(label + ':borrower-schema-or-identity-invalid'); continue;
      }
      if (ids.has(lease.leaseId) || identities.has(lease.processIdentity)) fail(label + ':duplicate-borrower-identity');
      ids.add(lease.leaseId); identities.add(lease.processIdentity);
    }
  };
  validate(before, 'before'); validate(after, 'after');
  const previous = new Map(before.filter(record => record?.lease).map(record => [record.lease.leaseId, record]));
  for (const [leaseId, record] of previous) {
    const current = after.find(value => value?.lease?.leaseId === leaseId);
    if (!current || current.file !== record.file || current.sha256 !== record.sha256 || JSON.stringify(current.lease) !== JSON.stringify(record.lease)) fail('existing-borrower-changed-or-missing');
  }
  for (const record of after) if (record?.lease && !previous.has(record.lease.leaseId)) added.push(record);
  if (!added.length || expectedCount !== undefined && added.length !== expectedCount) fail('new-borrower-count-invalid');
  const existingIdentities = new Set(before.map(record => record?.lease?.processIdentity));
  if (added.some(record => record.lease.state !== 'released' || record.lease.consumers?.length !== 0 || existingIdentities.has(record.lease.processIdentity))) fail('new-borrower-release-or-identity-invalid');
  const initial = before.find(record => record?.lease?.leaseId === primary.lease.leaseId), final = after.find(record => record?.lease?.leaseId === primary.lease.leaseId);
  if (initial?.lease.state !== 'active' || final?.lease.state !== 'active' || initial?.sha256 !== final?.sha256) fail('primary-borrower-not-active-and-unchanged');
  return { pass: failures.length === 0, failures, added, kernelReleaseClaimed: false, liveConsumerLabelAfterReleaseClaimed: false };
}
