import datetime, hashlib, json, os, pathlib, re, subprocess, tarfile, tempfile

FIELDS = {'tests', 'pass', 'fail', 'cancelled', 'skipped', 'todo'}
PATTERN = re.compile(r'^(?:ℹ |# )(tests|pass|fail|cancelled|skipped|todo) (\d+)\r?$', re.M)

def summaries(text, expected_blocks=None, exact=None):
    text = re.sub(r'\x1b\[[0-?]*[ -/]*[@-~]', '', text)
    blocks = []; current = None
    for key, value in PATTERN.findall(text):
        if key == 'tests':
            if current is not None: raise ValueError('incomplete Node summary')
            current = {}
        if current is None or key in current: raise ValueError('orphan/duplicate summary field')
        current[key] = int(value)
        if set(current) == FIELDS:
            if current['tests'] <= 0 or current['tests'] != sum(current[k] for k in FIELDS - {'tests'}):
                raise ValueError('empty/inconsistent Node summary')
            blocks.append(current); current = None
    if current is not None or not blocks: raise ValueError('missing/incomplete Node summaries')
    if expected_blocks is not None and len(blocks) != expected_blocks: raise ValueError('Node command coverage differs')
    total = {k: sum(b[k] for b in blocks) for k in FIELDS}
    if total['pass'] <= 0 or any(total[k] for k in ['fail', 'cancelled', 'todo']): raise ValueError('failed/cancelled/todo/all-skipped selection')
    if exact is not None and total != exact: raise ValueError('intended selected test counts differ')
    return {'blocks': blocks, 'totals': total, 'tapSkipLines': re.findall(r'^\s*ok \d+ - .* # SKIP.*$', text, re.M),
            'countScope': 'Node summary blocks only; shell/process/authority assertions remain separate exit-bound gates.'}

def digest(data): return hashlib.sha256(data).hexdigest()
def reference(path): return {'path': str(path), 'bytes': path.stat().st_size, 'sha256': digest(path.read_bytes())}
def payload(path):
    with tarfile.open(path) as archive:
        result = {}
        for member in archive.getmembers():
            if not member.isfile() or member.name in result: raise ValueError('unexpected/duplicate payload member')
            result[member.name] = {'bytes': archive.extractfile(member).read(), 'mode': member.mode}
        return result

def main():
    root = pathlib.Path.cwd(); evidence = root / 'native-evidence/l5-stable'; evidence.mkdir(parents=True, exist_ok=True)
    now = lambda: datetime.datetime.now(datetime.timezone.utc).isoformat()
    head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    assert head == os.environ['GITHUB_SHA']
    assert not subprocess.check_output(['git', 'status', '--porcelain=v1', '--untracked-files=no']), 'Tracked source changed before gates'
    assert 'GESTALT_SESSION_TMP_CONTEXT' not in os.environ, 'Hosted acceptance requires an ordinary independent runner context'
    frozen = root / 'tests/fixtures/l5-full-transport/candidate.tgz'
    assert digest(frozen.read_bytes()) == '36ce3852be3aa47d51d827064fafe20097717b5d8dd73d3693e9f48af286df67'
    original = payload(frozen); assert len(original) == 10
    helper = original['package/lib/native/gestalt-owned-process.exe']['bytes']
    provenance = original['package/lib/native/gestalt-owned-process.exe.provenance.json']['bytes']
    assert digest(helper) == '753b5842c0dd0aa293bc0246312b73e955718ec97491088939e0f951ae2e6128'
    assert digest(provenance) == '6577dc64c6f53d55d6bd103a8bcd2624d8cb916d1963f54b7227b62e6cdc6645'
    native = json.loads(provenance); assert native['platform'] == 'win32' and native['architecture'] == 'x64' and native['artifactSha256'] == digest(helper)
    inputs = ['src/cli/platform/native/owned-process.c', 'src/cli/platform/native/windows-storage-check.c', 'src/cli/platform/native/windows-storage-check.h']
    assert len(native['sourceInputs']) == len(inputs)
    for relative in inputs:
        data = (root / relative).read_bytes(); record = next(v for v in native['sourceInputs'] if v['path'] == relative)
        assert record['bytes'] == len(data) and record['sha256'] == digest(data)
    native_dir = root / 'dist/cli/native'; native_dir.mkdir(parents=True, exist_ok=True)
    (native_dir / 'gestalt-owned-process.exe').write_bytes(helper)
    (native_dir / 'gestalt-owned-process.exe.provenance.json').write_bytes(provenance)
    (evidence / 'frozen-inputs.json').write_text(json.dumps({'observedAt': now(), 'head': head, 'frozenTar': reference(frozen),
        'helper': reference(native_dir / 'gestalt-owned-process.exe'), 'originalProvenance': reference(native_dir / 'gestalt-owned-process.exe.provenance.json'),
        'originalCompilerEvidence': 'Frozen Windows target-built artifact/source3 proof at original timestamp; never compiled/executed as a Windows consumer on this Ubuntu job.',
        'sourceInputs': native['sourceInputs'], 'scope': 'Disposable hosted Ubuntu; no local host permission/owner remapping.'}, indent=2) + '\n')
    records = []; environment = {**os.environ, 'FORCE_COLOR': '0', 'BASE_PATH': '/gestalt/'}
    def command(name, argv, expected_blocks=None, exact=None, extra=None):
        record = {'name': name, 'argv': argv, 'cwd': str(root), 'startedAt': now(), 'head': head, 'status': 'running', 'environmentOverrides': {'FORCE_COLOR': '0', 'BASE_PATH': '/gestalt/', **(extra or {})}}
        records.append(record); (evidence / 'commands.json').write_text(json.dumps(records, indent=2) + '\n')
        out = evidence / (name + '.stdout'); err = evidence / (name + '.stderr')
        with out.open('wb') as stdout, err.open('wb') as stderr:
            result = subprocess.run(argv, cwd=root, env={**environment, **(extra or {})}, stdout=stdout, stderr=stderr)
        record.update(exit=result.returncode, status='completed', observedAt=now(), stdout=reference(out), stderr=reference(err))
        if expected_blocks is not None:
            try: record['reportedCounts'] = summaries(out.read_text(errors='replace') + '\n' + err.read_text(errors='replace'), expected_blocks, exact)
            except ValueError as error: record['summaryError'] = str(error)
        (evidence / 'commands.json').write_text(json.dumps(records, indent=2) + '\n')
        return record
    with tempfile.TemporaryDirectory(prefix='l5-native-sandbox-') as home:
        ready = command('codex-sandbox-preflight', ['./node_modules/.bin/codex', '-c', 'default_permissions=":read-only"', 'sandbox', '--', '/bin/true'], extra={'CODEX_HOME': home})
    run_script = (root / 'tests/run.sh').read_text()
    expected = sum(line.startswith('node --test ') for line in run_script.splitlines()) + 2
    assert expected == 34, 'Review full Manager command inventory after any test-runner change'
    if ready['exit'] == 0:
        command('manager-full', ['npm', 'test'], expected_blocks=expected)
        command('scratch-sandbox', ['npm', 'run', 'test:scratch:sandbox'], expected_blocks=1,
                exact={'tests': 1, 'pass': 1, 'fail': 0, 'cancelled': 0, 'skipped': 0, 'todo': 0})
    else:
        for name in ['manager-full', 'scratch-sandbox']:
            records.append({'name': name, 'status': 'not-executed', 'reason': 'Actual hosted native sandbox prerequisite failed; not a passing/skipped test', 'exit': None})
    command('packed-prerequisite', ['node', '--test', '--test-reporter=tap', 'tests/packed-core-prerequisite.test.mjs'], expected_blocks=1,
            exact={'tests': 8, 'pass': 8, 'fail': 0, 'cancelled': 0, 'skipped': 0, 'todo': 0})
    built = command('build-package', ['npm', 'run', 'build:package'])
    comparison = None
    if built['exit'] == 0:
        destination = evidence / 'packed'; destination.mkdir()
        packed = command('pack-built-payload', ['npm', 'pack', './dist/npm', '--ignore-scripts', '--json', '--pack-destination', str(destination)])
        if packed['exit'] == 0:
            try:
                info = json.loads((evidence / 'pack-built-payload.stdout').read_text())[0]
                archive = destination / info['filename']; current = payload(archive)
                assert set(current) == set(original), 'package member list changed'
                for name in current:
                    assert current[name]['mode'] == original[name]['mode'], name
                    if name != 'package/package.json': assert current[name]['bytes'] == original[name]['bytes'], name
                old_manifest = json.loads(original['package/package.json']['bytes']); new_manifest = json.loads(current['package/package.json']['bytes'])
                assert old_manifest.pop('gitHead') == '6707cfb91bb89d9c1a48a0477422441952424681'
                assert new_manifest.pop('gitHead') == head
                assert new_manifest == old_manifest, 'unexpected generated package metadata change'
                comparison = {'observedAt': now(), 'builtTar': reference(archive), 'frozenTar': reference(frozen), 'status': 'same payload/modes; only generated manifest.gitHead follows TEST checkout HEAD',
                    'memberCount': len(current), 'frozenGitHead': '6707cfb91bb89d9c1a48a0477422441952424681', 'builtGitHead': head,
                    'claimLimit': 'No claim of new whole Windows tar installation or final accepted-source/release artifact; current installed36ce proof retains its exact identity/original times.',
                    'members': {name: {'bytes': len(item['bytes']), 'sha256': digest(item['bytes']), 'mode': oct(item['mode'])} for name, item in current.items()}}
            except Exception as error:
                packed['payloadError'] = str(error)
    command('docs-build', ['npm', 'run', 'build'])
    if ready['exit'] == 0:
        authority = command('unix-authority', ['node', 'tests/native-unix-compatibility.mjs', '--agents', '.native-dependencies/agents', '--evidence', 'native-evidence/l5-stable/unix-authority'])
        if authority['exit'] == 0:
            try:
                execution = json.loads((evidence / 'unix-authority/execution.json').read_text())
                assert not execution['timedOut'] and execution['primaryError'] is None and execution['status']['exit'] == 0 and execution['observed']['pass']
                authority['authorityEvidence'] = reference(evidence / 'unix-authority/execution.json')
            except Exception as error: authority['authorityError'] = str(error)
    else: records.append({'name': 'unix-authority', 'status': 'not-executed', 'exit': None, 'reason': 'Native sandbox prerequisite failed'})
    (evidence / 'commands.json').write_text(json.dumps(records, indent=2) + '\n')
    (evidence / 'payload-comparison.json').write_text(json.dumps(comparison, indent=2) + '\n')
    cli = root / 'dist/cli/gestalt.mjs'
    identity = {'observedAt': now(), 'head': head, 'node': subprocess.check_output(['node', '--version'], text=True).strip(),
        'npm': subprocess.check_output(['npm', '--version'], text=True).strip(), 'CLI': reference(cli) if cli.exists() else None,
        'source29': 'Frozen product c931837de7727a4f8a12fac2eaff12a6af4bd982 / digestfa8572fd8524c8f43bd4959106e52be6be83035a017de99a52eb831439e2509c',
        'platform': 'linux', 'arch': subprocess.check_output(['node', '-p', 'process.arch'], text=True).strip(), 'expectedManagerNodeCommands': expected,
        'complete': all(r['status'] == 'completed' and r['exit'] == 0 and not any(r.get(k) for k in ['summaryError', 'payloadError', 'authorityError']) for r in records) and comparison is not None,
        'scope': 'One hosted Ubuntu full Manager/package/docs/sandbox/accepted-Org authority job; prior unchanged native Windows and current Mac evidence separately bound, not repeated. npm12/fullOS/release/terminal later L6.'}
    if identity['CLI'] is None or identity['CLI']['bytes'] != 336071 or identity['CLI']['sha256'] != '216b6ea052ceb2fa4223f8fc51b9129af020adcfca32fbfd70011e5f9a59bf60': identity['complete'] = False; identity['CLIError'] = 'Compiled core seal differs'
    identity['trackedSourceAfter'] = subprocess.check_output(['git', 'status', '--porcelain=v1', '--untracked-files=no'], text=True)
    if identity['trackedSourceAfter']: identity['complete'] = False; identity['sourceError'] = 'Tracked source changed during gates'
    if not identity['npm'].startswith('11.'): identity['complete'] = False; identity['npmLaneError'] = 'Expected primary npm11 lane; final npm12 matrix is deferred'
    (evidence / 'result.json').write_text(json.dumps(identity, indent=2) + '\n')
    assert identity['complete'], 'At least one full gate/coverage/payload check failed; complete raw captures retained'

if __name__ == '__main__': main()
