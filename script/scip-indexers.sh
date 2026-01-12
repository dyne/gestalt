#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ASSETS_DIR="${ROOT_DIR}/assets/scip"
MANIFEST_PATH="${ASSETS_DIR}/manifest.json"

OS="$(uname -s)"
case "${OS}" in
  Darwin) OS="darwin" ;;
  Linux) OS="linux" ;;
  *) OS="$(printf "%s" "${OS}" | tr '[:upper:]' '[:lower:]')" ;;
esac

ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported arch: ${ARCH}" >&2; exit 1 ;;
esac

mkdir -p "${ASSETS_DIR}"

download() {
  local url="$1"
  local output="$2"

  if command -v curl >/dev/null 2>&1; then
    curl -fL -o "${output}" "${url}"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -O "${output}" "${url}"
    return
  fi
  echo "curl or wget is required" >&2
  exit 1
}

render_url() {
  local template="$1"
  local tag="$2"
  local version="$3"

  printf "%s" "${template}" \
    | sed -e "s/{{tag}}/${tag}/g" \
          -e "s/{{version}}/${version}/g" \
          -e "s/{{os}}/${OS}/g" \
          -e "s/{{arch}}/${ARCH}/g"
}

download_tar_gz() {
  local name="$1"
  local tag="$2"
  local version="$3"
  local url_template="$4"
  local binary="$5"

  local url
  url="$(render_url "${url_template}" "${tag}" "${version}")"
  local filename
  filename="$(basename "${url}")"

  if [[ "${filename}" != *"${version}"* ]]; then
    echo "Asset name missing version (${version}): ${filename}" >&2
    exit 1
  fi

  local work
  work="$(mktemp -d)"
  trap 'rm -rf "${work}"' EXIT

  download "${url}" "${work}/${filename}"
  tar -xzf "${work}/${filename}" -C "${work}"

  local found
  found="$(find "${work}" -type f \( -name "${binary}" -o -name "${binary}.exe" \) | head -n 1)"
  if [[ -z "${found}" ]]; then
    echo "Binary ${binary} not found in ${filename}" >&2
    exit 1
  fi

  install -m 0755 "${found}" "${ASSETS_DIR}/${binary}"
  trap - EXIT
  rm -rf "${work}"
  echo "Downloaded ${name} -> ${ASSETS_DIR}/${binary}"
}

download_npm() {
  local package="$1"
  local tag="$2"
  local version="$3"
  local binary="$4"

  if ! command -v npm >/dev/null 2>&1; then
    echo "npm is required to download ${package}" >&2
    exit 1
  fi

  local work
  work="$(mktemp -d)"
  trap 'rm -rf "${work}"' EXIT

  local tarball
  tarball="$(printf "%s" "${package}" | sed -e 's/@//' -e 's|/|-|g')-${version}.tgz"
  (cd "${work}" && npm pack "${package}@${version}" >/dev/null)

  tar -xzf "${work}/${tarball}" -C "${work}"
  local source="${work}/package/bin/${binary}"
  if [[ ! -f "${source}" ]]; then
    echo "Binary ${binary} not found in npm package ${package}" >&2
    exit 1
  fi

  install -m 0755 "${source}" "${ASSETS_DIR}/${binary}"
  trap - EXIT
  rm -rf "${work}"
  echo "Downloaded ${package} -> ${ASSETS_DIR}/${binary}"
}

download_gem() {
  local gem_name="$1"
  local tag="$2"
  local version="$3"
  local binary="$4"

  if ! command -v gem >/dev/null 2>&1; then
    echo "gem is required to download ${gem_name}" >&2
    exit 1
  fi

  local work
  work="$(mktemp -d)"
  trap 'rm -rf "${work}"' EXIT

  (cd "${work}" && gem fetch "${gem_name}" -v "${version}" >/dev/null)
  (cd "${work}" && gem unpack "${gem_name}-${version}.gem" >/dev/null)

  local source="${work}/${gem_name}-${version}/bin/${binary}"
  if [[ ! -f "${source}" ]]; then
    echo "Binary ${binary} not found in gem ${gem_name}" >&2
    exit 1
  fi

  install -m 0755 "${source}" "${ASSETS_DIR}/${binary}"
  trap - EXIT
  rm -rf "${work}"
  echo "Downloaded ${gem_name} -> ${ASSETS_DIR}/${binary}"
}

download_maven() {
  local coordinate="$1"
  local tag="$2"
  local version="$3"
  local binary="$4"

  local group="${coordinate%%:*}"
  local artifact="${coordinate##*:}"
  local group_path
  group_path="$(printf "%s" "${group}" | tr '.' '/')"
  local jar="${artifact}-${version}.jar"
  local url="https://repo1.maven.org/maven2/${group_path}/${artifact}/${version}/${jar}"

  download "${url}" "${ASSETS_DIR}/${jar}"

  cat > "${ASSETS_DIR}/${binary}" <<EOF
#!/usr/bin/env bash
DIR="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
exec java -jar "\${DIR}/${jar}" "\$@"
EOF
  chmod 0755 "${ASSETS_DIR}/${binary}"
  echo "Downloaded ${coordinate} -> ${ASSETS_DIR}/${binary}"
}

TOOLS=(
  "scip-go|v0.1.26|tar.gz|https://github.com/sourcegraph/scip-go/releases/download/{{tag}}/scip-go_{{version}}_{{os}}_{{arch}}.tar.gz|scip-go"
  "scip-typescript|v0.3.13|npm|@sourcegraph/scip-typescript|scip-typescript"
  "scip-python|v0.3.9|npm|@sourcegraph/scip-python|scip-python"
  # "scip-java|0.11.2|maven|com.sourcegraph:scip-java_2.13|scip-java"
  # "scip-ruby|0.5.2|gem|scip-ruby|scip-ruby"
  # "scip-clang|v0.5.0|tar.gz|https://github.com/sourcegraph/scip-clang/releases/download/{{tag}}/scip-clang_{{version}}_{{os}}_{{arch}}.tar.gz|scip-clang"
  # "scip-dotnet|v0.2.0|tar.gz|https://github.com/sourcegraph/scip-dotnet/releases/download/{{tag}}/scip-dotnet_{{version}}_{{os}}_{{arch}}.tar.gz|scip-dotnet"
)

for entry in "${TOOLS[@]}"; do
  IFS="|" read -r name tag format source binary <<<"${entry}"
  version="${tag#v}"

  case "${format}" in
    tar.gz)
      download_tar_gz "${name}" "${tag}" "${version}" "${source}" "${binary}"
      ;;
    npm)
      download_npm "${source}" "${tag}" "${version}" "${binary}"
      ;;
    gem)
      download_gem "${source}" "${tag}" "${version}" "${binary}"
      ;;
    maven)
      download_maven "${source}" "${tag}" "${version}" "${binary}"
      ;;
    *)
      echo "Unknown format ${format} for ${name}" >&2
      exit 1
      ;;
  esac
done

python3 - <<'PY' "${ASSETS_DIR}" "${MANIFEST_PATH}"
import json
import os
import sys

assets_dir = sys.argv[1]
manifest_path = sys.argv[2]

def fnv1a64(data):
    h = 0xcbf29ce484222325
    for b in data:
        h ^= b
        h = (h * 0x100000001b3) & 0xFFFFFFFFFFFFFFFF
    return f"{h:016x}"

manifest = {}
for root, _, files in os.walk(assets_dir):
    for name in files:
        path = os.path.join(root, name)
        if os.path.abspath(path) == os.path.abspath(manifest_path):
            continue
        rel = os.path.relpath(path, assets_dir)
        with open(path, "rb") as handle:
            manifest[rel.replace(os.sep, "/")] = fnv1a64(handle.read())

with open(manifest_path, "w", encoding="utf-8") as handle:
    json.dump(dict(sorted(manifest.items())), handle, indent=2)
    handle.write("\n")
PY

echo "Wrote manifest ${MANIFEST_PATH}"
