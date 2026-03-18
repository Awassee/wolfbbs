#!/usr/bin/env python3
"""Minimal IRC protocol smoke test for WolfBBS."""

from __future__ import annotations

import os
import socket
import sys
import time
from typing import Callable, List


class IRCClient:
    def __init__(self, host: str, port: int, timeout: float = 6.0) -> None:
        self.sock = socket.create_connection((host, port), timeout=timeout)
        self.sock.settimeout(timeout)
        self.buf = b""
        self.lines: List[str] = []

    def send(self, line: str) -> None:
        payload = f"{line}\r\n".encode("utf-8")
        self.sock.sendall(payload)

    def close(self) -> None:
        try:
            self.sock.close()
        except OSError:
            pass

    def _drain_once(self) -> bool:
        try:
            chunk = self.sock.recv(4096)
        except socket.timeout:
            return False
        if not chunk:
            return False
        self.buf += chunk
        while b"\n" in self.buf:
            raw, self.buf = self.buf.split(b"\n", 1)
            line = raw.decode("utf-8", errors="replace").strip("\r")
            self.lines.append(line)
        return True

    def read_until(self, predicate: Callable[[str], bool], timeout: float = 6.0) -> bool:
        deadline = time.time() + timeout
        while time.time() < deadline:
            for line in self.lines:
                if predicate(line):
                    return True
            if not self._drain_once():
                time.sleep(0.03)
        return any(predicate(line) for line in self.lines)

    def has_numeric(self, code: str) -> bool:
        needle = f" {code} "
        return any(needle in line for line in self.lines)


def fail(msg: str, lines: List[str]) -> int:
    sys.stderr.write(f"FAIL: {msg}\n")
    if lines:
        sys.stderr.write("--- IRC transcript ---\n")
        for line in lines[-80:]:
            sys.stderr.write(line + "\n")
    return 1


def main() -> int:
    host = os.environ.get("IRC_HOST", "127.0.0.1")
    port = int(os.environ.get("IRC_PORT", "6667"))
    auth_user = os.environ.get("IRC_TEST_USER", "sysop")
    auth_pass = os.environ.get("IRC_TEST_PASS", "password123")

    # 1) Unauthenticated JOIN should fail with 451.
    try:
        unauth = IRCClient(host, port)
    except OSError as exc:
        return fail(
            f"could not connect to IRC endpoint {host}:{port} ({exc}); start the stack first (docker compose up -d --build)",
            [],
        )
    try:
        unauth.send("NICK unauthcheck")
        unauth.send("USER unauthcheck 0 * :unauth")
        unauth.send("JOIN #lobby")
        if not unauth.read_until(lambda line: " 451 " in line, timeout=5.0):
            return fail("unauthenticated JOIN did not return 451", unauth.lines)
    finally:
        unauth.close()

    # 2) Authenticated user should get welcome/motd numerics, join, names.
    try:
        authed = IRCClient(host, port)
    except OSError as exc:
        return fail(
            f"could not reconnect to IRC endpoint {host}:{port} ({exc})",
            [],
        )
    try:
        authed.send(f"PASS {auth_pass}")
        authed.send(f"NICK {auth_user}")
        authed.send(f"USER {auth_user} 0 * :{auth_user}")
        if not authed.read_until(lambda line: " 001 " in line, timeout=5.0):
            return fail("missing 001 welcome numeric", authed.lines)
        if not authed.read_until(lambda line: " 376 " in line, timeout=5.0):
            return fail("missing 376 end-of-motd numeric", authed.lines)

        authed.send("JOIN #lobby")
        if not authed.read_until(lambda line: " 353 " in line, timeout=5.0):
            return fail("missing 353 names reply after JOIN", authed.lines)
        if not authed.read_until(lambda line: " 366 " in line, timeout=5.0):
            return fail("missing 366 end-of-names reply after JOIN", authed.lines)

        authed.send("PRIVMSG #lobby :hello from verify")
        if not authed.read_until(lambda line: "PRIVMSG #lobby :hello from verify" in line, timeout=5.0):
            return fail("missing PRIVMSG echo in channel", authed.lines)

        # 3) Flood test: server should throttle with numeric reply.
        throttled = False
        for idx in range(24):
            authed.send(f"PRIVMSG #lobby :flood-{idx}")
        authed.read_until(lambda line: (" 432 " in line) or (" 439 " in line), timeout=4.0)
        for line in authed.lines:
            if " 432 " in line or " 439 " in line:
                throttled = True
                break
        if not throttled:
            return fail("flood burst did not trigger IRC throttle numeric (432/439)", authed.lines)

    finally:
        authed.close()

    print("IRC scripted checks passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
