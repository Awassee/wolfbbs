#!/usr/bin/env python3
"""Functional IRC protocol checks for WolfBBS."""

from __future__ import annotations

import base64
import os
import socket
import sys
import time
from dataclasses import dataclass, field
from typing import Callable, List

PACE_SECONDS = float(os.environ.get("IRC_TEST_PACE_SECONDS", "0.24"))
POST_FLOOD_COOLDOWN_SECONDS = float(os.environ.get("IRC_TEST_POST_FLOOD_COOLDOWN_SECONDS", "2.3"))


def now_utc_suffix() -> str:
    return str(int(time.time() * 1000))


@dataclass
class IRCClient:
    host: str
    port: int
    timeout: float = 6.0
    sock: socket.socket = field(init=False)
    buf: bytes = field(default=b"", init=False)
    lines: List[str] = field(default_factory=list, init=False)
    cursor: int = field(default=0, init=False)

    def __post_init__(self) -> None:
        self.sock = socket.create_connection((self.host, self.port), timeout=self.timeout)
        self.sock.settimeout(self.timeout)

    def send(self, line: str) -> None:
        self.sock.sendall(f"{line}\r\n".encode("utf-8"))

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
            if line:
                self.lines.append(line)
        return True

    def wait_for(self, predicate: Callable[[str], bool], timeout: float = 6.0) -> str | None:
        deadline = time.time() + timeout
        scan_idx = self.cursor
        while time.time() < deadline:
            for idx in range(scan_idx, len(self.lines)):
                line = self.lines[idx]
                if predicate(line):
                    self.cursor = idx + 1
                    return line
            scan_idx = len(self.lines)
            if not self._drain_once():
                time.sleep(0.03)
        for idx in range(scan_idx, len(self.lines)):
            line = self.lines[idx]
            if predicate(line):
                self.cursor = idx + 1
                return line
        return None

    def wait_contains(self, needle: str, timeout: float = 6.0) -> str | None:
        return self.wait_for(lambda line: needle in line, timeout=timeout)

    def transcript(self, last: int = 120) -> List[str]:
        return self.lines[-last:]


def fail(msg: str, clients: List[IRCClient]) -> int:
    sys.stderr.write(f"FAIL: {msg}\n")
    for idx, client in enumerate(clients, start=1):
        tail = client.transcript()
        if not tail:
            continue
        sys.stderr.write(f"--- IRC transcript client#{idx} ---\n")
        for line in tail:
            sys.stderr.write(line + "\n")
    return 1


def expect(client: IRCClient, needle: str, timeout: float, message: str, clients: List[IRCClient]) -> int:
    if client.wait_contains(needle, timeout=timeout) is None:
        return fail(message, clients)
    return 0


def auth_register(client: IRCClient, handle: str, password: str) -> None:
	client.send(f"PASS {password}")
	client.send(f"NICK {handle}")
	client.send(f"USER {handle} 0 * :{handle}")


def send_paced(client: IRCClient, line: str, delay: float = PACE_SECONDS) -> None:
    client.send(line)
    if delay > 0:
        time.sleep(delay)


def run_basic_auth_matrix(host: str, port: int, auth_user: str, auth_pass: str) -> int:
    clients: List[IRCClient] = []
    try:
        unauth = IRCClient(host, port)
        clients.append(unauth)
    except OSError as exc:
        return fail(
            f"could not connect to IRC endpoint {host}:{port} ({exc}); start the stack first (docker compose up -d --build)",
            clients,
        )

    try:
        send_paced(unauth, "NICK unauthcheck")
        send_paced(unauth, "USER unauthcheck 0 * :unauth")
        send_paced(unauth, "JOIN #lobby")
        if expect(unauth, " 451 ", 5.0, "unauthenticated JOIN did not return 451", clients):
            return 1
        send_paced(unauth, "PRIVMSG #lobby :unauth message")
        if expect(unauth, " 451 ", 5.0, "unauthenticated PRIVMSG did not return 451", clients):
            return 1
    finally:
        unauth.close()

    authed = IRCClient(host, port)
    clients.append(authed)
    try:
        auth_register(authed, auth_user, auth_pass)
        if expect(authed, " 001 ", 5.0, "missing 001 welcome numeric", clients):
            return 1
        if expect(authed, " 376 ", 5.0, "missing 376 end-of-motd numeric", clients):
            return 1

        send_paced(authed, "JOIN #lobby")
        if expect(authed, " 353 ", 5.0, "missing 353 names reply after JOIN", clients):
            return 1
        if expect(authed, " 366 ", 5.0, "missing 366 end-of-names after JOIN", clients):
            return 1
        if expect(authed, " 332 ", 5.0, "missing 332 topic reply after JOIN", clients):
            return 1

        send_paced(authed, "NAMES #lobby")
        if expect(authed, " 353 ", 5.0, "missing 353 after NAMES", clients):
            return 1
        if expect(authed, " 366 ", 5.0, "missing 366 after NAMES", clients):
            return 1

        send_paced(authed, "LIST")
        if expect(authed, " 322 ", 5.0, "missing 322 during LIST", clients):
            return 1
        if expect(authed, " 323 ", 5.0, "missing 323 end of LIST", clients):
            return 1

        send_paced(authed, f"WHOIS {auth_user}")
        if expect(authed, " 311 ", 5.0, "missing 311 in WHOIS", clients):
            return 1
        if expect(authed, " 312 ", 5.0, "missing 312 in WHOIS", clients):
            return 1
        if expect(authed, " 317 ", 5.0, "missing 317 in WHOIS", clients):
            return 1
        if expect(authed, " 318 ", 5.0, "missing 318 in WHOIS", clients):
            return 1

        send_paced(authed, "TOPIC #lobby")
        if expect(authed, " 332 ", 5.0, "missing 332 after TOPIC", clients):
            return 1
        send_paced(authed, "MODE #lobby")
        if expect(authed, " 324 ", 5.0, "missing 324 after MODE", clients):
            return 1

        send_paced(authed, "AWAY :qa away test")
        if expect(authed, " 306 ", 5.0, "missing 306 after AWAY set", clients):
            return 1
        send_paced(authed, "AWAY")
        if expect(authed, " 305 ", 5.0, "missing 305 after AWAY clear", clients):
            return 1

        time.sleep(1.6)
        marker = f"verify-message-{now_utc_suffix()}"
        send_paced(authed, f"PRIVMSG #lobby :{marker}")
        if expect(authed, f"PRIVMSG #lobby :{marker}", 5.0, "missing PRIVMSG echo in channel", clients):
            return 1

        notice_marker = f"verify-notice-{now_utc_suffix()}"
        send_paced(authed, f"NOTICE #lobby :{notice_marker}")
        if expect(authed, f"NOTICE #lobby :{notice_marker}", 5.0, "missing NOTICE echo in channel", clients):
            return 1

        ping_marker = f"clientprobe-{now_utc_suffix()}"
        send_paced(authed, f"PING :{ping_marker}")
        if expect(authed, " PONG :", 5.0, "missing PONG after PING", clients):
            return 1

        send_paced(authed, "FOOBAR")
        if expect(authed, " 421 ", 5.0, "missing 421 for unknown command", clients):
            return 1

        send_paced(authed, "PART #lobby")
        if expect(authed, " PART #lobby ", 5.0, "missing PART acknowledgement", clients):
            return 1
        send_paced(authed, "JOIN #lobby")
        if expect(authed, " 366 ", 5.0, "missing rejoin completion after PART", clients):
            return 1

        throttled = False
        for idx in range(24):
            authed.send(f"PRIVMSG #lobby :flood-{idx}")
        authed.wait_for(lambda line: (" 432 " in line) or (" 439 " in line), timeout=4.0)
        throttled = any((" 432 " in line) or (" 439 " in line) for line in authed.lines)
        if not throttled:
            return fail("flood burst did not trigger IRC throttle numeric (432/439)", clients)
    finally:
        authed.close()
    return 0


def run_sasl_matrix(host: str, port: int, auth_user: str, auth_pass: str) -> int:
    if POST_FLOOD_COOLDOWN_SECONDS > 0:
        time.sleep(POST_FLOOD_COOLDOWN_SECONDS)
    clients: List[IRCClient] = []
    try:
        sasl = IRCClient(host, port)
    except OSError as exc:
        return fail(f"could not connect for SASL matrix ({exc})", clients)
    clients.append(sasl)
    try:
        send_paced(sasl, "CAP LS 302")
        if expect(sasl, " CAP ", 5.0, "missing CAP response", clients):
            return 1
        send_paced(sasl, "CAP REQ :sasl")
        if expect(sasl, " ACK :sasl", 5.0, "missing CAP ACK :sasl", clients):
            return 1
        send_paced(sasl, "AUTHENTICATE PLAIN")
        if expect(sasl, "AUTHENTICATE +", 5.0, "missing AUTHENTICATE + challenge", clients):
            return 1
        payload = base64.b64encode(f"\x00{auth_user}\x00{auth_pass}".encode("utf-8")).decode("ascii")
        send_paced(sasl, f"AUTHENTICATE {payload}")
        if expect(sasl, " 903 ", 5.0, "missing SASL success numeric 903", clients):
            return 1
        if not any(" 001 " in line for line in sasl.lines):
            if expect(sasl, " 001 ", 5.0, "missing 001 after SASL auth registration", clients):
                return 1
        send_paced(sasl, "CAP END")
        send_paced(sasl, "JOIN #lobby")
        if expect(sasl, " 353 ", 5.0, "missing 353 after SASL JOIN", clients):
            return 1
        if expect(sasl, " 366 ", 5.0, "missing 366 after SASL JOIN", clients):
            return 1
    finally:
        sasl.close()
    return 0


def main() -> int:
    host = os.environ.get("IRC_HOST", "127.0.0.1")
    port = int(os.environ.get("IRC_PORT", "6667"))
    auth_user = os.environ.get("IRC_TEST_USER", "sysop")
    auth_pass = os.environ.get("IRC_TEST_PASS", "password123")

    rc = run_basic_auth_matrix(host, port, auth_user, auth_pass)
    if rc != 0:
        return rc
    rc = run_sasl_matrix(host, port, auth_user, auth_pass)
    if rc != 0:
        return rc

    print("IRC functional checks passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
