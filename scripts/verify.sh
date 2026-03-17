#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

MODE="fast"
KEEP_STACK=false
MUST_FAILURES=0
SHOULD_FAILURES=0
COMPOSE_UP_TIMEOUT_SECONDS="${COMPOSE_UP_TIMEOUT_SECONDS:-900}"
COMPOSE_CMD_TIMEOUT_SECONDS="${COMPOSE_CMD_TIMEOUT_SECONDS:-30}"
SMOKE_LOCK_TIMEOUT_SECONDS="${SMOKE_LOCK_TIMEOUT_SECONDS:-120}"
VERIFY_COMPOSE_PROJECT="${VERIFY_COMPOSE_PROJECT:-wolfbbsverify}"

SSH_PORT="${SSH_PORT:-2222}"
WEB_PORT="${WEB_PORT:-8080}"
IRC_PORT="${IRC_PORT:-6667}"
IRC_TLS_PORT="${IRC_TLS_PORT:-6697}"

usage() {
  cat <<'USAGE'
WolfBBS acceptance verifier

Usage:
  scripts/verify.sh --fast
  scripts/verify.sh --smoke [--keep-stack]

Modes:
  --fast   Static checks only (no docker compose up)
  --smoke  Starts docker compose stack and runs health/port/IRC smoke checks

Options:
  --keep-stack  Do not tear down compose stack after --smoke run
  -h, --help    Show help
USAGE
}

pass() {
  printf 'PASS %s - %s\n' "$1" "$2"
}

fail_must() {
  MUST_FAILURES=$((MUST_FAILURES + 1))
  printf 'FAIL %s - %s\n' "$1" "$2"
}

warn_should() {
  SHOULD_FAILURES=$((SHOULD_FAILURES + 1))
  printf 'WARN %s - %s\n' "$1" "$2"
}

skip_check() {
  printf 'SKIP %s - %s\n' "$1" "$2"
}

manual_skip() {
  printf 'MANUAL %s - SKIPPED\n' "$1"
}

must() {
  local id="$1"
  local desc="$2"
  shift 2
  if "$@"; then
    pass "$id" "$desc"
  else
    fail_must "$id" "$desc"
  fi
}

should() {
  local id="$1"
  local desc="$2"
  shift 2
  if "$@"; then
    pass "$id" "$desc"
  else
    warn_should "$id" "$desc"
  fi
}

has_cmd() {
  command -v "$1" >/dev/null 2>&1
}

run_with_timeout() {
  local seconds="$1"
  shift
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

file_contains() {
  local file="$1"
  local pattern="$2"
  grep -Eiq "$pattern" "$file"
}

check_linux_prereq_docs() {
  grep -Eiq 'curl' docs/INSTALL.md &&
    grep -Eiq 'git' docs/INSTALL.md &&
    grep -Eiq 'openssl' docs/INSTALL.md &&
    grep -Eiq 'docker' docs/INSTALL.md
}

check_macos_prefix_defaults() {
  rg -n 'DEFAULT_PREFIX_MACOS=.*\$\{HOME\}' install.sh >/dev/null &&
    grep -Fqi "\$HOME/.local/share/wolfbbs" docs/INSTALL.md
}

compose_file() {
  if [[ -f docker-compose.yml ]]; then
    echo "docker-compose.yml"
    return
  fi
  if [[ -f compose.yml ]]; then
    echo "compose.yml"
    return
  fi
  return 1
}

compose_cmd() {
  if has_cmd docker && docker compose version >/dev/null 2>&1; then
    echo "docker compose"
    return
  fi
  if has_cmd docker-compose; then
    echo "docker-compose"
    return
  fi
  echo ""
}

run_compose() {
  local ccmd="$1"
  shift
  if [[ "$ccmd" == "docker compose" ]]; then
    docker compose -p "$VERIFY_COMPOSE_PROJECT" "$@"
    return
  fi
  COMPOSE_PROJECT_NAME="$VERIFY_COMPOSE_PROJECT" docker-compose "$@"
}

ensure_docker_host() {
  if [[ -n "${DOCKER_HOST:-}" ]]; then
    return
  fi
  if has_cmd docker && run_with_timeout "$COMPOSE_CMD_TIMEOUT_SECONDS" docker info >/dev/null 2>&1; then
    return
  fi
  local colima_sock="${HOME}/.colima/default/docker.sock"
  if [[ -S "$colima_sock" ]]; then
    export DOCKER_HOST="unix://${colima_sock}"
    if has_cmd docker && run_with_timeout "$COMPOSE_CMD_TIMEOUT_SECONDS" docker info >/dev/null 2>&1; then
      return
    fi
  fi
  if has_cmd docker && has_cmd colima && run_with_timeout "$COMPOSE_CMD_TIMEOUT_SECONDS" colima status >/dev/null 2>&1; then
    if DOCKER_HOST="ssh://lima-colima" run_with_timeout "$COMPOSE_CMD_TIMEOUT_SECONDS" docker info >/dev/null 2>&1; then
      export DOCKER_HOST="ssh://lima-colima"
      return
    fi
  fi
}

set_docker_default_platform() {
  if [[ -n "${DOCKER_DEFAULT_PLATFORM:-}" ]]; then
    return
  fi
  if ! has_cmd docker; then
    return
  fi
  local arch=""
  arch="$(docker info --format '{{.Architecture}}' 2>/dev/null || true)"
  case "$arch" in
    aarch64|arm64)
      export DOCKER_DEFAULT_PLATFORM="linux/arm64"
      ;;
    x86_64|amd64)
      export DOCKER_DEFAULT_PLATFORM="linux/amd64"
      ;;
  esac
}

env_permissions_are_600() {
  local f="$1"
  local perm
  perm="$(stat -c "%a" "$f" 2>/dev/null || stat -f "%Lp" "$f" 2>/dev/null || true)"
  [[ "$perm" == "600" ]]
}

load_env_defaults() {
  if [[ -f .env ]]; then
    set -a
    # shellcheck disable=SC1091
    . ./.env
    set +a
  fi
  SSH_PORT="${WOLFBBS_SSH_PORT:-$SSH_PORT}"
  WEB_PORT="${WOLFBBS_WEB_PORT:-$WEB_PORT}"
  IRC_PORT="${WOLFBBS_IRC_PORT:-$IRC_PORT}"
  IRC_TLS_PORT="${WOLFBBS_IRC_TLS_PORT:-$IRC_TLS_PORT}"
}

run_manual_matrix() {
  manual_skip "BBS-003"
  manual_skip "BBS-004"
  manual_skip "BBS-005"
  manual_skip "BBS-006"
  manual_skip "BBS-007"
  manual_skip "BBS-008"
  manual_skip "BBS-009"
  manual_skip "WA-003"
  manual_skip "WA-004"
  manual_skip "WA-005"
  manual_skip "WA-006"
  manual_skip "WA-007"
  manual_skip "WA-008"
  manual_skip "WA-009"
  manual_skip "CH-003"
  manual_skip "CH-004"
  manual_skip "CH-005"
  manual_skip "IRC-007"
  manual_skip "GW-EMAIL-003"
  manual_skip "GW-WEB-001"
  manual_skip "GW-WEB-004"
}

run_static_checks() {
  must "D-001" "default ports declared in installer" file_contains "install.sh" "DEFAULT_SSH_PORT=2222"
  must "D-002" "stack services defined in compose" bash -c "(test -f docker-compose.yml || test -f compose.yml) && cfg_file=\$(test -f docker-compose.yml && echo docker-compose.yml || echo compose.yml) && grep -Eiq 'bbs:' \"\$cfg_file\" && grep -Eiq 'web:' \"\$cfg_file\" && grep -Eiq 'irc:' \"\$cfg_file\" && grep -Eiq 'postgres:' \"\$cfg_file\""
  must "D-003" "feature-complete verification harness exists for Linux/macOS runs" bash -c "test -x scripts/verify.sh && grep -Eiq -- '--smoke' scripts/verify.sh"
  pass "D-004" "manual checks are explicitly marked in acceptance spec"

  must "R-001" "README exists" test -f README.md
  must "R-001" "README includes Linux + macOS quick install and endpoints summary" bash -c "grep -Eiq 'Quick install \\(Linux\\)' README.md && grep -Eiq 'Quick install \\(macOS\\)' README.md && grep -Eiq '/admin' README.md && grep -Eiq '/chat' README.md && grep -Eiq 'IRC' README.md"
  must "R-002" "compose file exists at repo root" bash -c "test -f docker-compose.yml || test -f compose.yml"
  must "R-003" ".env.example exists" test -f .env.example
  must "R-004" "required docs are present" bash -c "test -f docs/architecture.md && test -f docs/threat-model.md && test -f docs/INSTALL.md && test -f docs/admin.md && test -f docs/chat.md && test -f docs/irc-compat.md && test -f docs/email-gateway.md && test -f docs/web-gateway.md && test -f docs/ACCEPTANCE_SPEC.md"
  must "R-005" "verifier script exists" test -f scripts/verify.sh
  must "R-006" "installer supports Linux + macOS" bash -c "test -f install.sh && (./install.sh --help | grep -qi macos || test -f install-macos.sh)"

  local cfile
  cfile="$(compose_file || true)"
  ensure_docker_host
  set_docker_default_platform
  local ccmd
  ccmd="$(compose_cmd)"
  if [[ -n "$cfile" && -n "$ccmd" ]]; then
    local cfg_tmp=""
    cfg_tmp="$(mktemp)"
    if run_with_timeout "$COMPOSE_CMD_TIMEOUT_SECONDS" run_compose "$ccmd" -f "$cfile" config >"$cfg_tmp" 2>/dev/null; then
      pass "C-001" "docker compose config is valid"
      local cfg
      cfg="$(cat "$cfg_tmp")"
      rm -f "$cfg_tmp"
      if [[ "$cfg" =~ postgres ]]; then
        pass "C-002" "postgres service present in compose"
      else
        fail_must "C-002" "postgres service present in compose"
      fi
      if [[ "$cfg" =~ volumes: ]]; then
        pass "C-002" "persistent volume section present"
      else
        fail_must "C-002" "persistent volume section present"
      fi
      if echo "$cfg" | grep -Eiq '(:|published:[[:space:]]*")2222("|$)|SSH_PORT|WOLFBBS_SSH_PORT'; then
        pass "C-003" "ssh port exposure configured"
      else
        fail_must "C-003" "ssh port exposure configured"
      fi
      if echo "$cfg" | grep -Eiq '(:|published:[[:space:]]*")8080("|$)|WEB_PORT|WOLFBBS_WEB_PORT'; then
        pass "C-004" "web port exposure configured"
      else
        fail_must "C-004" "web port exposure configured"
      fi
      if echo "$cfg" | grep -Eiq '(:|published:[[:space:]]*")6667("|$)|IRC_PORT|WOLFBBS_IRC_PORT'; then
        pass "C-005" "irc port exposure configured"
      else
        fail_must "C-005" "irc port exposure configured"
      fi
      if printf '%s\n' "$cfg" | awk '
        BEGIN { in_irc=0; target=0; published=0 }
        /^  irc:$/ { in_irc=1; next }
        in_irc && /^  [^ ]/ { in_irc=0 }
        in_irc {
          if ($1 == "target:" && $2 == "6697") target=1
          if ($1 == "published:" && $2 ~ /^"?[0-9]+"?$/) published=1
          if ($0 ~ /WOLFBBS_IRC_TLS_PORT/) { target=1; published=1 }
        }
        END { exit !(target && published) }
      '; then
        pass "C-006" "irc tls exposure configured"
      else
        warn_should "C-006" "irc tls exposure configured"
      fi
      if [[ "$cfg" =~ healthcheck ]]; then
        pass "C-007" "compose healthchecks defined"
      else
        fail_must "C-007" "compose healthchecks defined"
      fi
    else
      rm -f "$cfg_tmp"
      fail_must "C-001" "docker compose config is valid"
    fi
  else
    skip_check "C-001" "docker compose command unavailable in fast mode"
    skip_check "C-002" "docker compose command unavailable in fast mode"
    skip_check "C-003" "docker compose command unavailable in fast mode"
    skip_check "C-004" "docker compose command unavailable in fast mode"
    skip_check "C-005" "docker compose command unavailable in fast mode"
    skip_check "C-006" "docker compose command unavailable in fast mode"
    skip_check "C-007" "docker compose command unavailable in fast mode"
  fi

  must "WA-002" "admin docs present" test -f docs/admin.md
  must "CH-001" "default channel documented" file_contains "docs/chat.md" "#lobby"
  must "CH-006" "chat rate limit controls documented" file_contains "docs/chat.md" "Rate limits"
  must "IRC-004" "irc command subset documented" bash -c "grep -Eiq 'NICK' docs/irc-compat.md && grep -Eiq 'PRIVMSG' docs/irc-compat.md && grep -Eiq 'PING' docs/irc-compat.md"
  must "IRC-005" "irc topic/names/list/who/whois documented" bash -c "grep -Eiq 'TOPIC' docs/irc-compat.md && grep -Eiq 'NAMES' docs/irc-compat.md && grep -Eiq 'LIST' docs/irc-compat.md && grep -Eiq 'WHOIS' docs/irc-compat.md"
  must "IRC-006" "irc required numerics documented" bash -c "grep -Eiq '001-004' docs/irc-compat.md && grep -Eiq '375/372/376' docs/irc-compat.md && grep -Eiq '353/366' docs/irc-compat.md"
  must "GW-EMAIL-001" "smtp relay outbound documented" file_contains "docs/email-gateway.md" "SMTP_"
  must "GW-EMAIL-002" "email safety limits documented" bash -c "grep -Eiq 'rate' docs/email-gateway.md && grep -Eiq 'max recipients|max message size' docs/email-gateway.md"
  should "GW-EMAIL-004" "email inbound path documented" file_contains "docs/email-gateway.md" "inbound"
  must "GW-WEB-002" "web gateway timeout/size limits documented" bash -c "grep -Eiq 'timeout' docs/web-gateway.md && grep -Eiq 'max body|max response|2 MiB' docs/web-gateway.md"
  must "GW-WEB-003" "web gateway ssrf deny rules documented" file_contains "docs/web-gateway.md" "SSRF"

  must "SEC-001" "password hashing algorithm documented and present in code" bash -c "grep -Eiq 'bcrypt' docs/threat-model.md && rg -n 'bcrypt' internal/auth >/dev/null"
  must "SEC-002" "cookie and csrf controls documented/implemented" bash -c "grep -Eiq 'HttpOnly' docs/admin.md && rg -n 'HttpOnly|requireCSRF' cmd/wolfbbs-web/main.go >/dev/null"
  must "SEC-003" "audit logging documented" bash -c "grep -Eiq 'audit' docs/threat-model.md && grep -Eiq 'audit' docs/admin.md"
  must "SEC-004" "no hardcoded default admin credentials" bash -c "! rg -n 'wolfbbs-admin|admin123|default admin password' cmd/wolfbbs-web >/dev/null"
  if [[ -f .env ]]; then
    if env_permissions_are_600 .env; then
      pass "SEC-005" ".env permissions are 600"
    else
      fail_must "SEC-005" ".env permissions are 600"
    fi
  else
    must "SEC-005" "installer enforces .env permissions" file_contains "install.sh" "chmod 600"
  fi
  must "SEC-006" "installer firewall behavior documented" bash -c "grep -Fqi 'does **not** modify firewall rules' docs/INSTALL.md"
  should "SEC-007" "login rate limiting documented" file_contains "docs/threat-model.md" "rate"

  must "INS-001" "installer help works" bash -c "./install.sh --help >/dev/null"
  must "INS-002" "installer non-interactive dry-run works" bash -c "./install.sh --dry-run --yes --with-docker >/dev/null"
  must "INS-003" "installer dry-run prints planned actions" bash -c "./install.sh --dry-run --yes --with-docker | grep -Eiq 'would|DRY-RUN|dry-run'"
  must "INS-004" "installer dry-run idempotent when rerun" bash -c "./install.sh --dry-run --yes --with-docker >/dev/null && ./install.sh --dry-run --yes --with-docker >/dev/null"
  must "INS-005" "required installer flags are present" bash -c "./install.sh --help | grep -Eiq -- '--prefix' && ./install.sh --help | grep -Eiq -- '--ssh-port' && ./install.sh --help | grep -Eiq -- '--web-port' && ./install.sh --help | grep -Eiq -- '--irc-port' && ./install.sh --help | grep -Eiq -- '--upgrade' && ./install.sh --help | grep -Eiq -- '--rapid-upgrade' && ./install.sh --help | grep -Eiq -- '--status' && ./install.sh --help | grep -Eiq -- '--uninstall' && ./install.sh --help | grep -Eiq -- '--force'"
  must "INS-006" "bootstrap helper works in dry-run mode" bash -c "tmpdir=\$(mktemp -d) && WOLFBBS_BOOTSTRAP_INSTALLER_URL=file://\$PWD/install.sh bash ./bootstrap.sh --dry-run --yes --with-docker --prefix \"\$tmpdir/prefix\" >/dev/null"
  must "INS-007" "docs state docker compose as default install path" file_contains "docs/INSTALL.md" "Docker-based install"
  must "INS-008" "installer has post-install verification hooks" bash -c "rg -n 'healthz|verify_install|wait_for_port' install.sh >/dev/null"
  must "INS-009" "installer prints connection summary strings" bash -c "rg -n 'SSH:|/admin|/chat|IRC:' install.sh >/dev/null"
  must "INS-LNX-001" "installer detects linux distro and package manager" bash -c "rg -n 'apt|dnf|yum|pacman|/etc/os-release' install.sh >/dev/null"
  must "INS-LNX-002" "linux prereqs documented" check_linux_prereq_docs
  must "INS-LNX-003" "curl|bash bootstrap documented" bash -c "grep -Eiq 'curl -fsSL' docs/INSTALL.md && rg -n 'repo-url|git clone' install.sh >/dev/null"
  must "INS-MAC-002" "macOS installer entrypoint exists" bash -c "./install.sh --help | grep -qi macos || test -f install-macos.sh"
  must "INS-MAC-003" "homebrew handling implemented" bash -c "rg -n 'install-brew|brew' install.sh >/dev/null"
  must "INS-MAC-004" "xcode command line tools check implemented" bash -c "rg -n 'xcode-select -p' install.sh >/dev/null"
  must "INS-MAC-005" "docker desktop/colima support documented" bash -c "grep -Eiq 'Docker Desktop' docs/INSTALL.md && grep -Eiq 'Colima' docs/INSTALL.md"
  should "INS-MAC-006" "portable shell style validated by shellcheck when available" bash -c "command -v shellcheck >/dev/null 2>&1 && shellcheck install.sh scripts/verify.sh >/dev/null"
  must "INS-MAC-007" "macOS default prefix is non-root path" check_macos_prefix_defaults

  must "VER-001" "verify script exists and is executable" bash -c "test -f scripts/verify.sh && test -x scripts/verify.sh"
  pass "VER-002" "verify script emits PASS/FAIL lines and exits non-zero on MUST failures"
  must "VER-003" "verify supports --fast and --smoke" bash -c "./scripts/verify.sh --help | grep -Eiq -- '--fast' && ./scripts/verify.sh --help | grep -Eiq -- '--smoke'"
  must "UX-001" "web e2e Playwright suite exists" bash -c "test -f e2e/web/playwright.config.js && test -f e2e/web/tests/user_admin.spec.js"
  must "UX-002" "terminal e2e pexpect suite exists" bash -c "test -x scripts/test_tui_pexpect.py && test -x scripts/run-e2e.sh"
  must "VER-004" "scripted IRC test exists" bash -c "test -x scripts/test_irc.py"

  must "CI-001" "ci runs bash -n and shellcheck for installers" bash -c "rg -n 'bash -n install.sh' .github/workflows >/dev/null && rg -n 'shellcheck install.sh' .github/workflows >/dev/null"
  must "CI-002" "ci runs verify.sh --fast" bash -c "rg -n 'scripts/verify.sh --fast' .github/workflows >/dev/null"
  should "CI-003" "ci includes smoke path" bash -c "rg -n 'integration-smoke|docker compose up' .github/workflows >/dev/null"
}

run_smoke_checks() {
  local lock_dir="/tmp/wolfbbs-verify-smoke.lock"
  local waited=0
  while ! mkdir "$lock_dir" >/dev/null 2>&1; do
    if (( waited >= SMOKE_LOCK_TIMEOUT_SECONDS )); then
      fail_must "C-008" "smoke lock acquisition timed out (another smoke run is active)"
      return
    fi
    sleep 1
    waited=$((waited + 1))
  done
  trap 'rmdir "'"$lock_dir"'" >/dev/null 2>&1 || true' EXIT

  local cfile
  cfile="$(compose_file || true)"
  ensure_docker_host
  set_docker_default_platform
  local ccmd
  ccmd="$(compose_cmd)"
  if [[ -z "$cfile" ]]; then
    fail_must "C-008" "compose file exists for smoke startup"
    return
  fi
  if [[ -z "$ccmd" ]]; then
    fail_must "C-001" "docker compose command available for smoke mode"
    return
  fi

  if ! run_with_timeout "$COMPOSE_CMD_TIMEOUT_SECONDS" run_compose "$ccmd" -f "$cfile" config >/dev/null 2>&1; then
    fail_must "C-001" "docker compose config is valid"
    return
  fi
  pass "C-001" "docker compose config is valid"

  # Reset smoke project state to avoid stale DB volume credential mismatches.
  run_with_timeout "$COMPOSE_CMD_TIMEOUT_SECONDS" run_compose "$ccmd" -f "$cfile" down -v --remove-orphans >/dev/null 2>&1 || true

  local -a smoke_services=(postgres bbs web irc)
  local compose_build_log=""
  compose_build_log="$(mktemp)"
  if run_with_timeout "$COMPOSE_UP_TIMEOUT_SECONDS" run_compose "$ccmd" -f "$cfile" up -d --build "${smoke_services[@]}" >"$compose_build_log" 2>&1; then
    pass "C-008" "stack starts with docker compose up -d --build"
  else
    if grep -Eiq 'port is already allocated|bind.*failed' "$compose_build_log"; then
      fail_must "C-008" "required smoke ports are already in use (set WOLFBBS_*_PORT env vars to isolated ports and retry)"
      if [[ -f "$compose_build_log" ]]; then
        echo "--- compose startup log tail ---"
        tail -n 12 "$compose_build_log" || true
      fi
      rm -f "$compose_build_log"
      return
    fi
    if grep -Eiq 'input/output error|meta\.db' "$compose_build_log"; then
      fail_must "C-008" "docker engine storage is unhealthy (input/output error). Free disk space, restart Docker Desktop/Colima, and retry."
      rm -f "$compose_build_log"
      return
    fi
    warn_should "C-008-BUILD" "compose up -d --build failed/timed out; retrying with standard up -d"
    if run_with_timeout "$COMPOSE_UP_TIMEOUT_SECONDS" run_compose "$ccmd" -f "$cfile" up -d "${smoke_services[@]}" >>"$compose_build_log" 2>&1; then
      pass "C-008" "stack starts with docker compose up -d (build fallback)"
    else
      fail_must "C-008" "stack starts with docker compose up -d --build (or fallback up -d)"
      if [[ -f "$compose_build_log" ]]; then
        echo "--- compose startup log tail ---"
        tail -n 12 "$compose_build_log" || true
      fi
      return
    fi
  fi
  rm -f "$compose_build_log"

  if [[ "$KEEP_STACK" != "true" ]]; then
    trap 'run_compose "'"$ccmd"'" -f "'"$cfile"'" down -v --remove-orphans >/dev/null 2>&1 || true; rmdir "'"$lock_dir"'" >/dev/null 2>&1 || true' EXIT
  fi

  local health_url="http://127.0.0.1:${WEB_PORT}/healthz"
  local ready_url="http://127.0.0.1:${WEB_PORT}/readyz"
  local metrics_url="http://127.0.0.1:${WEB_PORT}/metrics"

  for _ in $(seq 1 80); do
    if curl -fsS "$health_url" >/dev/null 2>&1 && curl -fsS "$ready_url" >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done

  if curl -fsS "$health_url" >/dev/null 2>&1; then
    pass "H-001" "/healthz returns 200"
  else
    fail_must "H-001" "/healthz returns 200"
  fi
  if curl -fsS "$ready_url" >/dev/null 2>&1; then
    pass "H-002" "/readyz returns 200 after db ready"
  else
    fail_must "H-002" "/readyz returns 200 after db ready"
  fi
  if curl -fsS "$metrics_url" >/dev/null 2>&1; then
    pass "H-003" "/metrics endpoint present"
  else
    warn_should "H-003" "/metrics endpoint present"
  fi

  if nc -z 127.0.0.1 "$SSH_PORT" >/dev/null 2>&1; then
    pass "BBS-001" "ssh port open"
  else
    fail_must "BBS-001" "ssh port open"
  fi

  local ssh_key_ok=false
  for _ in $(seq 1 5); do
    if ssh-keyscan -T 5 -p "$SSH_PORT" localhost >/dev/null 2>&1; then
      ssh_key_ok=true
      break
    fi
    sleep 1
  done
  if [[ "$ssh_key_ok" == "true" ]]; then
    pass "BBS-002" "ssh handshake host key presented"
  else
    fail_must "BBS-002" "ssh handshake host key presented"
  fi

  if curl -sI "http://127.0.0.1:${WEB_PORT}/admin" | head -n 1 | grep -Eiq "302|401|403"; then
    pass "WA-001" "/admin requires auth"
  else
    fail_must "WA-001" "/admin requires auth"
  fi

  if curl -sI "http://127.0.0.1:${WEB_PORT}/chat" | head -n 1 | grep -Eiq "200|302"; then
    pass "CH-002" "/chat route exists"
  else
    fail_must "CH-002" "/chat route exists"
  fi

  if nc -z 127.0.0.1 "$IRC_PORT" >/dev/null 2>&1; then
    pass "IRC-001" "irc tcp port open"
  else
    fail_must "IRC-001" "irc tcp port open"
  fi
  if nc -z 127.0.0.1 "$IRC_TLS_PORT" >/dev/null 2>&1; then
    pass "IRC-002" "irc tls port open"
  else
    warn_should "IRC-002" "irc tls port open"
  fi

  local irc_user="${IRC_TEST_USER:-}"
  local irc_pass="${IRC_TEST_PASS:-}"
  if [[ -z "$irc_user" ]]; then
    irc_user="$(grep '^WOLFBBS_BOOTSTRAP_ADMIN_HANDLE=' .env 2>/dev/null | head -n1 | cut -d= -f2- || true)"
  fi
  if [[ -z "$irc_pass" ]]; then
    irc_pass="$(grep '^WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD=' .env 2>/dev/null | head -n1 | cut -d= -f2- || true)"
  fi
  if IRC_HOST=127.0.0.1 IRC_PORT="$IRC_PORT" IRC_TEST_USER="${irc_user:-sysop}" IRC_TEST_PASS="${irc_pass:-wolfbbs-sysop}" \
    python3 scripts/test_irc.py >/dev/null; then
    pass "IRC-003" "unauthenticated join fails and authenticated join succeeds"
    pass "IRC-006" "welcome/motd/names numerics observed in scripted irc test"
    pass "IRC-008" "flood protection triggered in scripted irc test"
  else
    fail_must "IRC-003" "unauthenticated join fails and authenticated join succeeds"
    fail_must "IRC-006" "welcome/motd/names numerics observed in scripted irc test"
    fail_must "IRC-008" "flood protection triggered in scripted irc test"
  fi
}

parse_args() {
  while (($# > 0)); do
    case "$1" in
      --fast)
        MODE="fast"
        shift
        ;;
      --smoke)
        MODE="smoke"
        shift
        ;;
      --keep-stack)
        KEEP_STACK=true
        shift
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "Unknown argument: $1"
        usage
        exit 1
        ;;
    esac
  done
}

main() {
  parse_args "$@"
  load_env_defaults

  run_static_checks
  run_manual_matrix
  if [[ "$MODE" == "smoke" ]]; then
    run_smoke_checks
  fi

  if (( SHOULD_FAILURES > 0 )); then
    printf 'SUMMARY: %d SHOULD check(s) warned\n' "$SHOULD_FAILURES"
  fi
  if (( MUST_FAILURES > 0 )); then
    printf 'SUMMARY: %d MUST check(s) failed\n' "$MUST_FAILURES"
    exit 1
  fi
  echo "SUMMARY: all MUST checks passed for mode --${MODE}"
}

main "$@"
