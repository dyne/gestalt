/** The released manager owns version identity until the native CLI cuts over. */
export function managerVersion(source) {
  const versions = [...source.matchAll(/^readonly GESTALT_CLI_VERSION='([^']*)'$/gm)];
  if (versions.length !== 1 || !/^(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$/.test(versions[0][1]))
    throw new Error('expected exactly one strict manager version');
  return versions[0][1];
}

export function managerHelp(source) {
  const blocks = [...source.matchAll(/^usage\(\) \{\n  cat <<'EOF'\n([\s\S]*?)^EOF\n\}/gm)];
  if (blocks.length !== 1) throw new Error('expected exactly one manager usage block');
  return blocks[0][1];
}
