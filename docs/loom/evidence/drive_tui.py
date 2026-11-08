#!/usr/bin/env python3
"""Drive the real ytss TUI in a pty: open the picker, select the config,
start a run with 'r', watch the events panel and the log file, then quit."""
import fcntl
import os
import pty
import re
import select
import struct
import subprocess
import sys
import termios
import time

BIN = "/private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/ytss-at/ytss"
CFG = "/private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/ytss-at/tui-config"
OUT = "/tmp/ytss-smoke-out"
LOG = os.path.join(OUT, "logs", "ytss.log")

env = dict(os.environ)
env["YTSS_CONFIG_DIR"] = CFG
env["TERM"] = "xterm-256color"

pid, fd = pty.fork()
if pid == 0:
    os.execve(BIN, [BIN, "tui"], env)

# Give the pty a real terminal size (120x30) so the TUI renders.
fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 30, 120, 0, 0))

def read_screen(timeout=1.0):
    out = b""
    end = time.time() + timeout
    while time.time() < end:
        r, _, _ = select.select([fd], [], [], 0.2)
        if r:
            try:
                data = os.read(fd, 65536)
            except OSError:
                break
            if not data:
                break
            out += data
        else:
            if out:
                break
    return out

def send(keys):
    os.write(fd, keys)

def strip_ansi(b):
    return re.sub(rb"\x1b\[[0-9;?]*[a-zA-Z]", b"", b).replace(b"\x1b[8m", b"")

def capture(label):
    data = read_screen()
    plain = strip_ansi(data).decode("utf-8", "replace")
    print("=====SCREEN %s BEGIN=====" % label)
    print(plain)
    print("=====SCREEN %s END=====" % label)

time.sleep(0.8)
capture("startup")
time.sleep(0.5)
capture("after-wait")
send(b"\r")           # Enter: open the file picker popup
time.sleep(0.6)
capture("picker-open")
send(b"\r")           # Enter: select smoke.yaml
time.sleep(0.6)
capture("config-loaded")
send(b"r")            # start the run
time.sleep(2.5)
capture("after-run")
# the log file should now exist with the run's records
try:
    with open(LOG, "rb") as f:
        content = f.read()
    print("=====LOG FILE %s BEGIN=====" % LOG)
    print(content.decode("utf-8", "replace"))
    print("=====LOG FILE END=====")
except FileNotFoundError:
    print("NO LOG FILE AT %s" % LOG)
send(b"q")            # quit (idle -> immediate)
time.sleep(0.5)
try:
    os.close(fd)
except OSError:
    pass
os.waitpid(pid, 0)
print("TUI EXITED")
