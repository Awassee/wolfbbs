#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

run_go_tests=true
run_tui=true
run_web=true
web_timeout_seconds="${WOLFBBS_WEB_E2E_TIMEOUT_SECONDS:-900}"
allow_unsupported_node="${WOLFBBS_ALLOW_UNSUPPORTED_NODE:-false}"
node_bin="${WOLFBBS_NODE_BIN:-}"
npm_bin="${WOLFBBS_NPM_BIN:-}"

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
  local bin="$1"
  "$bin" -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo "0"
}

try_use_node_toolchain() {
  local candidate_dir="$1"
  local candidate_node="$candidate_dir/node"
  local candidate_npm="$candidate_dir/npm"
  if [[ -x "$candidate_node" && -x "$candidate_npm" ]]; then
    node_bin="$candidate_node"
    npm_bin="$candidate_npm"
    return 0
  fi
  return 1
}

resolve_node_toolchain() {
  if [[ -n "${node_bin}" && -n "${npm_bin}" ]]; then
    if [[ -x "$node_bin" && -x "$npm_bin" ]]; then
      return 0
    fi
    echo "WOLFBBS_NODE_BIN/WOLFBBS_NPM_BIN set but not executable." >&2
    return 1
  fi
  if [[ -z "${node_bin}" ]]; then
    node_bin="$(command -v node || true)"
  fi
  if [[ -z "${npm_bin}" ]]; then
    npm_bin="$(command -v npm || true)"
  fi
  if [[ -z "${node_bin}" || -z "${npm_bin}" ]]; then
    echo "node and npm are required for web e2e. Install Node.js 22 or 24." >&2
    return 1
  fi
  return 0
}

prefer_compatible_node() {
  local major
  major="$(detect_node_major "$node_bin")"
  if ! [[ "$major" =~ ^[0-9]+$ ]]; then
    major=0
  fi
  if [[ "$major" -lt 25 ]]; then
    return 0
  fi
  if [[ "$allow_unsupported_node" == "true" ]]; then
    echo "Node.js $major detected; proceeding because --allow-unsupported-node is set."
    return 0
  fi

  if [[ "$(uname -s)" == "Darwin" ]]; then
    if has_cmd node24 && has_cmd npm24; then
      node_bin="$(command -v node24)"
      npm_bin="$(command -v npm24)"
      echo "Using node24/npm24 from PATH for Playwright compatibility."
      return 0
    fi
    if has_cmd node22 && has_cmd npm22; then
      node_bin="$(command -v node22)"
      npm_bin="$(command -v npm22)"
      echo "Using node22/npm22 from PATH for Playwright compatibility."
      return 0
    fi
    if has_cmd brew; then
      local brew24=""
      local brew22=""
      brew24="$(brew --prefix node@24 2>/dev/null || true)"
      if [[ -n "$brew24" ]] && try_use_node_toolchain "$brew24/bin"; then
        echo "Using Homebrew node@24 toolchain: $brew24"
        return 0
      fi
      brew22="$(brew --prefix node@22 2>/dev/null || true)"
      if [[ -n "$brew22" ]] && try_use_node_toolchain "$brew22/bin"; then
        echo "Using Homebrew node@22 toolchain: $brew22"
        return 0
      fi
    fi
  fi

  echo "Node.js $major detected." >&2
  echo "Web e2e requires Node 22/24 for stable Playwright runs in this environment." >&2
  echo "Install one and rerun, or pass --allow-unsupported-node to continue at your own risk." >&2
  echo "macOS quick path: brew install node@24 && export PATH=\"\$(brew --prefix node@24)/bin:\$PATH\"" >&2
  return 1
}

run_npm() {
  local npm_dir
  npm_dir="$(dirname "$npm_bin")"
  PATH="$npm_dir:$PATH" "$npm_bin" --prefix e2e/web "$@"
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
Environment overrides:
  WOLFBBS_NODE_BIN / WOLFBBS_NPM_BIN
                 Explicit node/npm binaries for web e2e (useful on macOS with node@24)
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
  if ! resolve_node_toolchain; then
    exit 1
  fi
  if ! prefer_compatible_node; then
    exit 1
  fi
  local_node_major="$(detect_node_major "$node_bin")"
  echo "Web e2e node toolchain: node=${node_bin} npm=${npm_bin} (major=${local_node_major})"
  echo "[3/3] web e2e (Playwright)"
  if ! run_with_timeout "$web_timeout_seconds" run_npm install; then
    echo "web e2e dependency install failed or timed out" >&2
    exit 1
  fi
  if ! run_with_timeout "$web_timeout_seconds" run_npm run install:browsers; then
    echo "web e2e browser install failed or timed out" >&2
    exit 1
  fi
  if ! run_with_timeout "$web_timeout_seconds" run_npm test; then
    echo "web e2e test run failed or timed out" >&2
    exit 1
  fi
fi

echo "PASS end-to-end suite"
