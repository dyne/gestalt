#!/usr/bin/env bash
set -Eeuo pipefail

manager_root=$(pwd -P)
native_root=$(realpath .native-mask-input)
fixture_root="$manager_root/tests/fixtures/live-host-native-mask"
output_root="$RUNNER_TEMP/native-mask-candidate"
mkdir -p "$output_root/logs" "$output_root/bin/codex-resources"
stage=verify-source
active_log=""
report_failure() {
  local result=$?
  trap - ERR
  python3 - "$stage" "$result" "$active_log" <<'PY'
import json,pathlib,re,sys
stage,status,path=sys.argv[1:]
text=pathlib.Path(path).read_text(errors='replace') if path and pathlib.Path(path).exists() else ''
codes=sorted(set(re.findall(r'error\[(E[0-9]{4})\]',text)))[:8]
locations=sorted(set(re.findall(r'(?:src|tests/suite)/(?:bwrap|landlock)\.rs:[0-9]+:[0-9]+',text)))[:8]
print('::error title=Native mask candidate failure::'+json.dumps({'stage':stage,'exitCode':int(status),'compilerCodes':codes,'locations':locations,'logBytes':len(text.encode()),'diskFull':'No space left on device' in text,'outOfMemory':'SIGKILL' in text or 'signal: 9' in text}))
PY
  exit "$result"
}
trap report_failure ERR

python3 - "$native_root" "$fixture_root" <<'PY'
import hashlib,json,pathlib,subprocess,sys
root,fixture=map(pathlib.Path,sys.argv[1:]);m=json.loads((fixture/'manifest.json').read_text())
assert subprocess.check_output(['git','-C',str(root),'rev-parse','HEAD'],text=True).strip()==m['upstream']['commit']
assert subprocess.check_output(['git','-C',str(root),'rev-parse','HEAD^{tree}'],text=True).strip()==m['upstream']['tree']
assert not subprocess.check_output(['git','-C',str(root),'status','--porcelain'],text=True).strip()
for path,pin in m['inputs'].items():assert hashlib.sha256((root/path).read_bytes()).hexdigest()==pin,path
assert hashlib.sha256((fixture/'native.patch').read_bytes()).hexdigest()==m['patchSha256']
assert hashlib.sha256((fixture/'fix.patch').read_bytes()).hexdigest()==m['fixPatchSha256']
PY
git -C "$native_root" apply --check "$fixture_root/native.patch"
git -C "$native_root" apply "$fixture_root/native.patch"
git -C "$native_root" diff --check
git -C "$native_root" add -- codex-rs/linux-sandbox/src/bwrap.rs codex-rs/linux-sandbox/tests/suite/landlock.rs
patched_tree=$(git -C "$native_root" write-tree)
expected_tree=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["patchedTree"])' "$fixture_root/manifest.json")
[[ $patched_tree == "$expected_tree" ]]
export SOURCE_DATE_EPOCH
SOURCE_DATE_EPOCH=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["build"]["sourceDateEpoch"])' "$fixture_root/manifest.json")
export CARGO_BUILD_JOBS=2 CARGO_INCREMENTAL=0 CARGO_NET_GIT_FETCH_WITH_CLI=true
export CARGO_TARGET_DIR="$RUNNER_TEMP/native-mask-target"
export RUSTFLAGS="--remap-path-prefix=$native_root=/codex-native-mask-source"
cd "$native_root/codex-rs"
rustc --version --verbose > "$output_root/rust-toolchain.txt"
gcc --version > "$output_root/c-compiler.txt"
dpkg-query -W -f='${Package} ${Version}\n' build-essential binutils pkg-config libcap-dev libasound2-dev > "$output_root/system-build-inputs.txt"

stage=build-bwrap
active_log="$output_root/logs/bwrap.log"
cargo build --locked --profile dev-small -p codex-bwrap > "$active_log" 2>&1
install -m0755 "$CARGO_TARGET_DIR/dev-small/bwrap" "$output_root/bin/codex-resources/bwrap"
export CODEX_BWRAP_SHA256
CODEX_BWRAP_SHA256=$(sha256sum "$output_root/bin/codex-resources/bwrap" | cut -d' ' -f1)
mkdir -p "$CARGO_TARGET_DIR/dev-small/codex-resources"
install -m0755 "$output_root/bin/codex-resources/bwrap" "$CARGO_TARGET_DIR/dev-small/codex-resources/bwrap"

stage=baseline-regression
active_log="$output_root/logs/baseline.log"
git -C "$native_root" apply --reverse "$fixture_root/fix.patch"
if cargo test --locked --profile dev-small -p codex-linux-sandbox --lib denied_missing_protected_metadata_keeps_directory_mask > "$active_log" 2>&1; then
  printf '%s\n' 'Baseline unexpectedly passed the new metadata regression' >&2
  false
else
  baseline_exit=$?
fi
[[ $baseline_exit == 101 ]]
grep -F 'denied_missing_protected_metadata_keeps_directory_mask ... FAILED' "$active_log" >/dev/null
grep -F 'denied metadata must use an unreadable directory mask' "$active_log" >/dev/null
git -C "$native_root" apply "$fixture_root/fix.patch"

stage=native-regressions
active_log="$output_root/logs/native-tests.log"
cargo test --locked --profile dev-small -p codex-linux-sandbox -- --test-threads=2 > "$active_log" 2>&1
grep -F 'sandbox_enforces_denied_metadata_directory_and_file_types ... ok' "$active_log" >/dev/null
grep -F 'denied_missing_protected_metadata_keeps_directory_mask ... ok' "$active_log" >/dev/null

stage=build-native
active_log="$output_root/logs/native-build.log"
cargo build --locked --profile dev-small -p codex-cli -p codex-linux-sandbox > "$active_log" 2>&1
install -m0755 "$CARGO_TARGET_DIR/dev-small/codex" "$output_root/bin/codex"
install -m0755 "$CARGO_TARGET_DIR/dev-small/codex-linux-sandbox" "$output_root/bin/codex-linux-sandbox"
cp "$fixture_root/LICENSE" "$fixture_root/native.patch" "$output_root/"
git -C "$native_root" diff --check
git -C "$manager_root" diff --check

stage=artifact-provenance
active_log=""
python3 - "$manager_root" "$native_root" "$fixture_root" "$output_root" <<'PY'
import hashlib,json,os,pathlib,re,subprocess,sys
manager,native,fixture,out=map(pathlib.Path,sys.argv[1:]);m=json.loads((fixture/'manifest.json').read_text())
assert subprocess.check_output(['git','-C',str(native),'write-tree'],text=True).strip()==m['patchedTree']
for path,pin in m['inputs'].items():assert hashlib.sha256((native/path).read_bytes()).hexdigest()==pin,path
for path in ['codex-rs/linux-sandbox/src/bwrap.rs','codex-rs/linux-sandbox/tests/suite/landlock.rs']:
 assert (native/path).read_bytes()==subprocess.check_output(['git','-C',str(native),'show',m['patchedTree']+':'+path])
tests=(out/'logs/native-tests.log').read_text(errors='replace')
result={'schemaVersion':1,'managerHead':subprocess.check_output(['git','-C',str(manager),'rev-parse','HEAD'],text=True).strip(),'managerTree':subprocess.check_output(['git','-C',str(manager),'rev-parse','HEAD^{tree}'],text=True).strip(),'runId':os.environ['GITHUB_RUN_ID'],'upstream':m['upstream'],'patchedSourceTree':m['patchedTree'],'patchSha256':m['patchSha256'],'fixPatchSha256':m['fixPatchSha256'],'build':m['build'],'declaredNativeVersion':'0.161.0','baselineRegression':'failed at intended directory-mask assertion','nativeTests':re.findall(r'test result: ok[^\n]*',tests),'binarySha256':hashlib.sha256((out/'bin/codex').read_bytes()).hexdigest(),'bwrapSha256':hashlib.sha256((out/'bin/codex-resources/bwrap').read_bytes()).hexdigest(),'sandboxHelperSha256':hashlib.sha256((out/'bin/codex-linux-sandbox').read_bytes()).hexdigest(),'sourceInputs':m['inputs'],'buildInputHashes':{name:hashlib.sha256((out/name).read_bytes()).hexdigest() for name in ['rust-toolchain.txt','c-compiler.txt','system-build-inputs.txt']},'runnerImageVersion':os.environ.get('ImageVersion'),'scope':'test-only GNU dev-small candidate; not a release/installation/activation','nativeOwningRuntimeProof':'pending; this build is not L9.2 acceptance'}
(out/'provenance.json').write_text(json.dumps(result,indent=2)+'\n')
PY
tar --sort=name --mtime="@$SOURCE_DATE_EPOCH" --owner=0 --group=0 --numeric-owner -C "$output_root" -czf "$RUNNER_TEMP/native-mask-candidate.tar.gz" bin LICENSE native.patch provenance.json rust-toolchain.txt c-compiler.txt system-build-inputs.txt
sha256sum "$RUNNER_TEMP/native-mask-candidate.tar.gz" > "$RUNNER_TEMP/native-mask-candidate.sha256"
python3 - "$output_root/provenance.json" "$RUNNER_TEMP/native-mask-candidate.tar.gz" <<'PY'
import hashlib,json,pathlib,sys
p=pathlib.Path(sys.argv[1]);result=json.loads(p.read_text());result['archiveSha256']=hashlib.sha256(pathlib.Path(sys.argv[2]).read_bytes()).hexdigest();p.write_text(json.dumps(result,indent=2)+'\n')
summary={key:result[key] for key in ['managerHead','managerTree','runId','upstream','patchedSourceTree','patchSha256','build','baselineRegression','nativeTests','binarySha256','bwrapSha256','sandboxHelperSha256','archiveSha256','nativeOwningRuntimeProof']}
print('::notice title=Verified Native mask candidate::'+json.dumps(summary,separators=(',',':')))
PY
