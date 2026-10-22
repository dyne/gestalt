#!/usr/bin/env python3
"""Bounded start/health/shutdown proof for the exact packaged executable."""
import hashlib
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import time
import urllib.request

binary = pathlib.Path(sys.argv[1]).resolve(strict=True)
output = pathlib.Path(sys.argv[2]).resolve()
with tempfile.TemporaryDirectory(prefix='impeccable-smoke-') as temporary:
    root = pathlib.Path(temporary)
    subprocess.run(['git', 'init', '--quiet', str(root)], check=True, timeout=10)
    home = root / 'home'
    home.mkdir()
    env = {'PATH': os.defpath, 'HOME': str(home), 'IMPECCABLE_LIVE_COPY_AGENT': 'chat',
           'IMPECCABLE_LIVE_PUBLIC_BASE_URL': 'https://preview.example.invalid/helper'}
    record = root / '.impeccable/live/server.json'
    # Logs can contain the private token. Keep them private and never upload them.
    with (root / 'private.log').open('wb') as log:
        child = subprocess.Popen([str(binary), 'live-server', '--port=0'], cwd=root,
                                 env=env, stdout=log, stderr=log)
        try:
            deadline = time.monotonic() + 20
            info = None
            while time.monotonic() < deadline:
                if child.poll() is not None:
                    raise SystemExit('Runtime exited before startup')
                try:
                    info = json.loads(record.read_text())
                    if info['pid'] == child.pid and 0 < info['port'] < 65536:
                        break
                except (OSError, ValueError, KeyError):
                    pass
                time.sleep(0.1)
            else:
                raise SystemExit('Runtime startup timed out')
            url = f'http://127.0.0.1:{info["port"]}/health'
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            with opener.open(url, timeout=5) as response:
                health = json.loads(response.read(65536))
            if health.get('status') != 'ok' or health.get('port') != info['port']:
                raise SystemExit('Runtime health failed')
            child.terminate()
            if child.wait(timeout=10) != 0 or record.exists():
                raise SystemExit('Runtime clean shutdown failed')
            try:
                opener.open(url, timeout=2).close()
            except OSError:
                pass
            else:
                raise SystemExit('Runtime listener remained open')
            result = {'schemaVersion': 1, 'state': 'passed',
                      'binarySha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
                      'health': 'ok', 'shutdownExit': 0, 'serverRecordRemoved': True,
                      'listenerClosed': True, 'startupTimeoutSeconds': 20,
                      'publicBaseUrlConfigured': info.get('publicBaseUrl') == env['IMPECCABLE_LIVE_PUBLIC_BASE_URL']}
            if not result['publicBaseUrlConfigured']:
                raise SystemExit('Runtime public URL configuration failed')
            output.write_text(json.dumps(result, indent=2) + '\n')
            print('Runtime start, health and clean shutdown passed; private logs removed.')
        finally:
            if child.poll() is None:
                child.terminate()
                try:
                    child.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    child.kill()
                    child.wait(timeout=5)
