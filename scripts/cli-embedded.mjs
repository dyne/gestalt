/** Compile the existing coherent JS owner, without a shell or runtime source
 * extraction. This boundary is consumed by install/update before skills dispatch
 * moves in the runtime milestone. */
export function skillsSetupProgram(manager) {
  manager = manager.replaceAll('\r\n', '\n');
  const start = manager.indexOf('command_skills() {');
  const end = manager.indexOf('\nvalidate_transfer_source_tree() {', start);
  const source = manager.slice(start, end).match(/<<'JS'\n([\s\S]*?)\nJS\n/);
  if (!source) throw new Error('Expected exactly one skills collection JS owner');
  return source[1]
    .replace("file === '/'", 'file === path.parse(file).root')
    .replace("resolved === '/'", 'resolved === path.parse(resolved).root')
    .replace("fs.realpathSync(process.env.HOME)", "fs.realpathSync(process.env.HOME || process.env.USERPROFILE)")
    .replaceAll("GIT_CONFIG_GLOBAL: '/dev/null'", "GIT_CONFIG_GLOBAL: process.platform === 'win32' ? 'NUL' : '/dev/null'")
    .replace("'core.hooksPath=/dev/null'", "'core.hooksPath=' + path.join(stage, 'no-hooks')")
    .replaceAll('fs.symlinkSync(before, file)', "fs.symlinkSync(before, file, process.platform === 'win32' ? 'junction' : 'dir')")
    .replaceAll('fs.symlinkSync(target(name), path.join(discovery, name))', "fs.symlinkSync(target(name), path.join(discovery, name), process.platform === 'win32' ? 'junction' : 'dir')");
}
