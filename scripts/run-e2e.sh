#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

run_go_tests=true
run_tui=true
run_web=true
web_timeout_seconds="${WOLFBBS_WEB_E2E_TIMEOUT_SECONDS:-900}"
allow_unsupported_node="${WOLFBBS_ALLOW_UNSUPPORTED_NODE:-false}"

has_cmd() {
  command -v "$1" >/dev/null 2>&1
}

run_with_timeout() {
  local seconds="$1"
  shift
  if has_cmd timeout; then
    timeout "$seconds" "$@"
    return $?
  fi
  if has_cmd gtimeout; then
    gtimeout "$seconds" "$@"
    return $?
  fi

  "$@" &
  local pid=$!
  local elapsed=0
  while kill -0 "$pid" >/dev/null 2>&1; do
    if ((elapsed >= seconds)); then
      kill -TERM "$pid" >/dev/null 2>&1 || true
      sleep 1
      kill -KILL "$pid" >/dev/null 2>&1 || true
      wait "$pid" 2>/dev/null || true
      return 124
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  wait "$pid"
}

detect_node_major() {
  node -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo "0"
}

usage() {
  cat <<'USAGE'
WolfBBS end-to-end runner

Usage:
  scripts/run-e2e.sh [options]

Options:
  --no-go        Skip go test ./...
  --no-tui       Skip terminal pexpect tests
  --no-web       Skip Playwright tests
  --web-timeout <seconds>
                 Timeout for each Playwright npm step (default: \$WOLFBBS_WEB_E2E_TIMEOUT_SECONDS or 900)
  --allow-unsupported-node
                 Allow Playwright run on Node >= 25 (may hang in some environments)
  -h, --help     Show help
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-go)
      run_go_tests=false
      ;;
    --no-tui)
      run_tui=false
      ;;
    --no-web)
      run_web=false
      ;;
    --web-timeout)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for --web-timeout" >&2
        exit 2
      fi
      web_timeout_seconds="$2"
      shift
      ;;
    --allow-unsupported-node)
      allow_unsupported_node=true
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

if ! [[ "$web_timeout_seconds" =~ ^[0-9]+$ ]] || [[ "$web_timeout_seconds" -le 0 ]]; then
  echo "--web-timeout must be a positive integer; got: $web_timeout_seconds" >&2
  exit 2
fi

if [[ "$run_go_tests" == "true" ]]; then
  echo "[1/3] go test ./..."
  go test ./...
fi

if [[ "$run_tui" == "true" ]]; then
  echo "[2/3] terminal e2e (pexpect)"
  python3 scripts/test_tui_pexpect.py
fi

if [[ "$run_web" == "true" ]]; then
  if ! command -v npm >/dev/null 2>&1; then
    echo "npm is required for web e2e. Install Node.js >= 18." >&2
    exit 1
  fi
  local_node_major="$(detect_node_major)"
  if [[ "$local_node_major" =~ ^[0-9]+$ ]] && [[ "$local_node_major" -ge 25 ]] && [[ "$allow_unsupported_node" != "true" ]]; then
    echo "Node.js $local_node_major detected." >&2
    echo "Playwright is unstable on Node >= 25 in this environment; use Node 22/24 or pass --allow-unsupported-node." >&2
    exit 1
  fi
  echo "[3/3] web e2e (Playwright)"
  if ! run_with_timeout "$web_timeout_seconds" npm --prefix e2e/web install; then
    echo "web e2e dependency install failed or timed out" >&2
    exit 1
  fi
  if ! run_with_timeout "$web_timeout_seconds" npm --prefix e2e/web run install:browsers; then
    echo "web e2e browser install failed or timed out" >&2
    exit 1
  fi
  if ! run_with_timeout "$web_timeout_seconds" npm --prefix e2e/web test; then
    echo "web e2e test run failed or timed out" >&2
    exit 1
  fi
fi

echo "PASS end-to-end suite"
