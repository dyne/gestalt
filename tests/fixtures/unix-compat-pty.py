# Test-only actual POSIX terminal; no production supervision or ownership claim.
import base64, errno, fcntl, json, os, pty, select, struct, subprocess, sys, termios, time
master, slave = pty.openpty()
fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH',25,81,0,0))
child = subprocess.Popen(sys.argv[1:], stdin=slave, stdout=slave, stderr=slave)
os.close(slave); chunks=[]; deadline=time.monotonic()+10
try:
    while time.monotonic()<deadline:
        if select.select([master],[],[],0.05)[0]:
            try: data=os.read(master,65536)
            except OSError as error:
                if error.errno!=errno.EIO: raise
                break
            if not data: break
            chunks.append(data)
        elif child.poll() is not None: break
    result=child.wait(timeout=1)
    print(json.dumps({'exit':result,'outputBase64':base64.b64encode(b''.join(chunks)).decode()}))
finally:
    if child.poll() is None: child.kill(); child.wait(timeout=5)
    os.close(master)
