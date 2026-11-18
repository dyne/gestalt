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

def authority_records(execution, authority, before, binding):
    def require(condition, message):
        if not condition: raise ValueError(message)
    require(execution['timedOut'] is False and execution['primaryError'] is None, 'authority execution timed out/failed')
    require(execution['status'] == {'exit': 0, 'signal': None}, 'authority process did not exit normally')
    require(execution['lastAuthorityPhase'] == authority['phase'] == 'authority-complete' and authority['pass'] is True, 'authority did not complete successfully')
    require(authority['guard']['ok'] is True and authority['publication']['attempted'] is True and authority['publication']['published'] is True, 'Org guard/publication failed')
    require(authority['publication']['path'].startswith(execution['fixture'].rstrip('/') + '/status/'), 'publication escaped private fixture')
    published = authority['publishedFile']
    require(published['planMatches'] is True and published['schemaVersion'] == 1 and published['mode'] == '600' and published['uid'] == authority['uid'] == before['uid'], 'published plan identity/private ownership differs')
    require(authority['ipc']['ok'] is True, 'authenticated whole IPC failed')
    request = 'authenticated host echo é東京'.encode().hex()
    require(authority['ipc']['requestHex'] == authority['ipc']['replyHex'] == request, 'authenticated IPC bytes differ')
    require(authority['identityMismatch']['refused'] is True, 'wrong creation identity was accepted')
    require(authority['actualIdentity'] == before['identity'] and authority['pid'] == before['pid'] == before['identity']['pid'] and authority['uid'] == before['identity']['uid'], 'exec changed actual child identity')
    require(authority['expected'] == execution['host'] and before['ppid'] == execution['host']['pid'], 'authority host identity differs')
    credentials = authority['sameSocketCredentials']
    require(credentials['exit'] == 0 and credentials['signal'] is None and credentials['peer'] == execution['host'], 'same-socket host credentials differ')
    require(any(m['exit'] == 0 and m['signal'] is None and m['timedOut'] is False and m['peer'] == before['identity'] for m in execution['measurements']), 'host did not measure exact child peer identity')
    require(before['environment'] == execution['ownerContextPreserved'], 'managed owner environment was remapped')
    require(authority['agentsCheckout'] in [binding['acceptedCommit'], binding['remoteCheckoutCommit']], 'unaccepted Agents checkout')
    expected = {r['path']: r for r in binding['files']}; actual = {r['path']: r for r in authority['sourceBindings']}
    require(len(expected) == len(actual) == len(authority['sourceBindings']) == 15 and set(actual) == set(expected), 'incomplete/duplicate Agents source binding')
    for name, record in expected.items():
        measured = actual[name]; canonical = '755' if record['gitMode'] == '100755' else '644'
        require(measured['bytes'] == record['bytes'] and measured['sha256'] == record['sha256'] and measured['regularFile'] is True and measured['canonicalGitMode'] == record['gitMode'] and measured['mode'] == canonical, 'Agents source bytes/mode differ: ' + name)
    return {'pass': True, 'phase': authority['phase'], 'publication': True, 'wholeAuthenticatedIpc': True,
            'sourceBindings': len(actual), 'actualChild': authority['actualIdentity'], 'expectedHost': execution['host'], 'identityMismatchRefused': True}


def main():
    root = pathlib.Path.cwd(); evidence = root / 'native-evidence/l6-stable'; evidence.mkdir(parents=True, exist_ok=True)
    now = lambda: datetime.datetime.now(datetime.timezone.utc).isoformat()
    head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    assert head == os.environ['GITHUB_SHA']
    assert not subprocess.check_output(['git', 'status', '--porcelain=v1', '--untracked-files=no']), 'Tracked source changed before gates'
    assert 'GESTALT_SESSION_TMP_CONTEXT' not in os.environ, 'Hosted acceptance requires an ordinary independent runner context'
    frozen = root / os.environ['GESTALT_RELEASE_TARBALL']
    assert digest(frozen.read_bytes()) == os.environ['GESTALT_RELEASE_TARBALL_SHA256']
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
    with tempfile.TemporaryDirectory(prefix='l6-native-sandbox-') as home:
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
                assert old_manifest.pop('gitHead') == os.environ['GESTALT_RELEASE_SOURCE_HEAD']
                assert new_manifest.pop('gitHead') == head
                assert new_manifest == old_manifest, 'unexpected generated package metadata change'
                comparison = {'observedAt': now(), 'builtTar': reference(archive), 'frozenTar': reference(frozen), 'status': 'same payload/modes; only generated manifest.gitHead follows TEST checkout HEAD',
                    'memberCount': len(current), 'frozenGitHead': os.environ['GESTALT_RELEASE_SOURCE_HEAD'], 'builtGitHead': head,
                    'claimLimit': 'This hosted Linux build has the TEST checkout gitHead. The frozen accepted-source tar is the package consumed by the separate Windows npm 11/12 job. This comparison alone supplies no Windows execution or final delivery-commit provenance.',
                    'members': {name: {'bytes': len(item['bytes']), 'sha256': digest(item['bytes']), 'mode': oct(item['mode'])} for name, item in current.items()}}
            except Exception as error:
                packed['payloadError'] = str(error)
    command('docs-build', ['npm', 'run', 'build'])
    if ready['exit'] == 0:
        authority = command('unix-authority', ['node', 'tests/native-unix-compatibility.mjs', '--agents', '.native-dependencies/agents', '--evidence', 'native-evidence/l6-stable/unix-authority'])
        if authority['exit'] == 0:
            try:
                execution = json.loads((evidence / 'unix-authority/execution.json').read_text())
                report = json.loads((evidence / 'unix-authority/authority.json').read_text())
                before = json.loads((evidence / 'unix-authority/before-exec.json').read_text())
                binding = json.loads((root / 'tests/fixtures/native-runtime/agents-source.json').read_text())
                authority['verifiedAuthority'] = authority_records(execution, report, before, binding)
                authority['authorityEvidence'] = {name: reference(evidence / ('unix-authority/' + name)) for name in ['execution.json', 'authority.json', 'before-exec.json']}
                authority['authorityEvidence']['acceptedSourceBinding'] = reference(root / 'tests/fixtures/native-runtime/agents-source.json')
            except Exception as error: authority['authorityError'] = str(error)
    else: records.append({'name': 'unix-authority', 'status': 'not-executed', 'exit': None, 'reason': 'Native sandbox prerequisite failed'})
    (evidence / 'commands.json').write_text(json.dumps(records, indent=2) + '\n')
    (evidence / 'payload-comparison.json').write_text(json.dumps(comparison, indent=2) + '\n')
    cli = root / 'dist/cli/gestalt.mjs'
    identity = {'observedAt': now(), 'head': head, 'node': subprocess.check_output(['node', '--version'], text=True).strip(),
        'npm': subprocess.check_output(['npm', '--version'], text=True).strip(), 'CLI': reference(cli) if cli.exists() else None,
        'sourceInventory': reference(root / 'native-evidence/setup/checkout.json'),
        'platform': 'linux', 'arch': subprocess.check_output(['node', '-p', 'process.arch'], text=True).strip(), 'expectedManagerNodeCommands': expected,
        'complete': all(r['status'] == 'completed' and r['exit'] == 0 and not any(r.get(k) for k in ['summaryError', 'payloadError', 'authorityError']) for r in records) and comparison is not None,
        'scope': 'One hosted Ubuntu full Manager/package/docs/sandbox/accepted-Org authority job; Windows and macOS target jobs produce separate evidence.'}
    if identity['CLI'] is None or identity['CLI']['bytes'] != 336071 or identity['CLI']['sha256'] != '216b6ea052ceb2fa4223f8fc51b9129af020adcfca32fbfd70011e5f9a59bf60': identity['complete'] = False; identity['CLIError'] = 'Compiled core seal differs'
    identity['trackedSourceAfter'] = subprocess.check_output(['git', 'status', '--porcelain=v1', '--untracked-files=no'], text=True)
    if identity['trackedSourceAfter']: identity['complete'] = False; identity['sourceError'] = 'Tracked source changed during gates'
    if not identity['npm'].startswith('11.'): identity['complete'] = False; identity['npmLaneError'] = 'Expected npm 11 for the Linux full suite and package comparison. The separate Windows job exercises the frozen candidate with npm 12.'
    (evidence / 'result.json').write_text(json.dumps(identity, indent=2) + '\n')
    assert identity['complete'], 'At least one full gate/coverage/payload check failed; complete raw captures retained'

if __name__ == '__main__': main()
