#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FROM_REF="${WOLFBBS_STABLE_FROM_REF:-v2.0.3}"
TO_REF="${WOLFBBS_STABLE_TO_REF:-HEAD}"
KEEP_ARTIFACTS=false
TMP_ROOT=""

usage() {
  cat <<'USAGE'
WolfBBS stable release matrix

Usage:
  scripts/stable-release-matrix.sh [options]

Options:
  --from-ref <git-ref>   baseline release to install first (default: v2.0.3)
  --to-ref <git-ref>     candidate ref to upgrade to (default: HEAD)
  --tmp-root <dir>       working directory for temp clone/install artifacts
  --keep                 keep temp artifacts after success
  -h, --help             show this help
USAGE
}

log() {
  printf '[stable-matrix] %s\n' "$*"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing required command: $1" >&2
    exit 1
  }
}

ensure_docker_access() {
  if docker info >/dev/null 2>&1; then
    return
  fi
  local colima_sock="${HOME}/.colima/default/docker.sock"
  if [[ -z "${DOCKER_HOST:-}" && -S "$colima_sock" ]]; then
    export DOCKER_HOST="unix://${colima_sock}"
  fi
  docker info >/dev/null 2>&1 || {
    echo "Docker daemon is not reachable." >&2
    exit 1
  }
}

allocate_ports() {
  python3 - <<'PY'
import socket
ports = []
for _ in range(5):
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    ports.append(str(s.getsockname()[1]))
    s.close()
print(" ".join(ports))
PY
}

run_step() {
  log "$*"
  "$@"
}

wait_for_http() {
  local url="$1"
  local timeout="${2:-120}"
  local elapsed=0
  while (( elapsed < timeout )); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  return 1
}

wait_for_port() {
  local port="$1"
  local timeout="${2:-120}"
  local elapsed=0
  while (( elapsed < timeout )); do
    if nc -z 127.0.0.1 "$port" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  return 1
}

assert_health() {
  local prefix="$1"
  local ssh_port="$2"
  local web_port="$3"
  local irc_port="$4"
  run_step bash install.sh --yes --prefix "$prefix" --status >/dev/null
  run_step bash install.sh --yes --prefix "$prefix" --doctor >/tmp/wolfbbs-stable-doctor.out
  if grep -q "FAIL doctor:" /tmp/wolfbbs-stable-doctor.out; then
    cat /tmp/wolfbbs-stable-doctor.out >&2
    echo "Doctor reported failures." >&2
    exit 1
  fi
  wait_for_port "$ssh_port" 120 || {
    echo "SSH port $ssh_port did not become reachable." >&2
    exit 1
  }
  wait_for_port "$irc_port" 120 || {
    echo "IRC port $irc_port did not become reachable." >&2
    exit 1
  }
  wait_for_http "http://127.0.0.1:${web_port}/healthz" 120 || {
    echo "Web healthz did not become reachable on port $web_port." >&2
    exit 1
  }
  wait_for_http "http://127.0.0.1:${web_port}/readyz" 120 || {
    echo "Web readyz did not become reachable on port $web_port." >&2
    exit 1
  }
  run_step bash install.sh --yes --prefix "$prefix" --debug-bundle >/tmp/wolfbbs-stable-debug.out
  grep -q "Debug bundle written:" /tmp/wolfbbs-stable-debug.out || {
    cat /tmp/wolfbbs-stable-debug.out >&2
    echo "Debug bundle was not written." >&2
    exit 1
  }
}

cleanup() {
  local prefix="$1"
  if [[ -n "$prefix" && -d "$prefix" ]]; then
    (cd "$ROOT_DIR" && bash install.sh --yes --prefix "$prefix" --clean-uninstall >/dev/null 2>&1 || true)
  fi
  if [[ "$KEEP_ARTIFACTS" != "true" && -n "$TMP_ROOT" && -d "$TMP_ROOT" ]]; then
    rm -rf "$TMP_ROOT"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --from-ref)
      FROM_REF="$2"
      shift 2
      ;;
    --to-ref)
      TO_REF="$2"
      shift 2
      ;;
    --tmp-root)
      TMP_ROOT="$2"
      shift 2
      ;;
    --keep)
      KEEP_ARTIFACTS=true
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
done

require_cmd git
require_cmd docker
require_cmd curl
require_cmd nc
ensure_docker_access

if [[ -z "$TMP_ROOT" ]]; then
  TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/wolfbbs-stable-release.XXXXXX")"
else
  mkdir -p "$TMP_ROOT"
fi

CHECKOUT_DIR="$TMP_ROOT/repo"
PREFIX_DIR="$TMP_ROOT/prefix"
LOG_FILE="$TMP_ROOT/stable-release-matrix.log"
PORTS="$(allocate_ports)"
read -r SSH_PORT WEB_PORT IRC_PORT IRC_TLS_PORT MAILIN_PORT <<<"$PORTS"

trap 'cleanup "$PREFIX_DIR"' EXIT

{
  log "tmp root: $TMP_ROOT"
  log "checkout dir: $CHECKOUT_DIR"
  log "prefix dir: $PREFIX_DIR"
  log "ports: ssh=$SSH_PORT web=$WEB_PORT irc=$IRC_PORT irc_tls=$IRC_TLS_PORT mailin=$MAILIN_PORT"

  run_step git clone "$ROOT_DIR" "$CHECKOUT_DIR"
  cd "$CHECKOUT_DIR"

  run_step git checkout "$FROM_REF"
  log "fresh install from $FROM_REF"
  run_step bash install.sh --yes --prefix "$PREFIX_DIR" \
    --ssh-port "$SSH_PORT" \
    --web-port "$WEB_PORT" \
    --irc-port "$IRC_PORT" \
    --irc-tls-port "$IRC_TLS_PORT" \
    --mailin-port "$MAILIN_PORT"
  assert_health "$PREFIX_DIR" "$SSH_PORT" "$WEB_PORT" "$IRC_PORT"

  log "upgrade from $FROM_REF to $TO_REF"
  run_step git checkout "$TO_REF"
  run_step bash install.sh --yes --prefix "$PREFIX_DIR" --upgrade
  assert_health "$PREFIX_DIR" "$SSH_PORT" "$WEB_PORT" "$IRC_PORT"

  log "rapid-upgrade candidate rebuild"
  run_step bash install.sh --yes --prefix "$PREFIX_DIR" --rapid-upgrade
  assert_health "$PREFIX_DIR" "$SSH_PORT" "$WEB_PORT" "$IRC_PORT"

  log "repair flow"
  run_step bash install.sh --yes --prefix "$PREFIX_DIR" --repair
  assert_health "$PREFIX_DIR" "$SSH_PORT" "$WEB_PORT" "$IRC_PORT"

  log "rollback to $FROM_REF"
  run_step git checkout "$FROM_REF"
  run_step bash install.sh --yes --prefix "$PREFIX_DIR" --rapid-upgrade
  assert_health "$PREFIX_DIR" "$SSH_PORT" "$WEB_PORT" "$IRC_PORT"

  log "restore candidate after rollback"
  run_step git checkout "$TO_REF"
  run_step bash install.sh --yes --prefix "$PREFIX_DIR" --rapid-upgrade
  assert_health "$PREFIX_DIR" "$SSH_PORT" "$WEB_PORT" "$IRC_PORT"

  log "clean uninstall"
  run_step bash install.sh --yes --prefix "$PREFIX_DIR" --clean-uninstall
  if [[ -d "$PREFIX_DIR" ]]; then
    echo "Prefix directory still exists after clean uninstall: $PREFIX_DIR" >&2
    exit 1
  fi

  log "reinstall candidate from clean prefix"
  run_step bash install.sh --yes --prefix "$PREFIX_DIR" \
    --ssh-port "$SSH_PORT" \
    --web-port "$WEB_PORT" \
    --irc-port "$IRC_PORT" \
    --irc-tls-port "$IRC_TLS_PORT" \
    --mailin-port "$MAILIN_PORT"
  assert_health "$PREFIX_DIR" "$SSH_PORT" "$WEB_PORT" "$IRC_PORT"

  log "PASS stable release matrix"
} 2>&1 | tee "$LOG_FILE"

log "evidence log: $LOG_FILE"
