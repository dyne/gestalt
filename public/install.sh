#!/usr/bin/env bash
# Download, verify, and install the Gestalt manager in a user-owned directory.
set -Eeuo pipefail
IFS=$'\n\t'

readonly DEFAULT_BASE_URL='https://dyne.github.io/gestalt'
skip_setup=false
temp_root=''

log() {
  printf 'gestalt-installer: %s\n' "$*" >&2
}

die() {
  log "error: $*"
  exit 1
}

cleanup() {
  if [[ -n $temp_root && -d $temp_root ]]; then
    rm -rf -- "$temp_root"
  fi
}
trap cleanup EXIT

usage() {
  cat <<'EOF'
Usage: install.sh [--no-setup]

Download and checksum the Gestalt manager, install it under
${GESTALT_BIN_DIR:-$HOME/.local/bin}, then set up Agents and Mobile.

  --no-setup  Install only the manager; do not run `gestalt install`.
  -h, --help  Show this help.
EOF
}

while (($# > 0)); do
  case $1 in
    --no-setup) skip_setup=true ;;
    -h | --help) usage; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
  shift
done

command -v curl >/dev/null 2>&1 || die 'curl is required'
command -v mktemp >/dev/null 2>&1 || die 'mktemp is required'
: "${HOME:?HOME is required}"

base_url=${GESTALT_INSTALL_BASE_URL:-$DEFAULT_BASE_URL}
bin_dir=${GESTALT_BIN_DIR:-$HOME/.local/bin}
[[ $bin_dir == /* ]] || die "GESTALT_BIN_DIR must be absolute: $bin_dir"
[[ $bin_dir != / && $bin_dir != "$HOME" ]] || die "refusing unsafe GESTALT_BIN_DIR: $bin_dir"

temp_root=$(mktemp -d "${TMPDIR:-/tmp}/gestalt-install.XXXXXXXX") ||
  die 'could not create a temporary directory'
manager_download=$temp_root/gestalt
checksum_download=$temp_root/gestalt.sha256

log "downloading manager from ${base_url%/}/gestalt"
curl --fail --silent --show-error --location \
  "${base_url%/}/gestalt" --output "$manager_download"
curl --fail --silent --show-error --location \
  "${base_url%/}/gestalt.sha256" --output "$checksum_download"

IFS=' ' read -r expected_checksum _ < "$checksum_download" || die 'could not read checksum file'
[[ $expected_checksum =~ ^[[:xdigit:]]{64}$ ]] || die 'published checksum has an invalid format'

if command -v sha256sum >/dev/null 2>&1; then
  actual_checksum=$(sha256sum "$manager_download")
elif command -v shasum >/dev/null 2>&1; then
  actual_checksum=$(shasum -a 256 "$manager_download")
else
  die 'sha256sum or shasum is required to verify the manager'
fi
actual_checksum=${actual_checksum%% *}
[[ $actual_checksum == "$expected_checksum" ]] || die 'manager checksum verification failed'

mkdir -p -- "$bin_dir"
staged_target=$(mktemp "$bin_dir/.gestalt.XXXXXXXX") || die 'could not stage manager installation'
install -m 0755 "$manager_download" "$staged_target"
mv -f -- "$staged_target" "$bin_dir/gestalt"
log "installed verified manager at $bin_dir/gestalt"

case :$PATH: in
  *:"$bin_dir":*) ;;
  *) log "add this directory to PATH: export PATH=\"$bin_dir:\$PATH\"" ;;
esac

if ! "$skip_setup"; then
  "$bin_dir/gestalt" install
fi
