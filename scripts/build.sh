#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

MODE="quick"
RUN_SMOKE=false
RUN_E2E=true
RUN_WEB_E2E=true
WEB_TIMEOUT="${WOLFBBS_WEB_E2E_TIMEOUT_SECONDS:-600}"
ALLOW_UNSUPPORTED_NODE=false

usage() {
  cat <<'USAGE'
WolfBBS build + QA runner

Usage:
  scripts/build.sh [options]

Options:
  --quick                   default: go test/build + verify --fast + terminal e2e
  --full                    quick mode + verify --smoke + web e2e
  --skip-e2e                skip all e2e checks
  --skip-web-e2e            run terminal e2e only
  --allow-unsupported-node  pass through to web e2e runner (for Node >=25)
  --web-timeout <seconds>   web e2e timeout (default: 600)
  -h, --help                show this help
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --quick)
      MODE="quick"
      ;;
    --full)
      MODE="full"
      RUN_SMOKE=true
      ;;
    --skip-e2e)
      RUN_E2E=false
      RUN_WEB_E2E=false
      ;;
    --skip-web-e2e)
      RUN_WEB_E2E=false
      ;;
    --allow-unsupported-node)
      ALLOW_UNSUPPORTED_NODE=true
      ;;
    --web-timeout)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for --web-timeout" >&2
        exit 2
      fi
      WEB_TIMEOUT="$2"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

if ! [[ "$WEB_TIMEOUT" =~ ^[0-9]+$ ]] || [[ "$WEB_TIMEOUT" -le 0 ]]; then
  echo "--web-timeout must be a positive integer." >&2
  exit 2
fi

echo "[1/6] go test ./..."
go test ./... -count=1 -timeout=420s

echo "[2/6] go build ./..."
go build ./...

echo "[3/6] scripts/verify.sh --fast"
scripts/verify.sh --fast

if [[ "$RUN_SMOKE" == "true" ]]; then
  echo "[4/6] scripts/verify.sh --smoke"
  scripts/verify.sh --smoke
else
  echo "[4/6] smoke verify skipped (use --full)"
fi

if [[ "$RUN_E2E" != "true" ]]; then
  echo "[5/6] terminal e2e skipped (--skip-e2e)"
  echo "[6/6] web e2e skipped (--skip-e2e)"
  echo "PASS build.sh (${MODE})"
  exit 0
fi

echo "[5/6] terminal e2e"
scripts/run-e2e.sh --no-go --no-web

if [[ "$RUN_WEB_E2E" != "true" ]]; then
  echo "[6/6] web e2e skipped (--skip-web-e2e)"
  echo "PASS build.sh (${MODE})"
  exit 0
fi

web_args=(--no-go --no-tui --web-timeout "$WEB_TIMEOUT")
if [[ "$ALLOW_UNSUPPORTED_NODE" == "true" ]]; then
  web_args+=(--allow-unsupported-node)
fi

if [[ "$(uname -s)" == "Darwin" ]] && command -v brew >/dev/null 2>&1; then
  node24_prefix="$(brew --prefix node@24 2>/dev/null || true)"
  if [[ -n "$node24_prefix" && -x "$node24_prefix/bin/node" && -x "$node24_prefix/bin/npm" ]]; then
    echo "[6/6] web e2e (Node 24 toolchain)"
    WOLFBBS_NODE_BIN="$node24_prefix/bin/node" WOLFBBS_NPM_BIN="$node24_prefix/bin/npm" scripts/run-e2e.sh "${web_args[@]}"
    echo "PASS build.sh (${MODE})"
    exit 0
  fi
fi

echo "[6/6] web e2e"
scripts/run-e2e.sh "${web_args[@]}"

echo "PASS build.sh (${MODE})"
