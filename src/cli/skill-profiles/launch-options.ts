import path from 'node:path';

const nativeCommands = new Set(['agents', 'exec', 'e', 'review', 'login', 'logout', 'mcp', 'plugin', 'app-server',
  'remote-control', 'app', 'completion', 'update', 'doctor', 'sandbox', 'debug', 'apply', 'a', 'queue', 'archive',
  'delete', 'migrate-rollouts', 'migrate-nfs-db', 'unarchive', 'cloud', 'exec-server', 'features', 'help']);
const valueFlags = new Set(['-m', '--model', '-i', '--image', '--local-provider', '-s', '--sandbox', '-a', '--ask-for-approval',
  '--search-provider', '--add-dir', '--remote-auth-token-env', '--permissions']);

/** Consume Gestalt's launch flags while preserving the user's native argument order. */
export function skillLaunchOptions(input: readonly string[], cwd: string) {
  const args = input[0] === '--' ? input.slice(1) : input;
  const argv: string[] = [], discovery: string[] = [];
  let profile: string | undefined, skip = false, native = false, explicitSkills = false, position: string | undefined;
  for (let index = 0; index < args.length; index++) {
    const argument = args[index]!;
    if (argument === '--') {
      argv.push(...args.slice(index));
      break;
    }
    if (argument === '--no-skill-profile') { skip = true; continue; }
    if (argument === '--skill-profile' || argument.startsWith('--skill-profile=')) {
      if (profile !== undefined) throw new Error('Specify one skill profile per launch.');
      profile = argument === '--skill-profile' ? args[++index] : argument.slice('--skill-profile='.length);
      if (!profile || profile.startsWith('-')) throw new Error('--skill-profile requires a saved profile name.');
      continue;
    }
    const pair = ['-C', '--cd', '-p', '--profile', '-c', '--config', '--enable', '--disable'].includes(argument);
    const next = pair || valueFlags.has(argument) || argument === '--remote' ? args[++index] : undefined;
    argv.push(argument, ...(next === undefined ? [] : [next]));
    const option = (short: string, long: string) => argument === short || argument === long ? next
      : argument.startsWith(`${long}=`) ? argument.slice(long.length + 1)
      : argument.startsWith(short) && argument.length > short.length ? argument.slice(short.length) : undefined;
    const directory = option('-C', '--cd');
    if (directory !== undefined) cwd = path.resolve(cwd, directory);
    const codexProfile = option('-p', '--profile');
    if (codexProfile !== undefined) discovery.push('--profile', codexProfile);
    const config = option('-c', '--config');
    if (config !== undefined) {
      discovery.push('-c', config);
      explicitSkills ||= /^skills(?:\.config)?\s*=/.test(config);
    }
    if (argument === '--enable' || argument === '--disable') { if (next) discovery.push(argument, next); }
    else if (/^--(?:enable|disable)=/.test(argument)) discovery.push(argument);
    if (['-h', '--help', '-V', '--version', '--remote'].includes(argument) || argument.startsWith('--remote=')) native = true;
    if (!argument.startsWith('-')) position ??= argument;
  }
  if (position && nativeCommands.has(position)) native = true;
  if (profile && (skip || native || explicitSkills))
    throw new Error('--skill-profile requires a local Codex session without another skill override.');
  return { argv: input[0] === '--' ? ['--', ...argv] : argv, cwd, discovery, profile, prompt: !skip && !native && !explicitSkills };
}
