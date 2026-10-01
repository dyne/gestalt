#!/usr/bin/env bash
set -Eeuo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/gestalt-skills-transfer.XXXXXXXX")
trap 'rm -rf -- "$test_root"' EXIT

source_home=$test_root/source-home
source_codex=$source_home/.codex-gestalt
source_gestalt=$source_home/.gestalt
archive=$test_root/skills.tar.gz
mkdir -p \
  "$source_home/.agents/skills/user-skill" \
  "$source_gestalt/.agents/skills/managed-skill/scripts" \
  "$source_codex/skills/local-skill" \
  "$source_codex/skills/.system/system-skill" \
  "$source_gestalt/skill-profiles"
printf '%s\n' '# User skill' > "$source_home/.agents/skills/user-skill/SKILL.md"
printf '%s\n' '# Managed skill' > "$source_gestalt/.agents/skills/managed-skill/SKILL.md"
printf '%s\n' '#!/usr/bin/env bash' 'printf managed' > \
  "$source_gestalt/.agents/skills/managed-skill/scripts/run"
chmod 0755 "$source_gestalt/.agents/skills/managed-skill/scripts/run"
printf '%s\n' '# Local skill' > "$source_codex/skills/local-skill/SKILL.md"
printf '%s\n' '# System skill' > "$source_codex/skills/.system/system-skill/SKILL.md"
ln -s "$source_gestalt/.agents/skills/managed-skill" "$source_codex/skills/managed-skill"
cat > "$source_gestalt/skill-profiles/focused.yml" <<EOF
version: 1
name: focused
skills:
  - name: user-skill
    path: $source_home/.agents/skills/user-skill/SKILL.md
    enabled: true
  - name: managed-skill
    path: $source_gestalt/.agents/skills/managed-skill/SKILL.md
    enabled: true
  - name: local-skill
    path: $source_codex/skills/local-skill/SKILL.md
    enabled: false
EOF

HOME=$source_home CODEX_HOME=$source_codex GESTALT_HOME=$source_gestalt \
  bash "$repo_root/public/gestalt" skills-export "$archive"
[[ -f $archive ]]
tar -tzf "$archive" > "$test_root/members"
grep -F 'gestalt-skills/user-agents-skills/user-skill/SKILL.md' "$test_root/members" >/dev/null
grep -F 'gestalt-skills/managed-skills/managed-skill/SKILL.md' "$test_root/members" >/dev/null
grep -F 'gestalt-skills/codex-skills/local-skill/SKILL.md' "$test_root/members" >/dev/null
grep -F 'gestalt-skills/skill-profiles/focused.yml' "$test_root/members" >/dev/null
if grep -F '.system' "$test_root/members" >/dev/null; then
  printf 'system skills unexpectedly entered export\n' >&2
  exit 1
fi
if HOME=$source_home CODEX_HOME=$source_codex GESTALT_HOME=$source_gestalt \
  bash "$repo_root/public/gestalt" skills-export "$archive" >/dev/null 2>&1; then
  printf 'skills export unexpectedly overwrote its destination\n' >&2
  exit 1
fi

target_home=$test_root/target-home
target_codex=$target_home/.codex-gestalt
target_gestalt=$target_home/.gestalt
mkdir -p "$target_home/.agents/skills/unrelated"
printf '%s\n' '# Unrelated' > "$target_home/.agents/skills/unrelated/SKILL.md"
HOME=$target_home CODEX_HOME=$target_codex GESTALT_HOME=$target_gestalt \
  bash "$repo_root/public/gestalt" skills-import "$archive"

cmp "$source_home/.agents/skills/user-skill/SKILL.md" \
  "$target_home/.agents/skills/user-skill/SKILL.md"
cmp "$source_gestalt/.agents/skills/managed-skill/SKILL.md" \
  "$target_gestalt/.agents/skills/managed-skill/SKILL.md"
cmp "$source_codex/skills/local-skill/SKILL.md" \
  "$target_codex/skills/local-skill/SKILL.md"
[[ -x $target_gestalt/.agents/skills/managed-skill/scripts/run ]]
[[ -f $target_home/.agents/skills/unrelated/SKILL.md ]]
[[ -L $target_codex/skills/managed-skill ]]
[[ $(readlink "$target_codex/skills/managed-skill") == \
  "$target_gestalt/.agents/skills/managed-skill" ]]
profile=$target_gestalt/skill-profiles/focused.yml
grep -F "path: $target_home/.agents/skills/user-skill/SKILL.md" "$profile" >/dev/null
grep -F "path: $target_gestalt/.agents/skills/managed-skill/SKILL.md" "$profile" >/dev/null
grep -F "path: $target_codex/skills/local-skill/SKILL.md" "$profile" >/dev/null
if grep -F "$source_home" "$profile" >/dev/null; then
  printf 'source installation path remained in imported profile\n' >&2
  exit 1
fi

printf '%s\n' '# Restored user skill' > "$source_home/.agents/skills/user-skill/SKILL.md"
second_archive=$test_root/second.tar.gz
HOME=$source_home CODEX_HOME=$source_codex GESTALT_HOME=$source_gestalt \
  bash "$repo_root/public/gestalt" skills-export "$second_archive"
HOME=$target_home CODEX_HOME=$target_codex GESTALT_HOME=$target_gestalt \
  bash "$repo_root/public/gestalt" skills-import "$second_archive"
grep -F '# Restored user skill' "$target_home/.agents/skills/user-skill/SKILL.md" >/dev/null

malicious_root=$test_root/malicious
mkdir -p "$malicious_root/gestalt-skills/user-agents-skills/bad-skill"
ln -s /etc/passwd "$malicious_root/gestalt-skills/user-agents-skills/bad-skill/SKILL.md"
malicious_archive=$test_root/malicious.tar.gz
tar -czf "$malicious_archive" -C "$malicious_root" gestalt-skills
if HOME=$target_home CODEX_HOME=$target_codex GESTALT_HOME=$target_gestalt \
  bash "$repo_root/public/gestalt" skills-import "$malicious_archive" >/dev/null 2>&1; then
  printf 'skills import unexpectedly accepted an archive link\n' >&2
  exit 1
fi

printf 'skills-transfer.test: PASS\n'
