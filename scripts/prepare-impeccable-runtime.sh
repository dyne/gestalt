#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
repo=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
inputs=$repo/vendor/impeccable
value() { node -p 'JSON.parse(require("node:fs").readFileSync(process.argv[1],"utf8"))[process.argv[2]]' "$inputs/provenance.json" "$1"; }
verify_hash() { printf '%s  %s\n' "$1" "$2" | sha256sum --check --status; }
verify_hash "$(value patchSha256)" "$inputs/remote-public-url.patch"
if [[ ${1:-} == --verify-inputs && $# == 1 ]]; then exit 0; fi
[[ $# == 2 && $1 == /* && $2 == /* && ! -e $1 && ! -e $2 ]] || {
  printf 'usage: prepare-impeccable-runtime.sh NEW_ABSOLUTE_SOURCE_DIR NEW_ABSOLUTE_TOOLS_DIR\n' >&2; exit 1;
}
source_root=$1 tools=$2
[[ $(value upstream) == https://github.com/pbakaus/impeccable.git ]]
git init --quiet "$source_root"
git -C "$source_root" -c core.hooksPath=/dev/null -c credential.helper= fetch --quiet --depth=1 \
  https://github.com/pbakaus/impeccable.git "$(value baseCommit)"
git -C "$source_root" -c core.hooksPath=/dev/null checkout --quiet --detach FETCH_HEAD
[[ $(git -C "$source_root" rev-parse HEAD) == "$(value baseCommit)" ]]
git -C "$source_root" apply --check "$inputs/remote-public-url.patch"
git -C "$source_root" apply --index "$inputs/remote-public-url.patch"
[[ $(git -C "$source_root" write-tree) == "$(value reconstructedTree)" ]]
verify_hash "$(value cargoLockSha256)" "$source_root/Cargo.lock"
verify_hash "$(value bunLockSha256)" "$source_root/bun.lock"
mkdir -p -- "$tools/debs" "$tools/sysroot" "$tools/bin"
package_rows=$(node -e 'const d=require(process.argv[1]);for(const p of d.packages)console.log([p.name,p.url,p.sha256].join("\t"))' "$inputs/provenance.json")
while IFS=$'\t' read -r name url checksum; do
  [[ $name != */* && $url == https://deb.debian.org/debian/pool/* ]]
  curl -q --fail --location --silent --show-error --proto '=https' --proto-redir '=https' \
    --connect-timeout 20 --max-time 120 "$url" --output "$tools/debs/$name"
  verify_hash "$checksum" "$tools/debs/$name"
  dpkg-deb --extract "$tools/debs/$name" "$tools/sysroot"
done <<< "$package_rows"
# Relocate the pinned musl sysroot; never install packages on the host.
sed "s@/usr/@$tools/sysroot/usr/@g" "$tools/sysroot/usr/lib/x86_64-linux-musl/musl-gcc.specs" > "$tools/musl-gcc.specs"
printf '#!/bin/sh\nexec x86_64-linux-gnu-gcc "$@" -specs "%s"\n' "$tools/musl-gcc.specs" > "$tools/bin/musl-gcc"
chmod 0755 "$tools/bin/musl-gcc"
mkdir -p -- "$tools/fontconfig"
cat > "$tools/fontconfig/fonts.conf" <<EOF
<?xml version="1.0"?><!DOCTYPE fontconfig SYSTEM "fonts.dtd">
<fontconfig><dir>$tools/sysroot/usr/share/fonts/truetype/liberation</dir><dir>/usr/share/fonts</dir><cachedir>$tools/fontconfig/cache</cachedir></fontconfig>
EOF
printf 'Reconstructed accepted source tree %s; pinned tools extracted privately\n' "$(value reconstructedTree)"
