import { parseDocument, stringify } from 'yaml';
import { decodeProfile, profileDocument, type SkillProfile } from './model.ts';

/** Read the same strict YAML document format used by Gestalt Mobile. */
export function parseProfileYaml(source: string): SkillProfile {
  const document = parseDocument(source, { prettyErrors: false, strict: true, uniqueKeys: true });
  if (document.errors.length) throw new Error('Invalid skill profile YAML.');
  return decodeProfile(document.toJS());
}

/** Write a deterministic profile that Mobile can load without conversion. */
export function serializeProfileYaml(profile: SkillProfile): string {
  return stringify(profileDocument(decodeProfile(profileDocument(profile))));
}
