#!/usr/bin/env bash
set -Eeuo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/gestalt-install-test.XXXXXXXX")
trap 'rm -rf -- "$test_root"' EXIT

test_home=$test_root/home
bin_dir=$test_home/bin
mkdir -p -- "$test_home"

HOME=$test_home \
GESTALT_BIN_DIR=$bin_dir \
GESTALT_INSTALL_BASE_URL="file://$repo_root/public" \
  bash "$repo_root/public/install.sh" --no-setup

[[ -x $bin_dir/gestalt ]]
cmp "$repo_root/public/gestalt" "$bin_dir/gestalt"
expected_version=$(HOME=$test_home bash "$repo_root/public/gestalt" version)
installed_version=$(HOME=$test_home "$bin_dir/gestalt" version)
[[ $installed_version == "$expected_version" ]]

bad_source=$test_root/bad-source
mkdir -p -- "$bad_source"
printf '<!DOCTYPE html>\n' > "$bad_source/gestalt"

if HOME=$test_home \
  GESTALT_BIN_DIR=$test_root/bad-bin \
  GESTALT_INSTALL_BASE_URL="file://$bad_source" \
    bash "$repo_root/public/install.sh" --no-setup > /dev/null 2>&1; then
  printf 'expected invalid manager download to fail\n' >&2
  exit 1
fi

printf 'install.test: PASS\n'
