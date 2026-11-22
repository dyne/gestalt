import path from 'node:path';

export interface SkillEntry { readonly path: string; readonly name: string; readonly enabled: boolean }
export interface CatalogSkill extends SkillEntry {
  readonly displayName?: string;
  readonly description?: string;
  readonly shortDescription?: string;
  readonly brandColor?: string;
  readonly dependencies?: readonly { type: string; value: string; description?: string }[];
  readonly verified?: boolean;
}
export interface SkillProfile {
  readonly name: string;
  readonly skills: readonly SkillEntry[];
  /** Absence preserves the v1 format. A boolean is explicit v2 Playwright consent. */
  readonly browser?: boolean;
}

/** Normalize names using the same filename contract as Gestalt Mobile. */
export function profileName(value: string): string {
  const name = value.trim().toLowerCase().replace(/[\s_]+/g, '-');
  if (!/^[a-z0-9][a-z0-9-]{0,62}$/.test(name) ||
      (process.platform === 'win32' && /^(?:con|prn|aux|nul|com[0-9]|lpt[0-9])$/.test(name)))
    throw new Error('Enter a skill profile name of 1–63 letters, digits or hyphens.');
  return name;
}

/** Canonicalize identities for comparison without rewriting the saved path. */
export function skillKey(value: string): string {
  return process.platform === 'win32' ? path.win32.normalize(value).toLowerCase() : path.normalize(value);
}

function durableKey(value: string): string | undefined {
  const components = skillKey(value).split(path.sep);
  const index = components.lastIndexOf('plugins');
  if (index < 0 || components[index + 1] !== 'cache' || components[index + 5] !== 'skills') return;
  return [...components.slice(0, index + 4), ...components.slice(index + 5)].join(path.sep);
}

export const managedSkill = (skill: SkillEntry): boolean => skill.name.startsWith('gestalt:');

export function record(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

export function validSkillPath(value: unknown): value is string {
  return typeof value === 'string' && path.isAbsolute(value) && path.normalize(value) === value && !/[\x00-\x1f\x7f]/.test(value);
}

/** Decode Mobile's versioned profile document and reject ambiguous paths or fields. */
export function decodeProfile(value: unknown): SkillProfile {
  const input = record(value);
  const invalid = () => new Error('Invalid skill profile. Expected a Gestalt Mobile v1 or v2 document.');
  if (!input || (input.version !== 1 && input.version !== 2) || typeof input.name !== 'string' || !Array.isArray(input.skills) ||
      Object.keys(input).some(key => !['version', 'name', 'skills', ...(input.version === 2 ? ['capabilities'] : [])].includes(key))) throw invalid();
  let browser: boolean | undefined;
  if (input.version === 2) {
    const capabilities = record(input.capabilities), playwright = record(capabilities?.playwright);
    if (!capabilities || Object.keys(capabilities).length !== 1 || !playwright || Object.keys(playwright).length !== 1 ||
        typeof playwright.enabled !== 'boolean') throw invalid();
    browser = playwright.enabled;
  }
  const paths = new Set<string>();
  const skills: SkillEntry[] = [];
  for (const item of input.skills as unknown[]) {
    const skill = record(item);
    if (!skill || Object.keys(skill).length !== 3 || typeof skill.name !== 'string' || !skill.name.trim() ||
        typeof skill.enabled !== 'boolean' || !validSkillPath(skill.path)) throw invalid();
    const entry = { name: skill.name.trim(), path: skill.path, enabled: skill.enabled };
    if (managedSkill(entry)) continue;
    const key = skillKey(entry.path);
    if (paths.has(key)) throw new Error('Duplicate path in skill profile.');
    paths.add(key);
    skills.push(entry);
  }
  return { name: profileName(input.name), skills: skills.sort((a, b) => a.path.localeCompare(b.path)),
    ...(browser === undefined ? {} : { browser }) };
}

/** Encode complete selections in the document format used by Gestalt Mobile. */
export function profileDocument(profile: SkillProfile) {
  return { version: profile.browser === undefined ? 1 : 2, name: profile.name,
    skills: profile.skills.map(({ path, name, enabled }) => ({ name, path, enabled })),
    ...(profile.browser === undefined ? {} : { capabilities: { playwright: { enabled: profile.browser } } }) };
}

/** Keep installed Gestalt infrastructure outside the editable selection. */
export function reconcileSkills(catalog: readonly CatalogSkill[], selection?: readonly SkillEntry[]) {
  const entries = new Map(selection?.filter(skill => !managedSkill(skill)).map(skill => [skillKey(skill.path), skill]));
  const discovered = new Set(catalog.map(skill => skillKey(skill.path)));
  const rebound = new Map<string, SkillEntry>();
  for (const [key, entry] of entries) {
    if (discovered.has(key)) continue;
    const identity = durableKey(key);
    if (!identity) continue;
    const matches = catalog.filter(skill => durableKey(skill.path) === identity);
    const match = matches.length === 1 && matches[0];
    if (match && !entries.has(skillKey(match.path)) && !rebound.has(skillKey(match.path))) {
      rebound.set(skillKey(match.path), entry);
      entries.delete(key);
    }
  }
  return {
    skills: catalog.filter(skill => !managedSkill(skill)).map(skill => ({ ...skill,
      enabled: selection ? (entries.get(skillKey(skill.path)) ?? rebound.get(skillKey(skill.path)))?.enabled ?? false : skill.enabled,
    })),
    missing: [...entries].filter(([key]) => !discovered.has(key)).map(([, skill]) => ({ ...skill, enabled: false })),
  };
}

/** None keeps only mandatory Gestalt skills. Capability skills remain optional. */
export function profileSkillRules(catalog: readonly CatalogSkill[], selection?: readonly SkillEntry[]) {
  const mandatory = (skill: CatalogSkill) => managedSkill(skill) && !['gestalt:serena', 'gestalt:playwright'].includes(skill.name);
  if (selection === undefined) return { rules: catalog.map(skill => ({ path: skill.path, enabled: mandatory(skill) })), missing: [] };
  const { skills, missing } = reconcileSkills(catalog, selection);
  const fixed = catalog.filter(mandatory);
  const rules = [...skills.map(({ path, enabled }) => ({ path, enabled })), ...fixed.map(({ path }) => ({ path, enabled: true }))];
  return { rules, missing };
}
