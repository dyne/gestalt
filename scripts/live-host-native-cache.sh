#!/usr/bin/env bash
set -Eeuo pipefail

native_root=$(realpath .native-mask-input)
output_root="${RUNNER_TEMP:?}/native-mask-candidate"
mkdir -p "$output_root/cache-logs"

case ${1:-} in
  configure)
    command -v ccache >/dev/null
    cache_root="$RUNNER_TEMP/native-mask-ccache"
    wrapper_root="$RUNNER_TEMP/native-mask-compilers"
    mkdir -p "$cache_root" "$wrapper_root" "$RUNNER_TEMP/native-mask-cargo" "$RUNNER_TEMP/native-mask-target"
    chmod 700 "$cache_root" "$wrapper_root"
    for compiler in gcc g++; do
      resolved=$(command -v "$compiler")
      printf '#!/bin/sh\nset -eu\nexec /usr/bin/ccache "%s" "$@"\n' "$resolved" > "$wrapper_root/$compiler"
      chmod 700 "$wrapper_root/$compiler"
    done
    {
      printf 'CARGO_HOME=%s\nCARGO_TARGET_DIR=%s\n' "$RUNNER_TEMP/native-mask-cargo" "$RUNNER_TEMP/native-mask-target"
      printf 'CCACHE_DIR=%s\nCCACHE_BASEDIR=%s\nCCACHE_COMPILERCHECK=content\n' "$cache_root" "$native_root"
      printf 'CCACHE_LOGFILE=%s\nCC=%s\nCXX=%s\n' "$output_root/cache-logs/ccache.log" "$wrapper_root/gcc" "$wrapper_root/g++"
      printf 'CARGO_BUILD_JOBS=2\nCARGO_INCREMENTAL=0\nCARGO_NET_GIT_FETCH_WITH_CLI=true\n'
      printf 'RUSTFLAGS=--remap-path-prefix=%s=/codex-native-mask-source\n' "$native_root"
    } >> "${GITHUB_ENV:?}"
    python3 - "$native_root" "$output_root" <<'PY'
import hashlib,json,os,pathlib,shutil,subprocess,sys
native,out=map(pathlib.Path,sys.argv[1:]);fixture=pathlib.Path('tests/fixtures/live-host-native-mask');manifest=json.loads((fixture/'manifest.json').read_text())
def digest(path):
 with pathlib.Path(path).open('rb') as stream:return hashlib.file_digest(stream,'sha256').hexdigest()
def command(*args):return subprocess.check_output(args,text=True).strip()
assert command('git','-C',str(native),'rev-parse','HEAD')==manifest['upstream']['commit']
tools={name:{'binary':digest(pathlib.Path(shutil.which(name)).resolve()),'version':command(name,'--version').splitlines()[0]} for name in ['gcc','g++','ccache']}
packages=command('dpkg-query','-W','-f=${Package} ${Version}\n')
toolchain={'rust':command('rustc','--version','--verbose'),'cargo':command('cargo','--version'),'tools':tools,'packagesSha256':hashlib.sha256(packages.encode()).hexdigest(),'image':os.environ.get('ImageVersion'),'runnerOs':os.environ['RUNNER_OS'],'runnerArch':os.environ['RUNNER_ARCH']}
inputs={str(p):digest(p) for p in [fixture/'manifest.json',fixture/'native.patch',fixture/'fix.patch',fixture/'lock-preparation.patch',pathlib.Path('scripts/live-host-native-mask-build.sh'),pathlib.Path('scripts/live-host-native-cache.sh'),pathlib.Path('.github/workflows/native-mask-candidate.yml')]}
context={'schemaVersion':1,'repository':os.environ['GITHUB_REPOSITORY'],'ref':os.environ['GITHUB_REF'],'toolchain':toolchain,'target':manifest['build']['target'],'profile':manifest['build']['profile'],'features':['codex-cli/default','codex-linux-sandbox/default','codex-bwrap/default'],'jobs':2,'incremental':False,'rustflags':'--remap-path-prefix=<native>=/codex-native-mask-source','sourceDateEpoch':manifest['build']['sourceDateEpoch'],'upstream':manifest['upstream'],'patchedTree':manifest['patchedTree'],'preparedLock':manifest['lockPreparation']['preparedSha256'],'inputs':inputs,'ccache':{'compilerCheck':'content','sloppiness':[]},'v8':'unchanged signed prebuilt host; not compiled'}
downloads={'schemaVersion':1,'repository':context['repository'],'ref':context['ref'],'toolchain':toolchain,'target':context['target'],'upstream':manifest['upstream'],'lock':context['preparedLock']}
def key(value):return hashlib.sha256(json.dumps(value,sort_keys=True,separators=(',',':')).encode()).hexdigest()
prefixes={kind:'live-native-'+kind+'-'+key(value)+'-' for kind,value in [('downloads',downloads),('target',context),('ccache',context)]}
(out/'cache-inputs.json').write_text(json.dumps({'context':context,'prefixes':prefixes},indent=2)+'\n')
with open(os.environ['GITHUB_OUTPUT'],'a') as stream:
 for name,prefix in prefixes.items():stream.write(name+'_prefix='+prefix+'\n'+name+'_key='+prefix+os.environ['GITHUB_RUN_ID']+'\n')
PY
    ;;
  validate|seal)
    python3 - "$native_root" "$output_root" "${1}" <<'PY'
import hashlib,json,os,pathlib,subprocess,sys,tomllib
native,out,mode=pathlib.Path(sys.argv[1]),pathlib.Path(sys.argv[2]),sys.argv[3];plan=json.loads((out/'cache-inputs.json').read_text())
packages=tomllib.loads((native/'codex-rs/Cargo.lock').read_text())['package'];checksums={p['name']+'-'+p['version']+'.crate':p['checksum'] for p in packages if 'checksum' in p}
archives=0
for archive in pathlib.Path(os.environ['CARGO_HOME']).glob('registry/cache/*/*.crate'):
 if archive.name not in checksums:continue
 with archive.open('rb') as stream:actual=hashlib.file_digest(stream,'sha256').hexdigest()
 assert actual==checksums[archive.name],'CARGO_CACHE_CHECKSUM_MISMATCH';archives+=1
git_dbs=0
for repo in pathlib.Path(os.environ['CARGO_HOME']).glob('git/db/*'):
 if not repo.is_dir():continue
 result=subprocess.run(['git','-C',str(repo),'fsck','--strict','--no-reflogs'],capture_output=True)
 assert result.returncode==0,'CARGO_GIT_CACHE_INVALID';git_dbs+=1
for variable in ['CARGO_TARGET_DIR','CCACHE_DIR']:
 root=pathlib.Path(os.environ[variable]);marker=root/'.gestalt-native-cache-inputs.json'
 if marker.exists():assert json.loads(marker.read_text())==plan['context'],'COMPILED_CACHE_CONTEXT_MISMATCH'
 elif mode=='validate':assert not any(root.iterdir()),'COMPILED_CACHE_CONTEXT_MISSING'
 if mode=='seal':marker.write_text(json.dumps(plan['context'],sort_keys=True)+'\n')
(out/('cache-'+mode+'.json')).write_text(json.dumps({'registryArchivesVerified':archives,'gitDatabasesVerified':git_dbs,'contextVerified':True,'restored':{name:os.environ.get(name,'') for name in ['NATIVE_DOWNLOAD_CACHE_KEY','NATIVE_TARGET_CACHE_KEY','NATIVE_CCACHE_KEY']},'cacheIsProvenance':False},indent=2)+'\n')
PY
    ;;
  cpp-smoke)
    ccache --max-size 1G >/dev/null
    ccache --zero-stats >/dev/null
    probe_root=$(mktemp -d "$RUNNER_TEMP/native-cxx-probe.XXXXXX")
    printf 'int cached_answer() { return 42; }\n' > "$probe_root/probe.cc"
    "$CXX" -c "$probe_root/probe.cc" -o "$probe_root/probe.o"
    "$CXX" -c "$probe_root/probe.cc" -o "$probe_root/probe.o"
    ccache --print-stats > "$output_root/cache-logs/ccache-cxx.tsv"
    python3 - "$output_root/cache-logs/ccache-cxx.tsv" <<'PY'
import json,pathlib,sys
stats={line.split()[0]:int(line.split()[1]) for line in pathlib.Path(sys.argv[1]).read_text().splitlines() if len(line.split())==2}
hits=sum(stats.get(key,0) for key in ['direct_cache_hit','preprocessed_cache_hit','cache_hit_direct','cache_hit_preprocessed'])
assert hits>=1,'CXX_CCACHE_HIT_NOT_OBSERVED'
print(json.dumps({'cxxCompilerInvoked':True,'ccacheHits':hits,'stats':stats}))
PY
    ;;
  native-smoke)
    git -C "$native_root" apply --check "$(pwd -P)/tests/fixtures/live-host-native-mask/lock-preparation.patch"
    git -C "$native_root" apply "$(pwd -P)/tests/fixtures/live-host-native-mask/lock-preparation.patch"
    cd "$native_root/codex-rs"
    cargo build --locked --profile dev-small -p codex-bwrap > "$output_root/cache-logs/bwrap-first.log" 2>&1
    ccache --print-stats > "$output_root/cache-logs/ccache-before-native-repeat.tsv"
    cargo clean --profile dev-small -p codex-bwrap > "$output_root/cache-logs/bwrap-clean.log" 2>&1
    cargo build --locked --profile dev-small -p codex-bwrap > "$output_root/cache-logs/bwrap-repeat.log" 2>&1
    ccache --print-stats > "$output_root/cache-logs/ccache-after-native-repeat.tsv"
    python3 - "$output_root" <<'PY'
import json,pathlib,sys
root=pathlib.Path(sys.argv[1]);names=['direct_cache_hit','preprocessed_cache_hit','cache_hit_direct','cache_hit_preprocessed']
def stats(name):return {line.split()[0]:int(line.split()[1]) for line in (root/'cache-logs'/name).read_text().splitlines() if len(line.split())==2}
before=stats('ccache-before-native-repeat.tsv');after=stats('ccache-after-native-repeat.tsv');hits=sum(after.get(k,0)-before.get(k,0) for k in names)
assert hits>=1,'NATIVE_C_CCACHE_HIT_NOT_OBSERVED'
proof={'nativePackage':'codex-bwrap','compiler':'cc::Build through CC wrapper','repeatCcacheHits':hits,'v8Compiled':False,'stats':after};(root/'cache-smoke.json').write_text(json.dumps(proof,indent=2)+'\n');print(json.dumps(proof))
PY
    ;;
  stats)
    ccache --print-stats > "$output_root/cache-logs/ccache-final.tsv"
    ;;
  *) printf 'usage: %s {configure|validate|seal|cpp-smoke|native-smoke|stats}\n' "$0" >&2;exit 2 ;;
esac
