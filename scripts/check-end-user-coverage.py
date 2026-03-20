#!/usr/bin/env python3
from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REGISTRY = ROOT / 'docs' / 'function-registry.json'
SCREENS = ROOT / 'internal' / 'ui' / 'screens.go'

CORE_ROUTES = {
    'web:/boards': 'message boards',
    'web:/mail': 'private mail',
    'web:/chat': 'live chat',
    'web:/doors': 'games and doors',
    'web:/today': 'today brief',
    'web:/events': 'events',
    'web:/settings': 'settings',
    'web:/offline': 'offline center',
    'web:/collections': 'collections',
    'web:/bookmarks': 'bookmarks',
    'web:/circles': 'circles',
    'web:/showcase': 'showcase',
    'web:/attention': 'attention center',
    'web:/profile/export': 'profile export',
    'web:/attention/export': 'attention export',
}

MENU_LABELS = [
    'Read Boards',
    'Chat Rooms',
    'Files & Downloads',
    'Games & Doors',
    'Internet Tools',
    "What's New",
    'My Activity',
    'Offline Packets',
    'Find a Feature',
]


def main() -> int:
    if not REGISTRY.exists():
        print(f'FAIL registry missing: {REGISTRY}', file=sys.stderr)
        return 1
    payload = json.loads(REGISTRY.read_text(encoding='utf-8'))
    entries = {row['id']: row for row in payload.get('entries', [])}
    failures: list[str] = []

    for route_id, label in CORE_ROUTES.items():
        row = entries.get(route_id)
        if row is None:
            failures.append(f'missing registry entry for {route_id} ({label})')
            continue
        if not row.get('user_tui'):
            failures.append(f'user_tui coverage missing for {route_id} ({label})')
        if not row.get('user_web'):
            failures.append(f'user_web coverage missing for {route_id} ({label})')

    screen_text = SCREENS.read_text(encoding='utf-8') if SCREENS.exists() else ''
    for label in MENU_LABELS:
        if label not in screen_text:
            failures.append(f'main menu label missing from terminal UI contract: {label}')

    if failures:
        for failure in failures:
            print(f'FAIL {failure}', file=sys.stderr)
        return 1

    for route_id, label in CORE_ROUTES.items():
        print(f'PASS {route_id} - {label} reachable in user web and user terminal')
    for label in MENU_LABELS:
        print(f'PASS menu label - {label}')
    print('PASS end-user coverage audit')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
