// Setup and runtime share one host-side v1/v2 schema and metadata-fence owner.
import { createScratchOwner, scratchEnvironment } from '../runtime/scratch-owner.mjs';
export { scratchEnvironment };
const bindings = new WeakMap();
export function borrowSetupScratch(environment, privateProbe) {
  const owner = createScratchOwner(privateProbe), context = owner.scratchContext(environment);
  if (!context) return null;
  const borrower = owner.scratchBorrow(context, 'manager-setup');
  bindings.set(borrower, owner);
  return borrower;
}
export async function releaseSetupScratch(ownership) {
  const owner = ownership && bindings.get(ownership);
  return owner ? owner.scratchReleaseReady(ownership) : false;
}
