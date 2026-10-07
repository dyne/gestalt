#!/usr/bin/env bash
set -Eeuo pipefail
repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/gestalt-xerj-test.XXXXXXXX")
test_root=$(CDPATH='' cd -- "$test_root" && pwd -P)
trap 'rm -rf -- "$test_root"' EXIT
mkdir -p "$test_root/bin" "$test_root/source" "$test_root/managed home"
export GESTALT_HOME="$test_root/managed home"
export CODEX_HOME="$GESTALT_HOME"
cd "$GESTALT_HOME"
export XERJ_TEST_SOURCE="$test_root/source"
export PATH="$test_root/bin:$PATH"
stage=xerj-1.0.0-rc.87-x86_64-unknown-linux-musl
mkdir -p "$test_root/source/$stage"
cat > "$test_root/source/$stage/xerj" <<'EOF'
#!/usr/bin/env python3
import json, os, pathlib, signal, sys
root=pathlib.Path(os.environ['XERJ_INSTALL_DIR'])
(root/'forwarded.json').write_text(json.dumps({'args':sys.argv[1:], 'env':dict(os.environ)}))
if sys.argv[1:2] == ['mcp']:
    print('mcp fixture diagnostics', file=sys.stderr)
    for line in sys.stdin:
        request=json.loads(line)
        if 'id' in request:
            print(json.dumps({'jsonrpc':'2.0','id':request['id'],'result':{}}), flush=True)
    sys.exit(0)
if 'wait-for-signal' in sys.argv:
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(42))
    (root/'waiting').touch()
    signal.pause()
else:
    print(sys.stdin.read(), end='')
    print('xerj stderr', file=sys.stderr)
    sys.exit(23 if 'fail' in sys.argv else 0)
EOF
chmod 0755 "$test_root/source/$stage/xerj"
tar -czf "$test_root/source/release.tar.gz" -C "$test_root/source" "$stage"
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "$test_root/source/release.tar.gz" > "$test_root/source/release.sha256"
else
  shasum -a 256 "$test_root/source/release.tar.gz" > "$test_root/source/release.sha256"
fi
cat > "$test_root/bin/uname" <<'EOF'
#!/usr/bin/env bash
case $1 in -s) printf 'Linux\n';; -m) printf 'x86_64\n';; *) exit 1;; esac
EOF
cat > "$test_root/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ $1 == -q ]]
url='' destination=''
while (($#)); do
  case $1 in
    https://*) url=$1; shift ;;
    --output) destination=$2; shift 2 ;;
    *) shift ;;
  esac
done
[[ $destination == "$GESTALT_HOME/xerj/.install."* ]]
[[ $url == https://github.com/xerj-org/xerj/releases/download/v1.0.0-rc.87/* ]]
[[ ${XERJ_TEST_FAIL_DOWNLOAD:-0} == 0 ]] || exit 22
if [[ ${XERJ_TEST_INTERRUPT_DOWNLOAD:-0} == 1 ]]; then
  kill -TERM "$PPID"
  exit 0
fi
case $url in
  *.sha256) cp "$XERJ_TEST_SOURCE/release.sha256" "$destination" ;;
  *) cp "$XERJ_TEST_SOURCE/release.tar.gz" "$destination" ;;
esac
EOF
chmod 0755 "$test_root/bin/"*
manager=(bash "$repo_root/public/gestalt")
assertions=0
check() { "$@"; assertions=$((assertions + 1)); }
refuse() {
  if "${manager[@]}" xerj "$@" > "$test_root/refusal.out" 2>&1; then
    printf 'unexpected xerj success: %s\n' "$*" >&2
    exit 1
  fi
  assertions=$((assertions + 1))
}
refuse --version
"${manager[@]}" xerj install > "$test_root/install.out" 2>&1
check test -x "$GESTALT_HOME/xerj/xerj"
check test ! -e "$CODEX_HOME/xerj-data"
check test -z "$(find "$GESTALT_HOME/xerj" -name '.install.*' -print)"
check test "$(find "$GESTALT_HOME" -type f | wc -l | tr -d ' ')" = 1
"${manager[@]}" xerj -V </dev/null > /dev/null 2> /dev/null
check test ! -e "$CODEX_HOME/xerj-data/managed.toml"
cp "$GESTALT_HOME/xerj/xerj" "$test_root/prior"
XERJ_TEST_FAIL_DOWNLOAD=1 refuse install
check cmp "$test_root/prior" "$GESTALT_HOME/xerj/xerj"
XERJ_TEST_INTERRUPT_DOWNLOAD=1 refuse install
check cmp "$test_root/prior" "$GESTALT_HOME/xerj/xerj"
check test -z "$(find "$GESTALT_HOME/xerj" -name '.install.*' -print)"
cp "$test_root/source/release.sha256" "$test_root/good-checksum"
printf '%064d  release.tar.gz\n' 0 > "$test_root/source/release.sha256"
refuse install
check cmp "$test_root/prior" "$GESTALT_HOME/xerj/xerj"
check test -z "$(find "$GESTALT_HOME/xerj" -name '.install.*' -print)"
cp "$test_root/good-checksum" "$test_root/source/release.sha256"
printf 'not a checksum\n' > "$test_root/source/release.sha256"
refuse install
check cmp "$test_root/prior" "$GESTALT_HOME/xerj/xerj"
check test -z "$(find "$GESTALT_HOME/xerj" -name '.install.*' -print)"
cp "$test_root/good-checksum" "$test_root/source/release.sha256"
# Inherited archive options must not change extraction behavior or launch tools.
TAR_OPTIONS="--to-command=touch $test_root/tar-escaped" GZIP='--invalid-option' \
  "${manager[@]}" xerj install > /dev/null 2>&1
check test ! -e "$test_root/tar-escaped"
printf 'foreign evidence\n' > "$GESTALT_HOME/xerj/foreign"
"${manager[@]}" xerj install > /dev/null 2>&1 &
first=$!
"${manager[@]}" xerj install > /dev/null 2>&1 &
second=$!
wait "$first"
wait "$second"
check test "$(cat "$GESTALT_HOME/xerj/foreign")" = 'foreign evidence'
check test -z "$(find "$GESTALT_HOME/xerj" -name '.install.*' -print)"
cp "$test_root/source/release.tar.gz" "$test_root/good-release"
make_checksum() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$test_root/source/release.tar.gz" > "$test_root/source/release.sha256"
  else
    shasum -a 256 "$test_root/source/release.tar.gz" > "$test_root/source/release.sha256"
  fi
}
printf 'corrupt archive\n' > "$test_root/source/release.tar.gz"
make_checksum
refuse install
check cmp "$test_root/prior" "$GESTALT_HOME/xerj/xerj"
mv "$test_root/source/$stage/xerj" "$test_root/stub-binary"
ln -s "$test_root/prior" "$test_root/source/$stage/xerj"
tar -czf "$test_root/source/release.tar.gz" -C "$test_root/source" "$stage"
make_checksum
refuse install
check cmp "$test_root/prior" "$GESTALT_HOME/xerj/xerj"
rm "$test_root/source/$stage/xerj"
mv "$test_root/stub-binary" "$test_root/source/$stage/xerj"
tar -czf "$test_root/source/release.tar.gz" -C "$test_root/source" "$stage/xerj" "$stage/xerj"
make_checksum
refuse install
check cmp "$test_root/prior" "$GESTALT_HOME/xerj/xerj"
cp "$test_root/good-release" "$test_root/source/release.tar.gz"
cp "$test_root/good-checksum" "$test_root/source/release.sha256"
XERJ_INSECURE_SKIP_CHECKSUM=1 refuse install
XERJ_INSTALL_DIR="$test_root/escape" refuse install
XERJ_VERSION=unreviewed refuse install
refuse --data-dir "$test_root/escape"
refuse -d "$test_root/escape"
refuse --config "$test_root/escape"
refuse --data-dir="$test_root/escape"
XERJ_CONFIG="$test_root/escape" refuse --version
XERJ_INGEST_MEMORY_OUTPUT="$test_root/escape" refuse --version
refuse autoindex "$test_root/source" --state-dir "$test_root/escape"
refuse autoindex "$test_root/source" --events-out "$test_root/escape"
refuse autoindex "$test_root/source" --status-file "$test_root/escape"
for command in init service export brain share feedback code corpus; do refuse "$command"; done
for address in 0.0.0.0 :: 192.168.1.2 public.example localhost; do
  refuse --bind "$address"
  refuse -b "$address"
  XERJ_BIND_ADDRESS="$address" refuse --version
done
for url in https://localhost:9200 http://0.0.0.0:9200 http://localhost:9200@evil.example \
  http://localhost.evil.example:9200 http://127.1:9200 http://2130706433:9200 \
  http://127.0.0.1:0 http://127.0.0.1:65536 http://127.0.0.1:9200/path \
  'http://127.0.0.1:9200?url=http://evil.example' 'http://[::]:9200' 'http://[::1%25zone]:9200'; do
  refuse mcp --url "$url"
  refuse autoindex "$test_root/source" --url "$url"
  XERJ_URL="$url" refuse search fixture
done
refuse mcp --auth ''
refuse search fixture -k
refuse search fixture -n 5
mkdir -p "$CODEX_HOME/xerj-data"
ln -s "$test_root/source" "$CODEX_HOME/xerj-data/escape"
refuse --version
rm "$CODEX_HOME/xerj-data/escape"
GESTALT_HOME="$test_root/managed home/../escape" refuse install
ln -s "$GESTALT_HOME" "$test_root/alias"
GESTALT_HOME="$test_root/alias" "${manager[@]}" xerj --version </dev/null > /dev/null 2> /dev/null
assertions=$((assertions + 1))
mv "$GESTALT_HOME/xerj" "$GESTALT_HOME/prior-xerj"
ln -s "$GESTALT_HOME/prior-xerj" "$GESTALT_HOME/xerj"
refuse install
rm "$GESTALT_HOME/xerj"
mv "$GESTALT_HOME/prior-xerj" "$GESTALT_HOME/xerj"
printf 'stdin with spaces\n' | "${manager[@]}" xerj autoindex "$test_root/source" --dry-run --json \
  > "$test_root/stdout" 2> "$test_root/stderr"
check test "$(cat "$test_root/stdout")" = 'stdin with spaces'
check test "$(cat "$test_root/stderr")" = 'xerj stderr'
real_mv=$(command -v mv)
cat > "$test_root/bin/mv" <<EOF
#!/usr/bin/env bash
set -euo pipefail
if [[ \${XERJ_TEST_INTERRUPT_CONFIG:-0} == 1 ]]; then
  kill -TERM "\$PPID"
  exit 0
fi
exec "$real_mv" "\$@"
EOF
chmod 0755 "$test_root/bin/mv"
XERJ_TEST_INTERRUPT_CONFIG=1 refuse --help
check test -z "$(find "$CODEX_HOME/xerj-data" -name '.config.*' -print)"
rm "$test_root/bin/mv"
python3 - "$GESTALT_HOME" "$HOME" <<'PY'
import hashlib, json, pathlib, sys
root=pathlib.Path(sys.argv[1]); record=json.loads((root/'xerj/forwarded.json').read_text())
prefix='ax-'+hashlib.sha256(str(root.parent/'source').encode()).hexdigest()[:16]
assert record['args']==['autoindex','--state-dir',str(root/'xerj-data/autoindex'/prefix),
                        str(root.parent/'source'),'--dry-run','--json','--prefix',prefix], record['args']
env=record['env']; assert env['HOME']==sys.argv[2]
for key in ['TMPDIR','XDG_CACHE_HOME','XDG_CONFIG_HOME','XDG_DATA_HOME','HF_HOME']:
    assert env[key].startswith(str(root/'xerj-data')+'/'), (key,env[key])
assert env['GESTALT_HOME']==str(root)
assert env['XERJ_INSTALL_DIR']==str(root/'xerj')
PY
assertions=$((assertions + 1))
"${manager[@]}" xerj -- --version </dev/null > /dev/null 2> /dev/null
assertions=$((assertions + 1))
python3 - "$GESTALT_HOME" <<'PY'
import json, pathlib, sys
root = pathlib.Path(sys.argv[1])
record = json.loads((root/'xerj/forwarded.json').read_text())
assert record['args'] == ['-V'], record['args']
assert 'XERJ_API_KEY' not in record['env']
assert 'XERJ_CONFIG' not in record['env']
PY
assertions=$((assertions + 1))
"${manager[@]}" xerj index --index fixture --file "$test_root/source/input.ndjson" </dev/null > /dev/null 2> /dev/null
python3 - "$GESTALT_HOME" <<'PY'
import json,pathlib,sys
root=pathlib.Path(sys.argv[1]); r=json.loads((root/'xerj/forwarded.json').read_text())
assert r['args'][:5]==['index','--config',str(root/'xerj-data/managed.toml'),
                      '--data-dir',str(root/'xerj-data')]
PY
assertions=$((assertions + 1))
XERJ_URL=http://localhost:9370 XERJ_API_KEY=fixture-key \
  "${manager[@]}" xerj search 'query with spaces' -k 5 --json </dev/null > /dev/null 2> /dev/null
python3 - "$GESTALT_HOME" <<'PY'
import json, pathlib, sys
r=json.loads((pathlib.Path(sys.argv[1])/'xerj/forwarded.json').read_text())
assert r['args']==['search','query with spaces','-k','5','--json']
assert r['env']['XERJ_URL']=='http://127.0.0.1:9370'
assert r['env']['XERJ_API_KEY']=='fixture-key'
PY
assertions=$((assertions + 1))
"${manager[@]}" xerj --bind ::1 --version </dev/null > /dev/null 2> /dev/null
python3 - "$GESTALT_HOME" <<'PY'
import json,pathlib,sys
r=json.loads((pathlib.Path(sys.argv[1])/'xerj/forwarded.json').read_text())
assert r['args'][-3:]==['--bind','::1','--version']
assert '--bind' in r['args']
PY
assertions=$((assertions + 1))
printf 'managed-fixture-key\n' > "$CODEX_HOME/xerj-data/admin.key"
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize"}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | \
  "${manager[@]}" xerj mcp --url http://localhost:9370 > "$test_root/mcp.out" 2> "$test_root/mcp.err"
python3 - "$GESTALT_HOME" "$test_root/mcp.out" <<'PY'
import json,pathlib,sys
messages=[json.loads(line) for line in pathlib.Path(sys.argv[2]).read_text().splitlines()]
assert [m['id'] for m in messages]==[1,2]
r=json.loads((pathlib.Path(sys.argv[1])/'xerj/forwarded.json').read_text())
assert r['args']==['mcp','--url','http://127.0.0.1:9370']
assert r['env']['XERJ_AUTH']=='ApiKey managed-fixture-key'
assert r['env']['XERJ_CODE_HOME']==str(pathlib.Path(sys.argv[1])/'xerj-data/code')
PY
assertions=$((assertions + 1))
check test "$(cat "$test_root/mcp.err")" = 'mcp fixture diagnostics'
XERJ_AUTH='Bearer explicit-fixture-key' "${manager[@]}" xerj mcp --url 'http://[::1]:9370' </dev/null > "$test_root/mcp-empty.out" 2> /dev/null
check test ! -s "$test_root/mcp-empty.out"
python3 - "$GESTALT_HOME" <<'PY'
import json,pathlib,sys
r=json.loads((pathlib.Path(sys.argv[1])/'xerj/forwarded.json').read_text())
assert r['env']['XERJ_AUTH']=='Bearer explicit-fixture-key'
assert r['args']==['mcp','--url','http://[::1]:9370']
PY
assertions=$((assertions + 1))
set +e
"${manager[@]}" xerj search fail </dev/null > /dev/null 2> /dev/null
status=$?
set -e
check test "$status" = 23
"${manager[@]}" xerj search wait-for-signal </dev/null > /dev/null 2> /dev/null &
child=$!
trap 'kill -TERM "$child" 2>/dev/null || true; wait "$child" 2>/dev/null || true; rm -rf -- "$test_root"' EXIT
for _ in {1..100}; do
  [[ ! -f $GESTALT_HOME/xerj/waiting ]] || break
  sleep 0.02
done
check test -f "$GESTALT_HOME/xerj/waiting"
kill -TERM "$child"
set +e
wait "$child"
status=$?
set -e
check test "$status" = 42
python3 - "$repo_root/public/gestalt" "$GESTALT_HOME" "$test_root" <<'PY'
import json, os, pathlib, subprocess, sys
manager, home, temporary = map(pathlib.Path, sys.argv[1:])
sources = [temporary/'first'/'repo', temporary/'second'/'repo']
for source in sources: source.mkdir(parents=True)
def run(cwd, *args):
    subprocess.run(['bash', str(manager), 'xerj', *map(str, args)], cwd=cwd,
                   stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                   stderr=subprocess.DEVNULL, check=True)
    return json.loads((home/'xerj/forwarded.json').read_text())
first = run(sources[0], 'autoindex', sources[0], '--dry-run')
again = run(sources[1], 'autoindex', sources[0], '--dry-run')
second = run(sources[1], 'autoindex', sources[1], '--dry-run')
def prefix(record): return record['args'][record['args'].index('--prefix')+1]
assert prefix(first) == prefix(again)
assert prefix(first) != prefix(second)
assert first['args'][2] != second['args'][2]
named = run(sources[1], 'autoindex', '--cwd', sources[0], '--dry-run')
relative = run(temporary, 'autoindex', '--cwd=first/repo', '--dry-run')
assert named['args'] == first['args'] == relative['args']
spaced = temporary/'source with spaces'
spaced.mkdir()
assert str(spaced) in run(temporary, 'autoindex', '--cwd', spaced, '--dry-run')['args']
for args in [('--cwd',), ('--cwd', ''), ('--cwd', '--watch'),
             ('--cwd', 'missing'), ('--cwd=',),
             ('--cwd', sources[0], sources[1]),
             (sources[0], '--cwd', sources[1]),
             ('--cwd', sources[0], '--cwd', sources[0]),
             ('map', '--cwd', sources[0])]:
    result = subprocess.run(['bash', str(manager), 'xerj', 'autoindex', *map(str, args)],
                            cwd=temporary, capture_output=True, text=True)
    assert result.returncode != 0 and '--cwd' in result.stderr, (args, result.stderr)
for cwd in sources:
    result = run(cwd, 'search', 'prior art', '-k', '5', '--json')
    assert result['env']['XERJ_URL'] == 'http://127.0.0.1:9200'
    assert result['env']['XERJ_CODE_HOME'] == str(home/'xerj-data/code')
    assert not (cwd/'.gestalt').exists()
explicit = run(sources[1], 'autoindex', sources[0], '--prefix', 'prior-art', '--dry-run')
assert prefix(explicit) == 'prior-art'
assert explicit['args'][2] == str(home/'xerj-data/autoindex/prior-art')
PY
assertions=$((assertions + 1))
printf 'xerj: %s assertions passed (install, failures, confinement, forwarding, signals)\n' "$assertions"

# The native journal is unlocked between watch passes. The wrapper's kernel
# lock must exclude a second watcher for its entire lifetime and survive cwd changes.
python3 - "$repo_root/public/gestalt" <<'PY'
import os, pathlib, subprocess, sys, time
manager=sys.argv[1]
root=pathlib.Path(os.environ['GESTALT_HOME'])
marker=root/'xerj/waiting'
marker.unlink(missing_ok=True)
args=[manager,'xerj','autoindex',str(root),'--watch','--no-graph']
owner=subprocess.Popen(args+['--stub','wait-for-signal'],stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
try:
    deadline=time.monotonic()+5
    while not marker.exists() and time.monotonic()<deadline: time.sleep(.02)
    assert marker.exists(), 'watch owner failed to start'
    second=subprocess.run(args,stdin=subprocess.DEVNULL,capture_output=True,timeout=5,cwd='/tmp')
    assert second.returncode==75, (second.returncode,second.stderr)
finally:
    owner.terminate()
    owner.communicate(timeout=5)
assert owner.returncode==42, owner.returncode
resumed=subprocess.run(args,stdin=subprocess.DEVNULL,capture_output=True,timeout=5)
assert resumed.returncode==0, resumed.stderr
marker.unlink(missing_ok=True)
parent=subprocess.Popen([sys.executable,'-c',
    'import subprocess,sys,time; p=subprocess.Popen(sys.argv[1:],stdin=subprocess.DEVNULL,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL); print(p.pid,flush=True); time.sleep(30)',
    *args,'--stub','wait-for-signal'],stdout=subprocess.PIPE,text=True)
child_pid=int(parent.stdout.readline())
try:
    deadline=time.monotonic()+5
    while not marker.exists() and time.monotonic()<deadline: time.sleep(.02)
    assert marker.exists(), 'parent-death fixture failed to start'
    parent.kill(); parent.wait(timeout=5)
    def child_alive():
        try: return pathlib.Path(f'/proc/{child_pid}/stat').read_text().split(') ')[1][0] != 'Z'
        except FileNotFoundError: return False
    deadline=time.monotonic()+5
    while child_alive() and time.monotonic()<deadline: time.sleep(.02)
    assert not child_alive(), 'watcher survived its host owner'
finally:
    if parent.poll() is None: parent.kill(); parent.wait(timeout=5)
print('xerj: watcher ownership, lock release and parent-death cleanup passed')
PY
