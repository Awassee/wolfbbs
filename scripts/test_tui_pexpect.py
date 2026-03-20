#!/usr/bin/env python3
"""
WolfBBS terminal end-to-end checks using pexpect.

Covers:
- new user registration/login
- main-menu navigation and status/config visibility
- quick-jump navigation
- deep terminal UX checks: menu traversal, compose editing, paging/wrapping, resize/redraw, and input correction
- offline/collections/settings parity checks
- sysop role elevation via oputil and deeper admin terminal traversal
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


def start_bbs_server(db_path: Path, port: int, log_path: Path, files_root: Path) -> subprocess.Popen[str]:
    env = os.environ.copy()
    env.setdefault("WOLFBBS_QUICK_JUMP_ENABLE", "true")
    env.setdefault("WOLFBBS_GUEST_TOUR_ENABLE", "true")
    env.setdefault("WOLFBBS_FILES_ROOT", str(files_root))
    env["WOLFBBS_OFFLINE_DIR"] = str(files_root / "offline")
    cmd = [
        "go",
        "run",
        "./cmd/wolfbbs",
        "-listen",
        f"127.0.0.1:{port}",
        "-db",
        f"sqlite://{db_path}",
    ]
    last_exc: Exception | None = None
    for attempt in range(1, 4):
        log_f = log_path.open("a", encoding="utf-8")
        proc = subprocess.Popen(
            cmd,
            cwd=str(ROOT),
            env=env,
            stdout=log_f,
            stderr=subprocess.STDOUT,
            text=True,
        )
        setattr(proc, "_wolfbbs_log_file", log_f)
        try:
            wait_for_port(port, proc, timeout_sec=120.0)
            return proc
        except Exception as exc:
            last_exc = exc
            stop_process(proc)
            if attempt == 3:
                raise
            time.sleep(1.0)
    raise RuntimeError(f"unreachable startup failure: {last_exc}")


def stop_process(proc: subprocess.Popen[str] | None) -> None:
    if proc is None:
        return
    try:
        if proc.poll() is None:
            proc.terminate()
            try:
                proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait(timeout=5)
    finally:
        log_f = getattr(proc, "_wolfbbs_log_file", None)
        if log_f is not None:
            log_f.close()
            setattr(proc, "_wolfbbs_log_file", None)
        time.sleep(0.5)


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
    last_completed: subprocess.CompletedProcess[str] | None = None
    for attempt in range(1, 4):
        completed = subprocess.run(
            cmd,
            cwd=str(ROOT),
            text=True,
            capture_output=True,
            check=False,
        )
        last_completed = completed
        if completed.returncode == 0:
            return
        time.sleep(0.4 * attempt)
    assert last_completed is not None
    raise RuntimeError(
        f"oputil set-role failed ({last_completed.returncode}):\n"
        f"stdout:\n{last_completed.stdout}\n"
        f"stderr:\n{last_completed.stderr}"
    )


def spawn_ssh(port: int, term_name: str = "xterm-256color", cols: int = 80, rows: int = 25) -> pexpect.spawn:
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
    env = os.environ.copy()
    env["TERM"] = term_name
    child = pexpect.spawn(cmd, cwd=str(ROOT), env=env, encoding="utf-8", timeout=25)
    child.setwinsize(rows, cols)
    idx = child.expect(
        [
            "Press any key to continue",
            "any other key to continue",
            "Handle:",
            r"[Pp]assword:",
            "WolfBBS Welcome",
            "Login",
            "Enter selection:",
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
                "WolfBBS Welcome",
                "Login",
            ]
        )
        if idx == 2:
            return child
        if idx in (3, 4):
            try:
                child.expect("Handle:")
                return child
            except pexpect.TIMEOUT:
                pass
    if idx in (4, 5):
        child.send("x")
        try:
            child.expect("Handle:")
            return child
        except pexpect.TIMEOUT:
            pass
    if idx == 6:
        child.send("Q")
        try:
            child.expect("Handle:")
            return child
        except pexpect.TIMEOUT:
            pass
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


def type_with_backspace(child: pexpect.spawn, value: str, extra_char: str = "x") -> None:
    child.send(value + extra_char)
    child.send("\x7f")
    child.sendline("")


def expect_main_menu_ready(child: pexpect.spawn) -> None:
    child.expect("Enter selection:")


def expect_after_optional_pager(child: pexpect.spawn, pattern: str, timeout: int = 25) -> None:
    while True:
        idx = child.expect([pattern, "-- More --"], timeout=timeout)
        if idx == 0:
            return
        child.send(" ")


def exit_optional_pager(child: pexpect.spawn, next_pattern: str, timeout: int = 25) -> None:
    while True:
        idx = child.expect([next_pattern, "-- More --"], timeout=timeout)
        if idx == 0:
            return
        child.send("Q")


def quick_jump(child: pexpect.spawn, target: str, heading: str) -> None:
    child.send("/")
    child.expect("Quick Jump Deck")
    child.expect("Feature or place")
    child.sendline(target)
    child.expect(heading)


def seed_file_fixture(files_root: Path) -> None:
    files_root.mkdir(parents=True, exist_ok=True)
    (files_root / "welcome-guide.txt").write_text("WolfBBS fixture file\n", encoding="utf-8")
    (files_root / "ansi-pack.ans").write_text("ANSI fixture file\n", encoding="utf-8")


def run_regular_user_smoke(child: pexpect.spawn) -> None:
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
    child.expect("Files & Downloads")
    child.sendline("Q")
    child.expect("Enter selection:")

    child.send("C")
    child.expect("Live Chat")
    child.send("Q")
    child.expect("Enter selection:")

    child.send("G")
    child.expect("Internet Tools")
    child.send("Q")
    child.expect("Enter selection:")

    child.send("D")
    child.expect("Games & Doors")
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
    child.expect("My Settings")
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
    child.expect("Feature or place")
    child.sendline("boards")
    child.expect("Select board ID")
    child.sendline("Q")
    child.expect("Enter selection:")

    child.send("X")
    child.expect("Config Center")
    child.expect("Guest tour enabled")
    child.send("x")
    child.expect("Enter selection:")


def run_regular_user_deep(child: pexpect.spawn) -> None:
    child.send("?")
    child.expect("Main Menu Key Guide")
    child.send("x")
    child.expect("Enter selection:")

    child.send("N")
    child.expect("Newscan Digest")
    child.send("x")
    child.expect("Enter selection:")

    # Boards: help, compose with edit helpers, paging, and search.
    child.send("M")
    child.expect("Select board ID")
    child.sendline("1")
    child.expect_exact("Commands: (N)ew")
    child.sendline("?")
    child.expect("Boards Navigation")
    child.send("x")
    child.expect_exact("Commands: (N)ew")
    child.sendline("N")
    child.expect("Subject:")
    type_with_backspace(child, "UX Subject")
    child.sendline("draft line to delete")
    child.sendline("/preview")
    child.expect("draft preview")
    child.sendline("/del")
    child.expect("Removed last line.")
    child.sendline("/help")
    child.expect("Compose helpers")
    for i in range(1, 26):
        child.sendline(f"line {i:02d} " + ("x" * 36))
    child.sendline(".")
    child.expect_exact("Commands: (N)ew")

    child.sendline("S")
    idx = child.expect(["Search query:", "Classic search is disabled by sysop. Press any key."])
    if idx == 0:
        child.sendline("line 01")
        child.expect("Classic Search Results")
        child.send("x")
    else:
        child.send("x")
    child.expect_exact("Commands: (N)ew")

    child.sendline("R")
    child.expect("Message ID")
    child.sendline("1")
    idx = child.expect([r"Long body detected; pager enabled\.", r"Command \(R/N/P/Q/\?\):"])
    if idx == 0:
        child.expect("More")
        child.send(" ")
        child.expect(r"Command \(R/N/P/Q/\?\):")
    child.send("Q")
    child.expect_exact("Commands: (N)ew")
    child.sendline("Q")
    child.expect("Select board ID")
    child.sendline("Q")
    child.expect("Enter selection:")

    # Mail: compose, input correction, compose helper commands, and delete.
    child.send("P")
    child.expect_exact("Commands: (C)ompose")
    child.sendline("?")
    child.expect("Private Mail Commands")
    child.send("x")
    child.expect_exact("Commands: (C)ompose")
    child.sendline("T")
    child.expect("Saved Reply Kits")
    child.expect("Command:")
    child.sendline("N")
    child.expect("Kit name:")
    child.sendline("Door Invite Kit")
    child.expect("Subject:")
    child.sendline("Meet me in the Door Hub")
    child.expect("Urgency")
    child.sendline("fyi")
    child.expect("Template body")
    child.sendline("Meet me in /doors after tonight's scan.")
    child.sendline(".")
    child.expect("Reply kit saved. Press any key.")
    child.send("x")
    child.expect("Command:")
    child.sendline("U1")
    child.expect("Loaded reply kit: Door Invite Kit")
    child.expect("To handle or external email:")
    type_with_backspace(child, "e2eadmin")
    child.expect("Subject")
    child.sendline("")
    child.expect("Urgency")
    child.sendline("")
    child.expect("Loaded kit body")
    child.sendline(".")
    child.expect("Mail sent.")
    child.expect_exact("Commands: (C)ompose")
    child.sendline("C")
    child.expect("To handle or external email:")
    type_with_backspace(child, "e2eadmin")
    child.expect("Subject:")
    type_with_backspace(child, "Mail UX Check")
    child.expect("Urgency")
    child.sendline("")
    child.sendline("draft mail line")
    child.sendline("/preview")
    child.expect("draft preview")
    child.sendline("/del")
    child.expect("Removed last line.")
    child.sendline("final mail body")
    child.sendline(".")
    child.expect_exact("Commands: (C)ompose")
    child.sendline("R")
    child.expect("Mail ID:")
    child.sendline("1")
    idx = child.expect_exact(["Reader commands: (P) reply  (D) delete  (Q) back", "Mail not found. Press any key."])
    if idx != 0:
        raise RuntimeError("mail reader did not open expected message ID")
    child.send("D")
    child.expect("Mail deleted. Press any key.")
    child.send("x")
    child.expect_exact("Commands: (C)ompose")
    child.sendline("Q")
    child.expect("Enter selection:")

    # Files: help/search/indexed queue surface/download queue surface.
    child.send("F")
    child.expect("Files & Downloads")
    child.expect("Selection:")
    child.sendline("?")
    child.expect("Files & Downloads Commands")
    child.send("x")
    child.expect("Selection:")
    child.sendline("R")
    child.expect("Recent Files")
    child.send("x")
    child.expect("Selection:")
    child.sendline("N")
    child.expect("New Files")
    child.send("x")
    child.expect("Selection:")
    child.sendline("C")
    child.expect("Featured Collections")
    child.sendline("Q")
    child.expect("Selection:")
    child.sendline("O")
    child.expect("Offline Center")
    child.send("J")
    child.expect("Offline Packet JSON")
    expect_after_optional_pager(child, "Save artifact to offline path")
    child.sendline("N")
    child.expect("Selection:")
    child.send("T")
    child.expect("Offline Packet Text")
    expect_after_optional_pager(child, "Save artifact to offline path")
    child.sendline("Y")
    child.expect("Saved text packet:")
    child.send("x")
    child.expect("Selection:")
    child.send("I")
    child.expect("Paste JSON reply payload")
    child.sendline('[{"to":"e2eadmin","subject":"Offline reply","body":"hello from packet"}]')
    child.sendline(".")
    child.expect("Imported 1 offline mail replie")
    child.send("x")
    child.expect("Selection:")
    child.send("Q")
    child.expect("Files & Downloads")
    child.expect("Selection:")
    child.sendline("S")
    child.expect("Search query:")
    type_with_backspace(child, "welcome-guide")
    child.expect("File Search")
    child.send("x")
    child.expect("Selection:")
    child.sendline("I")
    child.expect("Indexed FileBase")
    child.expect_exact("Commands: (S)earch  [ID] queue add  (Q)uit")
    child.sendline("Q")
    child.expect("Selection:")
    child.sendline("D")
    child.expect("Download Queue")
    child.expect("Commands: R<ID> remove  T<ID> ticket  B batch tip  Q quit")
    child.sendline("B")
    child.expect("Batch ZIP:")
    child.send("x")
    child.expect("Selection:")
    child.sendline("Q")
    child.expect("Files & Downloads")
    child.expect("Selection:")
    child.sendline("1")
    child.expect("Files: Uploads")
    child.expect("Selection:")
    child.sendline("Q")
    child.expect("Selection:")
    child.sendline("Q")
    child.expect("Enter selection:")

    # Chat: multi-channel deck, roster, slot switching, leave fallback, send, refresh.
    child.send("C")
    child.expect("Live Chat")
    child.expect("Current room: #lobby")
    child.expect("Open Rooms")
    child.send("?")
    child.expect("Live Chat Commands")
    child.send("x")
    child.expect("Selection:")
    child.send("O")
    child.expect("Online users:")
    child.expect("e2eadmin")
    child.expect("Press any key.")
    child.send("x")
    child.expect("Selection:")
    child.send("J")
    child.expect("Open or join room")
    child.sendline("#ux")
    child.expect("Current room: #ux")
    child.expect("#ux")
    child.expect("Selection:")
    child.send("2")
    child.expect("Current room: #lobby")
    child.expect("Selection:")
    child.send("J")
    child.expect("Open or join room")
    child.sendline("#art")
    child.expect("Current room: #art")
    child.expect("#art")
    child.expect("Selection:")
    child.send("S")
    child.expect("Message:")
    type_with_backspace(child, "hello art channel")
    child.expect("Selection:")
    child.send("L")
    child.expect("Current room:")
    child.expect("Selection:")
    child.send("R")
    child.expect("Selection:")
    child.send("Q")
    child.expect("Enter selection:")

    # Gateway: URL validation and compose validation.
    child.send("G")
    child.expect("Internet Tools")
    child.send("W")
    child.expect("URL:")
    type_with_backspace(child, "http://127.0.0.1")
    child.expect("Gateway blocked:")
    child.send("x")
    child.expect("Enter selection:")
    child.send("G")
    child.expect("Internet Tools")
    child.send("E")
    child.expect("To external email:")
    child.sendline("")
    child.expect("Subject:")
    child.sendline("gateway validation")
    child.expect("Body")
    child.sendline("body")
    child.sendline(".")
    child.expect("To/subject/body required. Press any key.")
    child.send("x")
    child.expect("Enter selection:")

    child.send("G")
    child.expect("Internet Tools")
    child.send("F")
    child.expect("Feed URL:")
    child.sendline("")
    child.expect("Feed URL required. Press any key.")
    child.send("x")
    child.expect("Enter selection:")

    child.send("G")
    child.expect("Internet Tools")
    child.send("S")
    child.expect("Article URL:")
    child.sendline("")
    child.expect("Article URL required. Press any key.")
    child.send("x")
    child.expect("Enter selection:")

    child.send("G")
    child.expect("Internet Tools")
    child.send("J")
    child.expect("JSON URL:")
    child.sendline("")
    child.expect("JSON URL required. Press any key.")
    child.send("x")
    child.expect("Enter selection:")

    child.send("G")
    child.expect("Internet Tools")
    child.send("A")
    ai_idx = child.expect(
        [
            "AI gateway is disabled. Configure /admin/gateways and set API key. Press any key.",
            "Prompt:",
        ]
    )
    if ai_idx == 0:
        child.send("x")
    else:
        child.sendline("")
        child.expect("Prompt is required. Press any key.")
        child.send("x")
    child.expect("Enter selection:")

    # Doors + caller visibility panels.
    child.send("D")
    child.expect("Games & Doors")
    child.send("?")
    child.expect("Games & Doors Commands")
    child.send("x")
    child.expect("Selection:")
    child.send("R")
    child.expect("Enter selection:")

    child.send("L")
    child.expect("Last Callers")
    child.expect("Orig")
    child.send("x")
    child.expect("Enter selection:")

    child.send("W")
    child.expect("Who's Online")
    child.expect("Orig")
    child.send("P")
    child.expect("Handle to page:")
    child.sendline("e2eadmin")
    child.expect("Pick another active handle. Press any key.")
    child.send("x")
    child.expect_exact("(P)age caller  (R)efresh  (Q)uit:")
    child.send("Q")
    child.expect("Enter selection:")

    # Status/config/settings and quick jump paths.
    child.send("Y")
    child.expect("Status Center")
    child.expect("Quick jump state")
    child.send("x")
    child.expect("Enter selection:")

    child.send("X")
    child.expect("Config Center")
    child.send("x")
    child.expect("Enter selection:")

    child.send("S")
    child.expect("My Settings")
    child.send("B")
    child.expect("Bookmarks")
    child.sendline("Q")
    child.expect("Selection:")
    child.send("O")
    child.expect("Circles")
    child.sendline("Q")
    child.expect("Selection:")
    child.send("X")
    child.expect("Profile Export")
    expect_after_optional_pager(child, "Save JSON export to offline path")
    child.sendline("N")
    child.expect("Selection:")
    child.send("E")
    child.expect("Attention Export")
    expect_after_optional_pager(child, "Save JSON export to offline path")
    child.sendline("N")
    child.expect("Selection:")
    child.send("A")
    child.expect("My Settings")
    child.send("P")
    child.expect("My Settings")
    child.send("C")
    child.expect("My Settings")
    child.send("S")
    child.expect("Preferences saved. Press any key.")
    child.send("x")
    expect_main_menu_ready(child)

    child.send("O")
    child.expect("Offline Center")
    child.send("Q")
    expect_main_menu_ready(child)
    child.send("V")
    child.expect("Product Showcase")
    child.send("x")
    expect_main_menu_ready(child)

    # Quick-jump parity and terminal resize/redraw.
    quick_jump(child, "collections", "Featured Collections")
    child.sendline("Q")
    expect_main_menu_ready(child)
    quick_jump(child, "offline", "Offline Center")
    child.send("Q")
    expect_main_menu_ready(child)
    quick_jump(child, "showcase", "Product Showcase")
    child.send("x")
    expect_main_menu_ready(child)
    child.setwinsize(18, 34)
    time.sleep(0.2)
    child.send("?")
    child.expect("Main Menu Key Guide")
    child.send("x")
    expect_main_menu_ready(child)
    child.send("Y")
    child.expect("Status Center")
    child.send("J")
    expect_after_optional_pager(child, '"area"')
    exit_optional_pager(child, "Selection:")
    child.send("Q")
    expect_main_menu_ready(child)


def run_regular_user_flow(
    port: int,
    *,
    term_name: str,
    cols: int,
    rows: int,
    create_if_missing: bool,
    deep: bool,
) -> None:
    child = spawn_ssh(port, term_name=term_name, cols=cols, rows=rows)
    try:
        complete_login(child, "e2eadmin", "password123", create_if_missing=create_if_missing)
        if deep:
            run_regular_user_deep(child)
        else:
            run_regular_user_smoke(child)
        child.send("Q")
        child.expect(pexpect.EOF)
    finally:
        child.close(force=True)


def run_admin_flow(port: int, *, term_name: str, cols: int, rows: int) -> None:
    child = spawn_ssh(port, term_name=term_name, cols=cols, rows=rows)
    try:
        complete_login(child, "e2eadmin", "password123", create_if_missing=False)
        child.send("A")
        idx = child.expect(
            [
                "Admin Center",
                "Admin Control Deck",
                "Use web admin at /admin for full sysop controls.",
            ]
        )
        if idx == 2:
            child.send("x")
            expect_main_menu_ready(child)
        else:
            for key, ready_pattern, exit_action in [
                ("C", "Command:", "BACK\n"),
                ("D", "Command:", "BACK\n"),
                ("E", "Command:", "BACK\n"),
                ("F", "Command:", "BACK\n"),
                ("G", "Command:", "BACK\n"),
                ("H", "Command:", "BACK\n"),
                ("I", "Command:", "BACK\n"),
                ("J", "Command:", "BACK\n"),
                ("L", "Press any key.", "x"),
                ("O", "Command:", "BACK\n"),
                ("R", "Selection:", "Q"),
                ("U", "Command:", "BACK\n"),
                ("B", "Command:", "BACK\n"),
                ("M", "Command:", "BACK\n"),
                ("A", "Press any key.", "x"),
                ("N", "Press any key.", "x"),
            ]:
                child.send(key)
                child.expect(ready_pattern)
                child.send(exit_action)
                child.expect("Admin Control Deck")
            child.send("Q")
            expect_main_menu_ready(child)
        child.send("Q")
        child.expect(pexpect.EOF)
    finally:
        child.close(force=True)


def main() -> int:
    tmp_root = Path(tempfile.mkdtemp(prefix="wolfbbs-tui-e2e-"))
    db_path = tmp_root / "wolfbbs-e2e.db"
    log_path = tmp_root / "wolfbbs-e2e.log"
    files_root = tmp_root / "files"
    seed_file_fixture(files_root)
    first_port = free_port()
    second_port = free_port()
    third_port = free_port()
    while second_port == first_port:
        second_port = free_port()
    while third_port in {first_port, second_port}:
        third_port = free_port()
    proc = None
    try:
        proc = start_bbs_server(db_path, first_port, log_path, files_root)
        run_regular_user_flow(first_port, term_name="ansi", cols=60, rows=24, create_if_missing=True, deep=False)
        run_regular_user_flow(first_port, term_name="vt100", cols=40, rows=22, create_if_missing=False, deep=False)
        stop_process(proc)
        proc = None

        proc = start_bbs_server(db_path, second_port, log_path, files_root)
        run_regular_user_flow(second_port, term_name="xterm-256color", cols=100, rows=30, create_if_missing=False, deep=True)
        stop_process(proc)
        proc = None

        run_oputil_set_role(db_path, "e2eadmin", "sysop")

        proc = start_bbs_server(db_path, third_port, log_path, files_root)
        run_admin_flow(third_port, term_name="xterm-256color", cols=100, rows=30)
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
