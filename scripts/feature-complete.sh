#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

WEB_TIMEOUT_SECONDS="${WOLFBBS_FEATURE_COMPLETE_WEB_TIMEOUT_SECONDS:-900}"

if ! [[ "$WEB_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || [[ "$WEB_TIMEOUT_SECONDS" -le 0 ]]; then
  echo "WOLFBBS_FEATURE_COMPLETE_WEB_TIMEOUT_SECONDS must be a positive integer" >&2
  exit 2
fi

echo "[feature-complete 1/6] go test ./..."
go test ./...

echo "[feature-complete 2/6] scripts/verify.sh --fast"
scripts/verify.sh --fast

echo "[feature-complete 3/6] scripts/qa-functional.sh --with-web-e2e --with-manual-auto"
scripts/qa-functional.sh --with-web-e2e --with-manual-auto --web-timeout "$WEB_TIMEOUT_SECONDS"

echo "[feature-complete 4/6] scripts/verify.sh --smoke"
scripts/verify.sh --smoke

echo "[feature-complete 5/6] scripts/security-audit.sh"
scripts/security-audit.sh

echo "[feature-complete 6/6] complete"
echo "PASS feature-complete"
