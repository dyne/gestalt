# Test-only real POSIX terminal. Production Unix lifetime remains legacy-owned.
import base64, errno, fcntl, json, os, pathlib, pty, select, signal, struct, subprocess, sys, termios, time

request = json.loads(pathlib.Path(sys.argv[1]).read_text())
master, slave = pty.openpty()
fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 40, 110, 0, 0))
child = subprocess.Popen(request['command'], cwd=request['cwd'], env=request['environment'], stdin=slave, stdout=slave, stderr=slave)
os.close(slave)
chunks = []; tail = b''; submitted = False; stopped = False; deadline = time.monotonic() + 180
control_events = []; interrupt_offset = None; interrupt_settled_count = None
driver_interrupted = False
def stop_driver(signum, frame):
    global driver_interrupted
    driver_interrupted = True
signal.signal(signal.SIGTERM, stop_driver)
try:
    while time.monotonic() < deadline and not driver_interrupted:
        if select.select([master], [], [], 0.05)[0]:
            try:
                chunk = os.read(master, 65536)
            except OSError as error:
                if error.errno != errno.EIO:
                    raise
                break
            if not chunk:
                break
            chunks.append(chunk); query = tail + chunk
            for _ in range(query.count(b'\x1b[6n')):
                os.write(master, b'\x1b[1;1R')
            tail = query[-3:]
        output = b''.join(chunks)
        if not submitted and b'7501;state=idle' in output:
            os.write(master, b'Native core slice; trusted hook must block provider I/O\r')
            submitted = True
        if not stopped and pathlib.Path(request['report']).exists() and b'Native core slice no-model barrier' in output:
            if request['control'] == 'eof':
                os.write(master, b'\x04')
            elif request['control'] == 'interrupt':
                # Pinned 0.164 disables double-press quit. While work is active
                # Ctrl-C interrupts; only a later observed idle Ctrl-C exits.
                interrupt_offset = len(output); interrupt_settled_count = output.count(b'\x1b]7501;state=idle') + output.count(b'\x1b]7501;state=done')
                os.write(master, b'\x03')
            else:
                raise RuntimeError('Unknown terminal control fixture')
            stopped = True
            control_events.append({'action': 'eof' if request['control'] == 'eof' else 'ctrl-c-after-barrier',
                                   'outputOffset': len(output), 'settledCount': output.count(b'\x1b]7501;state=idle') + output.count(b'\x1b]7501;state=done'), 'observedAt': time.time()})
        if interrupt_offset is not None and output.count(b'\x1b]7501;state=idle') + output.count(b'\x1b]7501;state=done') > interrupt_settled_count:
            interrupted_text = b'Conversation interrupted' in output[interrupt_offset:]
            os.write(master, b'\x03'); interrupt_offset = None
            control_events.append({'action': 'ctrl-c-after-settled', 'outputOffset': len(output),
                                   'settledState': 'done' if output.rfind(b'\x1b]7501;state=done') > output.rfind(b'\x1b]7501;state=idle') else 'idle',
                                   'conversationInterruptedObserved': interrupted_text, 'observedAt': time.time()})
        if child.poll() is not None:
            break
    timed_out = child.poll() is None
    if timed_out:
        # Request existing legacy cleanup first. Neither this timeout nor a
        # parent-only forced kill supplies a new product complete-tree proof.
        child.terminate()
        try:
            child.wait(timeout=5)
        except subprocess.TimeoutExpired:
            child.kill()
    exit_code = child.wait(timeout=5)
    result = {'pid': child.pid, 'exit': exit_code, 'timedOut': timed_out, 'submittedBlockedFixtureTurn': submitted,
              'control': request['control'], 'controlSent': stopped, 'controlEvents': control_events, 'driverInterrupted': driver_interrupted, 'outputBase64': base64.b64encode(b''.join(chunks)).decode(),
              'ownership': 'Unix compatibility; no new complete-tree claim', 'observedAt': time.time()}
    pathlib.Path(request['result']).write_text(json.dumps(result, indent=2) + '\n')
    sys.exit(0 if not driver_interrupted and not timed_out and exit_code == 0 and submitted and stopped else 1)
finally:
    if child.poll() is None:
        child.kill(); child.wait(timeout=5)
    os.close(master)
