#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

run_go_tests=true
run_tui=true
run_web=true
web_timeout_seconds="${WOLFBBS_WEB_E2E_TIMEOUT_SECONDS:-900}"
allow_unsupported_node="${WOLFBBS_ALLOW_UNSUPPORTED_NODE:-false}"
skip_browser_install="${WOLFBBS_SKIP_BROWSER_INSTALL:-false}"
skip_npm_install="${WOLFBBS_SKIP_NPM_INSTALL:-false}"
web_e2e_mirror_mode="${WOLFBBS_WEB_E2E_MIRROR:-auto}"
web_e2e_mirror_dir="${WOLFBBS_WEB_E2E_MIRROR_DIR:-/tmp/wolfbbs-web-e2e-runner}"
web_e2e_dir="${WOLFBBS_WEB_E2E_DIR:-$ROOT_DIR/e2e/web}"
node_bin="${WOLFBBS_NODE_BIN:-}"
npm_bin="${WOLFBBS_NPM_BIN:-}"
use_existing_web="${WOLFBBS_E2E_USE_EXISTING_WEB:-false}"

has_cmd() {
  command -v "$1" >/dev/null 2>&1
}

run_with_timeout() {
  local seconds="$1"
  shift
  local command_name="$1"
  local is_function=false
  if declare -F "$command_name" >/dev/null 2>&1; then
    is_function=true
  fi
  if [[ "$is_function" == "false" ]]; then
    if has_cmd timeout; then
      timeout "$seconds" "$@"
      return $?
    fi
    if has_cmd gtimeout; then
      gtimeout "$seconds" "$@"
      return $?
    fi
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
  PATH="$npm_dir:$PATH" "$npm_bin" --prefix "$web_e2e_dir" "$@"
}

run_npm_skip_browser_download() {
  PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 run_npm "$@"
}

file_hash() {
  local file="$1"
  if has_cmd shasum; then
    shasum -a 256 "$file" | awk '{print $1}'
    return 0
  fi
  if has_cmd sha256sum; then
    sha256sum "$file" | awk '{print $1}'
    return 0
  fi
  return 1
}

npm_lock_hash_file() {
  echo "$web_e2e_dir/node_modules/.wolfbbs-lock.sha256"
}

npm_install_needed() {
  local lock_file="$web_e2e_dir/package-lock.json"
  local node_modules_dir="$web_e2e_dir/node_modules"
  local playwright_pkg="$node_modules_dir/@playwright/test"
  local stamp_file
  stamp_file="$(npm_lock_hash_file)"

  if [[ ! -d "$node_modules_dir" || ! -d "$playwright_pkg" ]]; then
    return 0
  fi
  if [[ ! -f "$lock_file" ]]; then
    # Without a lockfile, fall back to installing to keep dependencies consistent.
    return 0
  fi
  if [[ ! -f "$stamp_file" ]]; then
    return 0
  fi
  local current_hash
  local recorded_hash
  current_hash="$(file_hash "$lock_file" || true)"
  recorded_hash="$(tr -d ' \n\r\t' <"$stamp_file" 2>/dev/null || true)"
  if [[ -z "$current_hash" || -z "$recorded_hash" || "$current_hash" != "$recorded_hash" ]]; then
    return 0
  fi
  return 1
}

record_npm_lock_hash() {
  local lock_file="$web_e2e_dir/package-lock.json"
  local stamp_file
  stamp_file="$(npm_lock_hash_file)"
  if [[ ! -f "$lock_file" ]]; then
    return 0
  fi
  local hash
  hash="$(file_hash "$lock_file" || true)"
  if [[ -z "$hash" ]]; then
    return 0
  fi
  mkdir -p "$(dirname "$stamp_file")"
  printf '%s\n' "$hash" >"$stamp_file"
}

free_space_mb() {
  local path="$1"
  if [[ "$(uname -s)" == "Darwin" ]]; then
    df -Pm "$path" | awk 'NR==2 {print $4}'
  else
    df -Pm "$path" | awk 'NR==2 {print $4}'
  fi
}

ensure_disk_space_for_browser_install() {
  local min_mb="${WOLFBBS_WEB_E2E_MIN_FREE_MB:-1200}"
  if ! [[ "$min_mb" =~ ^[0-9]+$ ]]; then
    min_mb=1200
  fi
  local free_mb
  free_mb="$(free_space_mb "$ROOT_DIR" 2>/dev/null || echo 0)"
  if ! [[ "$free_mb" =~ ^[0-9]+$ ]]; then
    free_mb=0
  fi
  if (( free_mb < min_mb )); then
    echo "Insufficient free disk for Playwright browser install: ${free_mb}MB available, ${min_mb}MB required." >&2
    echo "Set WOLFBBS_SKIP_BROWSER_INSTALL=true if browsers are already installed, or free disk space and retry." >&2
    return 1
  fi
  return 0
}

prepare_web_e2e_dir() {
  local source_dir="$ROOT_DIR/e2e/web"
  local mirror_mode
  mirror_mode="$(printf '%s' "$web_e2e_mirror_mode" | tr '[:upper:]' '[:lower:]')"
  if [[ "$mirror_mode" == "false" || "$mirror_mode" == "off" || "$mirror_mode" == "no" ]]; then
    web_e2e_dir="$source_dir"
    return 0
  fi
  if [[ "$mirror_mode" == "auto" && "$(uname -s)" != "Darwin" ]]; then
    web_e2e_dir="$source_dir"
    return 0
  fi
  if ! has_cmd rsync; then
    echo "rsync not found; using source web e2e directory: $source_dir"
    web_e2e_dir="$source_dir"
    return 0
  fi
  mkdir -p "$web_e2e_mirror_dir"
  rsync -a --delete \
    --exclude node_modules \
    --exclude playwright-report \
    --exclude test-results \
    "$source_dir/" "$web_e2e_mirror_dir/"
  web_e2e_dir="$web_e2e_mirror_dir"
  echo "Using mirrored web e2e workspace: $web_e2e_dir"
}

load_env_file_if_present() {
  local env_file="$1"
  if [[ ! -f "$env_file" ]]; then
    return 0
  fi
  set -a
  # shellcheck disable=SC1090
  . "$env_file"
  set +a
}

load_e2e_env_defaults() {
  local installer_env_file="${WOLFBBS_INSTALLER_ENV_FILE:-$HOME/.local/share/wolfbbs/.env}"
  if [[ -f "$installer_env_file" ]]; then
    load_env_file_if_present "$installer_env_file"
    echo "Loaded installer env defaults: $installer_env_file"
  fi
  if [[ -f "$ROOT_DIR/.env" ]]; then
    load_env_file_if_present "$ROOT_DIR/.env"
    echo "Loaded repo env defaults: $ROOT_DIR/.env"
  fi
}

playwright_cache_dir() {
  if [[ -n "${PLAYWRIGHT_BROWSERS_PATH:-}" && "${PLAYWRIGHT_BROWSERS_PATH}" != "0" ]]; then
    echo "${PLAYWRIGHT_BROWSERS_PATH}"
    return 0
  fi
  if [[ "$(uname -s)" == "Darwin" ]]; then
    echo "$HOME/Library/Caches/ms-playwright"
    return 0
  fi
  echo "$HOME/.cache/ms-playwright"
}

playwright_chromium_installed() {
  local cache_dir
  cache_dir="$(playwright_cache_dir)"
  if [[ ! -d "$cache_dir" ]]; then
    return 1
  fi
  # macOS full Chromium bundle
  if compgen -G "$cache_dir/chromium-*/chrome-*/Chromium.app/Contents/MacOS/Chromium" >/dev/null; then
    return 0
  fi
  # macOS headless shell
  if compgen -G "$cache_dir/chromium_headless_shell-*/chrome-headless-shell-*/chrome-headless-shell" >/dev/null; then
    return 0
  fi
  # Linux full Chromium
  if compgen -G "$cache_dir/chromium-*/chrome-linux/chrome" >/dev/null; then
    return 0
  fi
  # Linux headless shell
  if compgen -G "$cache_dir/chromium_headless_shell-*/chrome-headless-shell-linux64/chrome-headless-shell" >/dev/null; then
    return 0
  fi
  return 1
}

ensure_base_url_for_mirror() {
  if [[ "${use_existing_web}" != "true" && "${use_existing_web}" != "1" ]]; then
    return 1
  fi
  if [[ -n "${WOLFBBS_E2E_BASE_URL:-}" ]]; then
    return 0
  fi
  local web_port="${WOLFBBS_WEB_PORT:-8080}"
  local candidate="${WOLFBBS_WEB_E2E_BASE_URL_DEFAULT:-http://127.0.0.1:${web_port}}"
  if has_cmd curl && curl -fsS --max-time 3 "${candidate}/healthz" >/dev/null 2>&1; then
    export WOLFBBS_E2E_BASE_URL="$candidate"
    echo "Detected running web service; using WOLFBBS_E2E_BASE_URL=${WOLFBBS_E2E_BASE_URL}"
    return 0
  fi
  return 1
}

seed_web_e2e_credentials() {
  local default_admin_handle="${WOLFBBS_BOOTSTRAP_ADMIN_HANDLE:-sysop}"
  local default_admin_password="${WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD:-wolfbbs-sysop}"
  export WOLFBBS_E2E_ADMIN_HANDLE="${WOLFBBS_E2E_ADMIN_HANDLE:-$default_admin_handle}"
  export WOLFBBS_E2E_ADMIN_PASSWORD="${WOLFBBS_E2E_ADMIN_PASSWORD:-$default_admin_password}"
  export WOLFBBS_E2E_USER_HANDLE="${WOLFBBS_E2E_USER_HANDLE:-$WOLFBBS_E2E_ADMIN_HANDLE}"
  export WOLFBBS_E2E_USER_PASSWORD="${WOLFBBS_E2E_USER_PASSWORD:-$WOLFBBS_E2E_ADMIN_PASSWORD}"
  export WOLFBBS_E2E_IRC_NICK="${WOLFBBS_E2E_IRC_NICK:-$WOLFBBS_E2E_ADMIN_HANDLE}"
  export WOLFBBS_E2E_IRC_PASS="${WOLFBBS_E2E_IRC_PASS:-$WOLFBBS_E2E_ADMIN_PASSWORD}"
  export WOLFBBS_E2E_IRC_PORT="${WOLFBBS_E2E_IRC_PORT:-${WOLFBBS_IRC_PORT:-6667}}"
  echo "Web e2e auth defaults: admin=${WOLFBBS_E2E_ADMIN_HANDLE} user=${WOLFBBS_E2E_USER_HANDLE}"
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
  --skip-browser-install
                 Skip Playwright browser installation step (or set WOLFBBS_SKIP_BROWSER_INSTALL=true)
  --skip-npm-install
                 Skip npm dependency installation step (or set WOLFBBS_SKIP_NPM_INSTALL=true)
Environment overrides:
  WOLFBBS_NODE_BIN / WOLFBBS_NPM_BIN
                 Explicit node/npm binaries for web e2e (useful on macOS with node@24)
  WOLFBBS_WEB_E2E_MIRROR
                 auto|true|false (default: auto; on macOS auto mirrors e2e/web to /tmp)
  WOLFBBS_WEB_E2E_MIRROR_DIR
                 Override mirror path (default: /tmp/wolfbbs-web-e2e-runner)
  WOLFBBS_WEB_E2E_DIR
                 Override web e2e directory/prefix path
  WOLFBBS_E2E_REPO_ROOT
                 Repo root used by Playwright webServer command when tests run from a mirrored path
  WOLFBBS_E2E_USE_EXISTING_WEB
                 Set to true to reuse an already-running web service instead of booting the current tree
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
    --skip-browser-install)
      skip_browser_install=true
      ;;
    --skip-npm-install)
      skip_npm_install=true
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

load_e2e_env_defaults

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
  if ! prepare_web_e2e_dir; then
    echo "Failed to prepare web e2e workspace." >&2
    exit 1
  fi
  if [[ "$web_e2e_dir" != "$ROOT_DIR/e2e/web" ]]; then
    export WOLFBBS_E2E_REPO_ROOT="${WOLFBBS_E2E_REPO_ROOT:-$ROOT_DIR}"
    if [[ -z "${WOLFBBS_E2E_BASE_URL:-}" ]]; then
      ensure_base_url_for_mirror || true
    fi
  fi
  seed_web_e2e_credentials
  local_node_major="$(detect_node_major "$node_bin")"
  echo "Web e2e node toolchain: node=${node_bin} npm=${npm_bin} (major=${local_node_major})"
  echo "Web e2e working directory: ${web_e2e_dir}"
  echo "[3/3] web e2e (Playwright)"
  if [[ "$skip_npm_install" == "true" ]]; then
    echo "Skipping npm dependency install (--skip-npm-install)."
  elif npm_install_needed; then
    echo "Installing web e2e dependencies..."
    if [[ -f "$web_e2e_dir/package-lock.json" ]]; then
      if ! run_with_timeout "$web_timeout_seconds" run_npm_skip_browser_download ci; then
        echo "web e2e dependency install (npm ci) failed or timed out" >&2
        exit 1
      fi
    else
      if ! run_with_timeout "$web_timeout_seconds" run_npm_skip_browser_download install; then
        echo "web e2e dependency install (npm install) failed or timed out" >&2
        exit 1
      fi
    fi
    record_npm_lock_hash
  else
    echo "Web e2e dependencies already up to date; skipping npm install."
  fi
  if [[ "$skip_browser_install" == "true" ]]; then
    if playwright_chromium_installed; then
      echo "Skipping Playwright browser install (--skip-browser-install)."
    else
      echo "--skip-browser-install was set, but no Playwright Chromium browser is installed." >&2
      echo "Run without --skip-browser-install once, or pre-install browsers with: npm --prefix \"$web_e2e_dir\" run install:browsers" >&2
      exit 1
    fi
  elif playwright_chromium_installed; then
    echo "Playwright Chromium cache already present; skipping browser install."
  else
    if ! ensure_disk_space_for_browser_install; then
      exit 1
    fi
    if ! run_with_timeout "$web_timeout_seconds" run_npm run install:browsers; then
      echo "web e2e browser install failed or timed out" >&2
      exit 1
    fi
  fi
  if ! run_with_timeout "$web_timeout_seconds" run_npm test; then
    echo "web e2e test run failed or timed out" >&2
    exit 1
  fi
fi

echo "PASS end-to-end suite"
