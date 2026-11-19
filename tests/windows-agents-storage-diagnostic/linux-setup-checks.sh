#!/usr/bin/env bash
set -euo pipefail

workspace=$(pwd -P)
agents="$workspace/.native-dependencies/agents"
receipt="$workspace/native-evidence/linux-agents-storage"
mkdir -p "$receipt"
fixture=
completed=false
finish() {
  local exit_code=$?
  trap - EXIT
  local disposition='uncreated'
  if [[ -n $fixture ]]; then
    if [[ $completed == true ]]; then
      if rmdir -- "$fixture"; then disposition='removed after successful fixtures'; else disposition='retained: fixture directory is not empty'; exit_code=1; fi
    else
      disposition='retained for disposable runner'
    fi
  fi
  printf '%s\n' "$exit_code" > "$receipt/gate.exit"
  printf '%s\n' "$disposition" > "$receipt/fixture-disposition.txt"
  exit "$exit_code"
}
trap finish EXIT
[[ ${GITHUB_ACTIONS:-} == true && ${PROBE_RUNNER_ENVIRONMENT:-} == github-hosted && ${RUNNER_OS:-} == Linux ]]
[[ $(node --version) == v24.22.3 && $(uname -m) == x86_64 ]]
[[ $(git rev-parse HEAD) == "${GITHUB_SHA:?}" && $(git rev-parse HEAD^) == 283c74bfee93ca05bd4c82dd9608429434cff490 ]]
[[ $(git -C "$agents" rev-parse HEAD) == b4f783d75c0543010c665072bf7aef07f74f973f ]]
[[ $(git -C "$agents" rev-parse 'HEAD^{tree}') == 69980d21f265610045293a872c18803de5568a8c ]]
[[ -z $(git -C "$agents" status --porcelain --untracked-files=all) ]]
git -C "$agents" ls-tree -r -z HEAD > "$receipt/agents-git-tree.bin"
git ls-tree -r -z HEAD > "$receipt/manager-git-tree.bin"
[[ $(git -C "$agents" ls-tree -r --name-only HEAD | wc -l) == 755 ]]
[[ $(stat -c '%u %a' /tmp) == '0 1777' && $(id -u) != 0 ]]
fixture=$(mktemp -d /tmp/gestalt-agents-storage.XXXXXXXX)
printf '%s\n' "$fixture" > "$receipt/fixture-path.txt"
{
  date -u +%FT%TZ
  uname -mnr
  node --version
  stat -c '%n uid=%u mode=%a' / /tmp "$fixture"
  git rev-parse HEAD 'HEAD^{tree}' HEAD^
  git -C "$agents" rev-parse HEAD 'HEAD^{tree}'
} > "$receipt/identity.txt"
cd "$agents"
run() {
  local id=$1
  shift
  date -u +%FT%TZ > "$receipt/$id.started"
  printf '%s\n' "$@" > "$receipt/$id.argv"
  local result=0
  env -u GESTALT_SESSION_TMP_CONTEXT TMPDIR="$fixture" TMP="$fixture" TEMP="$fixture" "$@" \
    >"$receipt/$id.stdout" 2>"$receipt/$id.stderr" || result=$?
  printf '%d\n' "$result" > "$receipt/$id.exit"
  date -u +%FT%TZ > "$receipt/$id.observed"
  return "$result"
}
first=0
second=0
run storage-setup-helpers bash tests/repository/test-native-setup.sh || first=$?
run setup-api node --test --test-reporter=tap tests/repository/test-setup-api.mjs || second=$?
[[ $first == 0 && $second == 0 ]]
node --input-type=module - "$receipt" <<'JS'
import assert from 'node:assert/strict';
import {readFile,writeFile} from 'node:fs/promises';
import path from 'node:path';
const dir=process.argv[2];
const groups=[];
for (const [name,expected] of [['storage-setup-helpers',21],['setup-api',3]]) {
  const text=await readFile(path.join(dir,name+'.stdout'),'utf8');
  const counts=Object.fromEntries([...text.matchAll(/^# (tests|pass|fail|cancelled|skipped|todo) (\d+)$/gm)].map(([,key,value])=>[key,+value]));
  assert.deepEqual(counts,{tests:expected,pass:expected,fail:0,cancelled:0,skipped:0,todo:0});
  groups.push({name,...counts});
}
await writeFile(path.join(dir,'results.json'),JSON.stringify({scope:'Existing Agents fixture suites',groups},null,2)+'\n');
JS
[[ -z $(git status --porcelain --untracked-files=all) ]]
completed=true
