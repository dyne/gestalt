#!/usr/bin/env bash
set -Eeuo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/gestalt-install-test.XXXXXXXX")
trap 'rm -rf -- "$test_root"' EXIT

# Exercise the HTTPS installer's stdin entry point with a recording npm.
mkdir -p "$test_root/tools"
cat > "$test_root/tools/npm" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@"
exit "${NPM_TEST_STATUS:-0}"
EOF
chmod +x "$test_root/tools/npm"
install_args=$(HOME="$test_root/user home" PATH="$test_root/tools:$PATH" bash < "$repo_root/public/install")
expected_args=$(printf '%s\n' install -g --prefix "$test_root/user home/.local" \
  --allow-scripts=@jaromil/gestalt @jaromil/gestalt@latest)
[[ $install_args == "$expected_args" ]]
status=0
PATH="$test_root/tools:$PATH" NPM_TEST_STATUS=42 bash < "$repo_root/public/install" >/dev/null || status=$?
[[ $status == 42 ]]

printf 'install.test: PASS\n'
