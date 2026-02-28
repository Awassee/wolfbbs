#!/usr/bin/env python3
"""
WolfBBS terminal end-to-end checks using pexpect.

Covers:
- new user registration/login
- main-menu navigation and status/config visibility
- quick-jump navigation
- sysop role elevation via oputil and admin terminal entrypoint
"""

from __future__ import annotations

import os
import socket
import subprocess
import sys
import tempfile
import time
from pathlib import Path

try:
    import pexpect
except ImportError as exc:  # pragma: no cover - environment dependency
    print(
        "pexpect is required. Install with: python3 -m pip install pexpect",
        file=sys.stderr,
    )
    raise SystemExit(2) from exc


ROOT = Path(__file__).resolve().parents[1]


def free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def wait_for_port(port: int, proc: subprocess.Popen[str] | None = None, timeout_sec: float = 90.0) -> None:
    deadline = time.time() + timeout_sec
    while time.time() < deadline:
        if proc is not None and proc.poll() is not None:
            raise RuntimeError(f"bbs process exited early with code {proc.returncode}")
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
            sock.settimeout(0.5)
            if sock.connect_ex(("127.0.0.1", port)) == 0:
                return
        time.sleep(0.1)
    raise RuntimeError(f"timeout waiting for ssh port {port}")


def start_bbs_server(db_path: Path, port: int, log_path: Path) -> subprocess.Popen[str]:
    env = os.environ.copy()
    env.setdefault("WOLFBBS_QUICK_JUMP_ENABLE", "true")
    env.setdefault("WOLFBBS_GUEST_TOUR_ENABLE", "true")
    cmd = [
        "go",
        "run",
        "./cmd/wolfbbs",
        "-listen",
        f"127.0.0.1:{port}",
        "-db",
        f"sqlite://{db_path}",
    ]
    log_f = log_path.open("a", encoding="utf-8")
    proc = subprocess.Popen(
        cmd,
        cwd=str(ROOT),
        env=env,
        stdout=log_f,
        stderr=subprocess.STDOUT,
        text=True,
    )
    wait_for_port(port, proc)
    return proc


def stop_process(proc: subprocess.Popen[str] | None) -> None:
    if proc is None:
        return
    if proc.poll() is not None:
        return
    proc.terminate()
    try:
        proc.wait(timeout=10)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait(timeout=5)


def run_oputil_set_role(db_path: Path, handle: str, role: str) -> None:
    cmd = [
        "go",
        "run",
        "./cmd/oputil",
        "--db",
        f"sqlite://{db_path}",
        "users",
        "set-role",
        "--handle",
        handle,
        "--role",
        role,
    ]
    completed = subprocess.run(
        cmd,
        cwd=str(ROOT),
        text=True,
        capture_output=True,
        check=False,
    )
    if completed.returncode != 0:
        raise RuntimeError(
            f"oputil set-role failed ({completed.returncode}):\n"
            f"stdout:\n{completed.stdout}\n"
            f"stderr:\n{completed.stderr}"
        )


def spawn_ssh(port: int) -> pexpect.spawn:
    cmd = (
        "ssh "
        "-o StrictHostKeyChecking=no "
        "-o UserKnownHostsFile=/dev/null "
        "-o LogLevel=ERROR "
        "-o PreferredAuthentications=password "
        "-o PubkeyAuthentication=no "
        "-o NumberOfPasswordPrompts=1 "
        "-o ConnectTimeout=10 "
        f"-p {port} localhost"
    )
    child = pexpect.spawn(cmd, cwd=str(ROOT), encoding="utf-8", timeout=25)
    idx = child.expect(
        [
            "Press any key to continue",
            "any other key to continue",
            "Handle:",
            r"[Pp]assword:",
        ]
    )
    if idx == 2:
        return child
    if idx == 3:
        child.sendline("wolfbbs")
        idx = child.expect(
            [
                "Press any key to continue",
                "any other key to continue",
                "Handle:",
            ]
        )
        if idx == 2:
            return child
    child.send("x")
    child.expect("Handle:")
    return child


def complete_login(
    child: pexpect.spawn,
    handle: str,
    password: str,
    create_if_missing: bool = False,
) -> None:
    child.sendline(handle)
    child.expect("Password:")
    child.sendline(password)
    idx = child.expect(
        [
            r"Create account\? \(Y/N\):",
            "Any key to return.",
            "Enter selection:",
        ]
    )
    if idx == 0:
        if not create_if_missing:
            raise RuntimeError(f"unexpected account-create prompt for {handle}")
        child.sendline("Y")
        child.expect("Welcome to WolfBBS! Press any key to continue.")
        child.send("x")
        child.expect("Any key to return.")
        child.send("x")
    elif idx == 1:
        child.send("x")
    child.expect("Enter selection:")


def run_regular_user_flow(port: int) -> None:
    child = spawn_ssh(port)
    try:
        complete_login(child, "e2eadmin", "password123", create_if_missing=True)
        child.send("?")
        child.expect("Main Menu Key Guide")
        child.send("x")
        child.expect("Enter selection:")

        child.send("N")
        child.expect("Newscan Digest")
        child.send("x")
        child.expect("Enter selection:")

        child.send("M")
        child.expect("Select board ID")
        child.sendline("Q")
        child.expect("Enter selection:")

        child.send("P")
        child.expect("Private Mail")
        child.sendline("Q")
        child.expect("Enter selection:")

        child.send("F")
        child.expect("Files")
        child.sendline("Q")
        child.expect("Enter selection:")

        child.send("C")
        child.expect("Live Chat")
        child.send("Q")
        child.expect("Enter selection:")

        child.send("G")
        child.expect("Gateway Menu")
        child.send("Q")
        child.expect("Enter selection:")

        child.send("D")
        child.expect("Door Hub")
        child.send("R")
        child.expect("Enter selection:")

        child.send("L")
        child.expect("Last Callers")
        child.send("x")
        child.expect("Enter selection:")

        child.send("W")
        child.expect("Who's Online")
        child.send("x")
        child.expect("Enter selection:")

        child.send("S")
        child.expect("MCI Preferences")
        child.send("Q")
        child.expect("Enter selection:")

        child.send("A")
        child.expect("Admin access denied. Press any key.")
        child.send("x")
        child.expect("Enter selection:")

        child.send("Y")
        child.expect("Status Center")
        child.expect("Quick jump state")
        child.send("x")
        child.expect("Enter selection:")

        child.send("/")
        child.expect("Jump target")
        child.sendline("boards")
        child.expect("Select board ID")
        child.sendline("Q")
        child.expect("Enter selection:")

        child.send("X")
        child.expect("Config Center")
        child.expect("Guest tour enabled")
        child.send("x")
        child.expect("Enter selection:")

        child.send("Q")
        child.expect(pexpect.EOF)
    finally:
        child.close(force=True)


def run_admin_flow(port: int) -> None:
    child = spawn_ssh(port)
    try:
        complete_login(child, "e2eadmin", "password123", create_if_missing=False)
        child.send("A")
        child.expect("Use web admin at /admin for full sysop controls.")
        child.send("x")
        child.expect("Enter selection:")
        child.send("Q")
        child.expect(pexpect.EOF)
    finally:
        child.close(force=True)


def main() -> int:
    tmp_root = Path(tempfile.mkdtemp(prefix="wolfbbs-tui-e2e-"))
    db_path = tmp_root / "wolfbbs-e2e.db"
    log_path = tmp_root / "wolfbbs-e2e.log"
    port = free_port()
    proc = None
    try:
        proc = start_bbs_server(db_path, port, log_path)
        run_regular_user_flow(port)
        stop_process(proc)
        proc = None

        run_oputil_set_role(db_path, "e2eadmin", "sysop")

        proc = start_bbs_server(db_path, port, log_path)
        run_admin_flow(port)
        print("PASS terminal e2e (pexpect)")
        return 0
    except Exception as exc:  # pragma: no cover - integration failure path
        print(f"FAIL terminal e2e: {exc}", file=sys.stderr)
        if log_path.exists():
            print(f"log: {log_path}", file=sys.stderr)
        return 1
    finally:
        stop_process(proc)


if __name__ == "__main__":
    raise SystemExit(main())
