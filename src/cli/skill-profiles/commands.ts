import { realpath } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import type { CommandInvocation, CommandStreams } from '../contracts.ts';
import { SetupContext, setupPaths } from '../setup/context.ts';
import { discoverSkills } from './catalog.ts';
import { FileSkillProfiles } from './store.ts';
import { skillLaunchOptions } from './launch-options.ts';
import { profileName, profileSkillRules, type SkillProfile } from './model.ts';
import { editProfiles, pickProfile, type EditorPorts } from './tui.ts';

async function profilePorts(invocation: CommandInvocation, streams: CommandStreams,
  discovery: readonly string[] = []): Promise<EditorPorts> {
  const cwd = await realpath(invocation.cwd);
  const ownershipHelper = process.platform === 'win32' ? fileURLToPath(new URL('./native/gestalt-owned-process.exe', import.meta.url)) : undefined;
  const paths = await setupPaths(invocation.environment, ownershipHelper);
  const context = new SetupContext(invocation, streams, paths, '1');
  const store = new FileSkillProfiles(context);
  return {
    list: () => store.list(), defaults: () => store.defaults(cwd),
    discover: () => discoverSkills({ ...invocation, cwd }, streams, discovery),
    save: profile => store.save(profile), delete: name => store.delete(name),
  };
}

/** Start the standalone editor in the current Codex workspace. */
export async function manageSkillProfiles(invocation: CommandInvocation, streams: CommandStreams,
  interactive: boolean): Promise<number> {
  if (invocation.argv.length && ['--help', '-h'].includes(invocation.argv[0]!)) {
    await streams.stdout(Buffer.from('Usage: gestalt skill-profiles\nManage skill profiles for the current workspace.\n'));
    return 0;
  }
  if (invocation.argv.length) throw new Error('Usage: gestalt skill-profiles');
  if (!interactive) throw new Error('The skill-profile manager requires an interactive terminal.');
  await editProfiles(await profilePorts(invocation, streams));
  return 0;
}

/** Build a launch-only override. Existing Codex flags and config files stay intact. */
export async function prepareProfileLaunch(invocation: CommandInvocation, streams: CommandStreams,
  interactive: boolean): Promise<{ argv: readonly string[]; skillRules?: string } | undefined> {
  const options = skillLaunchOptions(invocation.argv, invocation.cwd);
  const launch = { argv: options.argv };
  if (!options.profile && (!options.prompt || !interactive)) return launch;
  const ports = await profilePorts({ ...invocation, cwd: options.cwd }, streams, options.discovery);
  let profile: SkillProfile | undefined;
  if (options.profile) {
    const name = profileName(options.profile);
    profile = (await ports.list()).profiles.find(entry => entry.name === name);
    if (!profile) throw new Error(`Unknown skill profile: ${options.profile}`);
  } else {
    while (true) {
      const choice = await pickProfile(await ports.list());
      if (!choice) return undefined;
      if (choice.kind === 'manage') { await editProfiles(ports, 'inline'); continue; }
      profile = choice.kind === 'profile' ? choice.profile : undefined;
      break;
    }
  }
  const catalog = await ports.discover();
  const effective = profileSkillRules(catalog.skills, profile?.skills);
  for (const warning of catalog.warnings) await streams.stderr(Buffer.from(`gestalt: ${warning.replace(/[\r\n\x1b]/g, ' ')}\n`));
  for (const skill of effective.missing) await streams.stderr(Buffer.from(`gestalt: skill profile entry is missing and disabled: ${skill.path}\n`));
  return { ...launch, skillRules: JSON.stringify(effective.rules) };
}
