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
  python3 - "$stage" "$result" "$active_log" "$native_root/codex-rs/Cargo.lock" "$output_root/rust-toolchain.txt" <<'PY'
import json,pathlib,re,sys
stage,status,path,lock_path,toolchain_path=sys.argv[1:]
text=pathlib.Path(path).read_text(errors='replace') if path and pathlib.Path(path).exists() else ''
lock=pathlib.Path(lock_path).read_text() if pathlib.Path(lock_path).exists() else ''
toolchain=pathlib.Path(toolchain_path).read_text() if pathlib.Path(toolchain_path).exists() else ''
codes=sorted(set(re.findall(r'error\[(E[0-9]{4})\]',text)))[:8]
locations=sorted(set(re.findall(r'(?:src|tests/suite)/(?:bwrap|landlock)\.rs:[0-9]+:[0-9]+',text)))[:8]
patterns={'dependencyLoad':r'failed to (?:load source|get) (?:for )?dependency','sourceUpdate':r'failed to update','gitFetch':r'failed to (?:fetch|clone)|git fetch','missingRevision':r'revision .*not found','repositoryUnavailable':r'Repository not found|repository .*not found','network':r'network failure|connection|timed out|resolve host','tls':r'SSL|TLS|certificate','authentication':r'Authentication failed|could not read Username|Permission denied','lockfileChange':r'lock file.*needs to be updated|lockfile.*needs to be updated|cannot update.*lock.*--locked','registry':r'failed to download|failed to get.*crates.io|download of .*failed','rustVersion':r'requires rustc|not supported by.*rustc','manifest':r'failed to parse manifest|manifest.*invalid','patchPackageMissing':r'patch location.*does not contain','buildScript':r'failed to run custom build command','systemLibrary':r'libcap.*not.*found|pkg-config.*failed'}
classes=sorted(name for name,pattern in patterns.items() if re.search(pattern,text,re.I))
known_names=set(re.findall(r'^name = "([a-zA-Z0-9_.-]+)"$',lock,re.M))
dependencies=sorted({name for name in re.findall(r'(?:dependency|package) `([a-zA-Z0-9_.-]+)`',text) if name in known_names})[:12]
public_repositories=['openai-oss-forks/crossterm','openai-oss-forks/tokio-tungstenite','openai-oss-forks/tungstenite-rs','microsoft/mxc','modelcontextprotocol/rust-sdk','helix-editor/nucleo','dzbarsky/rules_rust','hyperium/h3']
repositories=[name for name in public_repositories if name in text]
known_revisions=set(re.findall(r'#[a-f0-9]{40}',lock))
revisions=sorted(value[1:] for value in known_revisions if value[1:] in text)[:12]
release=re.search(r'^release: ([0-9.]+)$',toolchain,re.M);host=re.search(r'^host: (x86_64-unknown-linux-gnu)$',toolchain,re.M)
metadata=re.search(r'NATIVE_METADATA_EXIT kind=(missing|empty-directory|directory|empty-file|file) exitCode=(-?[0-9]+) stderrClass=(EISDIR|ENOTDIR|EACCES|EROFS|ENOENT|unclassified) stdoutBytes=([0-9]+) stderrBytes=([0-9]+)',text)
metadata_failure={'fixtureKind':metadata[1],'exitCode':int(metadata[2]),'stderrClass':metadata[3],'stdoutBytes':int(metadata[4]),'stderrBytes':int(metadata[5]),'commandRole':{'11':'denied-file-read-succeeded','12':'denied-directory-list-succeeded','13':'denied-metadata-write-succeeded'}.get(metadata[2],'launch-or-command-failure')} if metadata else None
failed_tests=sorted(set(re.findall(r'^---- ([a-zA-Z_][a-zA-Z0-9_:]*) stdout ----$',text,re.M)))[:24]
test_results=re.findall(r'^test result: (?:ok|FAILED)\. [0-9]+ passed; [0-9]+ failed; [0-9]+ ignored; [0-9]+ measured; [0-9]+ filtered out',text,re.M)[:16]
print('::error title=Native mask candidate failure::'+json.dumps({'stage':stage,'exitCode':int(status),'compilerCodes':codes,'locations':locations,'cargoErrorClasses':classes,'dependencies':dependencies,'publicRepositories':repositories,'lockedRevisions':revisions,'httpStatuses':sorted(set(re.findall(r'\b(?:401|403|404|408|429|500|502|503)\b',text))),'rustRelease':release[1] if release else None,'host':host[1] if host else None,'failedTests':failed_tests,'testResults':test_results,'metadataFailure':metadata_failure,'logBytes':len(text.encode()),'diskFull':'No space left on device' in text,'outOfMemory':'SIGKILL' in text or 'signal: 9' in text}))
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
stage=prepare-release-lock
git -C "$native_root" apply --check "$fixture_root/lock-preparation.patch"
git -C "$native_root" apply "$fixture_root/lock-preparation.patch"
python3 - "$native_root" "$fixture_root" <<'PY'
import hashlib,json,pathlib,subprocess,sys,tomllib
root,fixture=map(pathlib.Path,sys.argv[1:]);m=json.loads((fixture/'manifest.json').read_text());pin=m['lockPreparation']
assert hashlib.sha256((fixture/'lock-preparation.patch').read_bytes()).hexdigest()==pin['patchSha256']
original=subprocess.check_output(['git','-C',str(root),'show',m['upstream']['commit']+':codex-rs/Cargo.lock'])
prepared=(root/'codex-rs/Cargo.lock').read_bytes()
assert hashlib.sha256(original).hexdigest()==pin['originalSha256']
assert hashlib.sha256(prepared).hexdigest()==pin['preparedSha256']
lock=tomllib.loads(original.decode());local={p['name']:p for p in lock['package'] if 'source' not in p};assert len(local)==pin['localPackageCount']
workspace=tomllib.loads((root/'codex-rs/Cargo.toml').read_text());assert workspace['workspace']['package']['version']==pin['toVersion']
versions={}
for path in (root/'codex-rs').rglob('Cargo.toml'):
 package=tomllib.loads(path.read_text()).get('package',{});name=package.get('name')
 if name in local:
  assert package['version']=={'workspace':True};versions[name]=pin['toVersion']
assert versions.keys()==local.keys()
for package in lock['package']:
 if 'source' not in package:
  assert package['version']==pin['fromVersion'];package['version']=versions[package['name']]
assert tomllib.loads(prepared.decode())==lock,'Lock preparation must only synchronize local workspace versions'
PY
git -C "$native_root" add -- codex-rs/Cargo.lock
prepared_tree=$(git -C "$native_root" write-tree)
expected_build_tree=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["lockPreparation"]["preparedBuildTree"])' "$fixture_root/manifest.json")
[[ $prepared_tree == "$expected_build_tree" ]]
export SOURCE_DATE_EPOCH
SOURCE_DATE_EPOCH=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["build"]["sourceDateEpoch"])' "$fixture_root/manifest.json")
export CARGO_BUILD_JOBS=2 CARGO_INCREMENTAL=0 CARGO_NET_GIT_FETCH_WITH_CLI=true
export CARGO_TARGET_DIR="$RUNNER_TEMP/native-mask-target"
export RUSTFLAGS="--remap-path-prefix=$native_root=/codex-native-mask-source"
cd "$native_root/codex-rs"
stage=verify-toolchain
active_log="$output_root/rust-toolchain.txt"
rustc --version --verbose > "$output_root/rust-toolchain.txt"
grep -Fx 'release: 1.95.0' "$output_root/rust-toolchain.txt" >/dev/null
grep -Fx 'host: x86_64-unknown-linux-gnu' "$output_root/rust-toolchain.txt" >/dev/null
cargo --version > "$output_root/cargo-toolchain.txt"
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
# The upstream private-/tmp test copies the sandbox away from its bundled
# resources. Keep the exact built helper reachable in that disposable namespace.
sudo install -m0755 "$output_root/bin/codex-resources/bwrap" /usr/local/bin/bwrap
[[ $(sha256sum /usr/local/bin/bwrap | cut -d' ' -f1) == "$CODEX_BWRAP_SHA256" ]]

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
cp "$fixture_root/LICENSE" "$fixture_root/native.patch" "$fixture_root/lock-preparation.patch" "$output_root/"
git -C "$native_root" diff --check
git -C "$manager_root" diff --check

stage=artifact-provenance
active_log=""
python3 - "$manager_root" "$native_root" "$fixture_root" "$output_root" <<'PY'
import hashlib,json,os,pathlib,re,subprocess,sys
manager,native,fixture,out=map(pathlib.Path,sys.argv[1:]);m=json.loads((fixture/'manifest.json').read_text())
assert subprocess.check_output(['git','-C',str(native),'write-tree'],text=True).strip()==m['lockPreparation']['preparedBuildTree']
for path,pin in m['inputs'].items():assert hashlib.sha256((native/path).read_bytes()).hexdigest()==(m['lockPreparation']['preparedSha256'] if path=='codex-rs/Cargo.lock' else pin),path
for path in ['codex-rs/linux-sandbox/src/bwrap.rs','codex-rs/linux-sandbox/tests/suite/landlock.rs']:
 assert (native/path).read_bytes()==subprocess.check_output(['git','-C',str(native),'show',m['patchedTree']+':'+path])
tests=(out/'logs/native-tests.log').read_text(errors='replace')
result={'schemaVersion':1,'managerHead':subprocess.check_output(['git','-C',str(manager),'rev-parse','HEAD'],text=True).strip(),'managerTree':subprocess.check_output(['git','-C',str(manager),'rev-parse','HEAD^{tree}'],text=True).strip(),'runId':os.environ['GITHUB_RUN_ID'],'upstream':m['upstream'],'patchedSourceTree':m['patchedTree'],'buildSourceTree':m['lockPreparation']['preparedBuildTree'],'lockPreparation':m['lockPreparation'],'patchSha256':m['patchSha256'],'fixPatchSha256':m['fixPatchSha256'],'build':m['build'],'declaredNativeVersion':'0.161.0','baselineRegression':'failed at intended directory-mask assertion','nativeTests':re.findall(r'test result: ok[^\n]*',tests),'binarySha256':hashlib.sha256((out/'bin/codex').read_bytes()).hexdigest(),'bwrapSha256':hashlib.sha256((out/'bin/codex-resources/bwrap').read_bytes()).hexdigest(),'sandboxHelperSha256':hashlib.sha256((out/'bin/codex-linux-sandbox').read_bytes()).hexdigest(),'sourceInputs':m['inputs'],'buildInputHashes':{name:hashlib.sha256((out/name).read_bytes()).hexdigest() for name in ['rust-toolchain.txt','cargo-toolchain.txt','c-compiler.txt','system-build-inputs.txt']},'runnerImageVersion':os.environ.get('ImageVersion'),'scope':'test-only GNU dev-small candidate; not a release/installation/activation','nativeOwningRuntimeProof':'pending; this build is not L9.2 acceptance'}
(out/'provenance.json').write_text(json.dumps(result,indent=2)+'\n')
PY
tar --sort=name --mtime="@$SOURCE_DATE_EPOCH" --owner=0 --group=0 --numeric-owner -C "$output_root" -czf "$RUNNER_TEMP/native-mask-candidate.tar.gz" bin LICENSE native.patch lock-preparation.patch provenance.json rust-toolchain.txt cargo-toolchain.txt c-compiler.txt system-build-inputs.txt
sha256sum "$RUNNER_TEMP/native-mask-candidate.tar.gz" > "$RUNNER_TEMP/native-mask-candidate.sha256"
python3 - "$output_root/provenance.json" "$RUNNER_TEMP/native-mask-candidate.tar.gz" <<'PY'
import hashlib,json,pathlib,sys
p=pathlib.Path(sys.argv[1]);result=json.loads(p.read_text());result['archiveSha256']=hashlib.sha256(pathlib.Path(sys.argv[2]).read_bytes()).hexdigest();p.write_text(json.dumps(result,indent=2)+'\n')
summary={key:result[key] for key in ['managerHead','managerTree','runId','upstream','patchedSourceTree','buildSourceTree','lockPreparation','patchSha256','build','baselineRegression','nativeTests','binarySha256','bwrapSha256','sandboxHelperSha256','archiveSha256','nativeOwningRuntimeProof']}
print('::notice title=Verified Native mask candidate::'+json.dumps(summary,separators=(',',':')))
PY
