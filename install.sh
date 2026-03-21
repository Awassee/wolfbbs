#!/usr/bin/env bash
set -euo pipefail

DEFAULT_PREFIX_LINUX="/opt/wolfbbs"
DEFAULT_PREFIX_MACOS="${HOME}/.local/share/wolfbbs"
DEFAULT_SSH_PORT=2222
DEFAULT_WEB_PORT=8080
DEFAULT_IRC_PORT=6667
DEFAULT_IRC_TLS_PORT=6697
DEFAULT_MAILIN_PORT=8091
DEFAULT_CHECKOUT_SUBDIR="app"
DEFAULT_REPO_SLUG="Awassee/wolfbbs"
DEFAULT_REPO_URL="https://github.com/${DEFAULT_REPO_SLUG}.git"
DEFAULT_BBS_NAME="WolfBBS"
DEFAULT_SETUP_PROFILE="basic"

PREFIX=""
WITH_DOCKER=true
DRY_RUN=false
NON_INTERACTIVE=false
FORCE=false
SSH_PORT="$DEFAULT_SSH_PORT"
WEB_PORT="$DEFAULT_WEB_PORT"
IRC_PORT="$DEFAULT_IRC_PORT"
IRC_TLS_PORT="$DEFAULT_IRC_TLS_PORT"
MAILIN_PORT="$DEFAULT_MAILIN_PORT"
INSTALL_BREW=false
UNINSTALL=false
CLEAN_UNINSTALL=false
UPGRADE=false
RAPID_UPGRADE=false
STATUS=false
DOCTOR=false
START=false
STOP=false
RESTART=false
LOGS=false
REPAIR=false
DEPS_ONLY=false
DEBUG_BUNDLE=false
PORT_AUDIT=false
REPO_URL="${WOLFBBS_REPO_URL:-${WOLFBBS_GH:-}}"
BBS_NAME="${WOLFBBS_BBS_NAME:-$DEFAULT_BBS_NAME}"
BBS_HOSTNAME="${WOLFBBS_HOSTNAME:-}"
SETUP_PROFILE="${WOLFBBS_SETUP_PROFILE:-$DEFAULT_SETUP_PROFILE}"
OS=""
DISTRO=""
ID_LIKE=""
PKG_MGR=""
ARCH=""

SCRIPT_SOURCE="${BASH_SOURCE[0]:-$0}"
if SCRIPT_PATH_TMP="$(cd "$(dirname "$SCRIPT_SOURCE")" 2>/dev/null && pwd)"; then
  SCRIPT_PATH="$SCRIPT_PATH_TMP"
else
  SCRIPT_PATH="$(pwd)"
fi
LOG_FILE="${SCRIPT_PATH}/install.log"
WORK_DIR="${SCRIPT_PATH}"
PURGE=false
ENV_FILE=""
DOCKER_BIN="docker"
DOCKER_SOCKET_PATH="${WOLFBBS_DOCKER_SOCKET:-}"
BOOTSTRAP_ADMIN_HANDLE=""
BOOTSTRAP_ADMIN_PASSWORD=""
ENV_CREATED_THIS_RUN=false
SETUP_WIZARD_RAN=false
WIZARD_BOOTSTRAP_ADMIN_HANDLE=""
WIZARD_BOOTSTRAP_ADMIN_PASSWORD=""
WIZARD_BBS_NAME=""
WIZARD_BBS_HOSTNAME=""
WIZARD_SETUP_PROFILE=""
WIZARD_SECURE_COOKIE=""
WIZARD_REQUIRE_VERIFIED_EMAIL=""
WIZARD_MENU_ENABLE=""
WIZARD_TERM_ENCODING=""
USED_INSTALLER_CONFIG_FLAGS=false
AUTO_OPEN_ADVANCED_INSTALL=false

init_log_file() {
  local candidate=""
  local dir=""
  local candidates=()

  if [[ -n "${PREFIX:-}" ]]; then
    candidates+=("${PREFIX}/install.log")
  fi
  candidates+=("${SCRIPT_PATH}/install.log")
  candidates+=("${PWD}/install.log")
  candidates+=("/tmp/wolfbbs-install.log")
  candidates+=("/tmp/wolfbbs-install-$$.log")

  for candidate in "${candidates[@]}"; do
    dir="$(dirname "$candidate")"
    if mkdir -p "$dir" >/dev/null 2>&1 && touch "$candidate" >/dev/null 2>&1; then
      LOG_FILE="$candidate"
      return 0
    fi
  done
  echo "Unable to create installer log file in any standard location."
  exit 1
}

supports_color() {
  if [[ -n "${NO_COLOR:-}" ]]; then
    return 1
  fi
  if [[ ! -t 1 ]]; then
    return 1
  fi
  if ! command -v tput >/dev/null 2>&1; then
    return 1
  fi
  local colors
  colors="$(tput colors 2>/dev/null || echo 0)"
  [[ "${colors:-0}" -ge 8 ]]
}

style() {
  local code="$1"
  shift
  if supports_color; then
    printf '\033[%sm%s\033[0m' "$code" "$*"
  else
    printf '%s' "$*"
  fi
}

menu_divider() {
  printf '%s\n' "├──────────────────────────────────────────────────────────────────────────────┤"
}

menu_line() {
  local left="${1:-}"
  printf '│ %-76s │\n' "$left"
}

find_compose_file_in_dir() {
  local dir="$1"
  if [[ -f "${dir}/docker-compose.yml" ]]; then
    echo "${dir}/docker-compose.yml"
    return 0
  fi
  if [[ -f "${dir}/compose.yml" ]]; then
    echo "${dir}/compose.yml"
    return 0
  fi
  return 1
}

managed_checkout_dir() {
  printf '%s/%s' "$PREFIX" "$DEFAULT_CHECKOUT_SUBDIR"
}

find_installed_compose_file() {
  local managed_dir=""
  if find_compose_file_in_dir "$PREFIX" >/dev/null 2>&1; then
    find_compose_file_in_dir "$PREFIX"
    return 0
  fi
  managed_dir="$(managed_checkout_dir)"
  if find_compose_file_in_dir "$managed_dir" >/dev/null 2>&1; then
    find_compose_file_in_dir "$managed_dir"
    return 0
  fi
  return 1
}

adopt_installed_compose_if_present() {
  local installed_compose=""
  installed_compose="$(find_installed_compose_file || true)"
  if [[ -z "$installed_compose" ]]; then
    return 1
  fi
  compose_file="$installed_compose"
  WORK_DIR="$(dirname "$installed_compose")"
  return 0
}

prompt_default() {
  local label="$1"
  local current_value="$2"
  local reply=""
  printf '%s [%s]: ' "$label" "$current_value"
  read -r reply
  reply="$(trim "$reply")"
  if [[ -z "$reply" ]]; then
    printf '%s' "$current_value"
    return
  fi
  printf '%s' "$reply"
}

toggle_bool_flag() {
  local value="${1:-false}"
  if [[ "$value" == "true" ]]; then
    printf '%s' "false"
    return
  fi
  printf '%s' "true"
}

bool_word() {
  local value="${1:-false}"
  if [[ "$value" == "true" ]]; then
    printf '%s' "ON"
    return
  fi
  printf '%s' "OFF"
}

detect_compose_hint() {
  local local_compose=""
  local installed_compose=""

  local_compose="$(find_compose_file || true)"
  if [[ -n "$local_compose" ]]; then
    printf '%s' "$local_compose"
    return
  fi
  installed_compose="$(find_installed_compose_file || true)"
  if [[ -n "$installed_compose" ]]; then
    printf '%s' "$installed_compose"
    return
  fi
  printf '%s' "not detected yet"
}

show_interactive_advanced_install_menu() {
  local choice=""
  local setup_input=""
  local host_default=""
  local host_input=""

  while true; do
    host_default="${BBS_HOSTNAME:-auto}"
    echo "┌──────────────────── Guided Install: Advanced Options ───────────────────────┐"
    menu_line "Tune advanced defaults before install. Recommended path is still /admin/setup."
    menu_divider
    menu_line "Setup profile : ${SETUP_PROFILE}"
    menu_line "BBS name      : ${BBS_NAME}"
    menu_line "Hostname      : ${host_default}"
    menu_line "Dry run       : $(bool_word "$DRY_RUN")"
    menu_line "Force .env    : $(bool_word "$FORCE")"
    menu_line "Install brew  : $(bool_word "$INSTALL_BREW") (macOS only)"
    menu_divider
    menu_line "1) Set setup profile (basic|critical|expert)"
    menu_line "2) Set BBS display name"
    menu_line "3) Set hostname (or 'auto')"
    menu_line "4) Toggle dry-run"
    menu_line "5) Toggle force .env overwrite"
    menu_line "6) Toggle install-brew"
    menu_line "b) Back"
    echo "└──────────────────────────────────────────────────────────────────────────────┘"
    printf "Selection [b]: "
    read -r choice
    choice="$(trim "$choice")"
    if [[ -z "$choice" ]]; then
      choice="b"
    fi
    case "$(printf '%s' "$choice" | tr '[:upper:]' '[:lower:]')" in
      1|profile|setup)
        setup_input="$(prompt_default 'Setup profile (basic|critical|expert)' "$SETUP_PROFILE")"
        if is_valid_setup_profile "$setup_input"; then
          SETUP_PROFILE="$(normalize_setup_profile "$setup_input")"
        else
          echo "Invalid setup profile: ${setup_input}"
        fi
        ;;
      2|name|bbs)
        BBS_NAME="$(prompt_default 'BBS display name' "$BBS_NAME")"
        USED_INSTALLER_CONFIG_FLAGS=true
        ;;
      3|host|hostname)
        host_input="$(prompt_default "Public hostname (use 'auto' for detected host)" "$host_default")"
        if [[ "$(printf '%s' "$host_input" | tr '[:upper:]' '[:lower:]')" == "auto" ]]; then
          BBS_HOSTNAME=""
        else
          BBS_HOSTNAME="$host_input"
        fi
        USED_INSTALLER_CONFIG_FLAGS=true
        ;;
      4|dry|dry-run)
        DRY_RUN="$(toggle_bool_flag "$DRY_RUN")"
        ;;
      5|force)
        FORCE="$(toggle_bool_flag "$FORCE")"
        ;;
      6|brew|install-brew)
        INSTALL_BREW="$(toggle_bool_flag "$INSTALL_BREW")"
        ;;
      b|back|return)
        return
        ;;
      *)
        echo "Unknown selection: ${choice}"
        echo
        ;;
    esac
  done
}

show_interactive_troubleshooting_menu() {
  local choice=""
  local compose_hint=""
  local env_hint=""

  while true; do
    compose_hint="$(detect_compose_hint)"
    env_hint="${PREFIX}/.env"
    if [[ -f "$env_hint" ]]; then
      env_hint="${env_hint} (present)"
    else
      env_hint="${env_hint} (missing)"
    fi

    echo "┌───────────────────── WolfBBS Troubleshooting Center ────────────────────────┐"
    menu_line "Guided diagnostics and recovery actions for the current install."
    menu_divider
    menu_line "Prefix   : ${PREFIX}"
    menu_line "Compose  : ${compose_hint}"
    menu_line "Env file : ${env_hint}"
    menu_divider
    menu_line "1) Doctor diagnostics (safe, non-mutating)"
    menu_line "2) Status snapshot (endpoints + probes)"
    menu_line "3) Service logs (tail)"
    menu_line "4) Generate debug bundle (detailed diagnostics file)"
    menu_line "5) Port audit (listener + process scan)"
    menu_line "6) Repair stack (ensure deps/env + rebuild + verify)"
    menu_line "7) Restart services"
    menu_line "8) Start services"
    menu_line "9) Stop services"
    menu_line "b) Back to previous menu"
    menu_line "q) Quit installer"
    echo "└──────────────────────────────────────────────────────────────────────────────┘"
    printf "Selection [1]: "
    read -r choice
    choice="$(trim "$choice")"
    if [[ -z "$choice" ]]; then
      choice="1"
    fi
    case "$(printf '%s' "$choice" | tr '[:upper:]' '[:lower:]')" in
      1|doctor)
        DOCTOR=true
        return
        ;;
      2|status)
        STATUS=true
        return
        ;;
      3|logs|log)
        LOGS=true
        return
        ;;
      4|debug|bundle|debug-bundle)
        DEBUG_BUNDLE=true
        return
        ;;
      5|ports|port|port-audit)
        PORT_AUDIT=true
        return
        ;;
      6|repair)
        REPAIR=true
        return
        ;;
      7|restart)
        RESTART=true
        return
        ;;
      8|start)
        START=true
        return
        ;;
      9|stop)
        STOP=true
        return
        ;;
      b|back|return)
        return
        ;;
      q|quit|exit)
        echo "Aborted."
        exit 0
        ;;
      *)
        echo "Unknown selection: ${choice}"
        echo
        ;;
    esac
  done
}

show_interactive_install_plan() {
  local args_count="${1:-0}"
  local choice=""
  local repo_display=""

  if [[ "$args_count" -gt 0 ]]; then
    return
  fi
  if [[ "$NON_INTERACTIVE" == "true" || ! -t 0 || ! -t 1 ]]; then
    return
  fi
  if action_selected; then
    return
  fi
  if [[ "$AUTO_OPEN_ADVANCED_INSTALL" == "true" ]]; then
    AUTO_OPEN_ADVANCED_INSTALL=false
    show_interactive_advanced_install_menu
  fi

  while true; do
    repo_display="${REPO_URL:-$DEFAULT_REPO_URL}"
    echo "┌───────────────────── WolfBBS Guided Install Plan ───────────────────────────┐"
    menu_line "OpenClaw-style flow: quick defaults, plus optional advanced controls."
    menu_divider
    menu_line "Install dir : ${PREFIX}"
    menu_line "Code checkout: $(managed_checkout_dir)"
    menu_line "Ports       : SSH ${SSH_PORT} | Web ${WEB_PORT} | IRC ${IRC_PORT} | TLS ${IRC_TLS_PORT} | Mail ${MAILIN_PORT}"
    menu_line "Source repo : ${repo_display}"
    menu_line "Profile     : ${SETUP_PROFILE} | Dry-run: $(bool_word "$DRY_RUN") | Force .env: $(bool_word "$FORCE")"
    menu_line "Next step   : web setup at /admin/setup and /admin/config"
    menu_divider
    menu_line "1) Continue with recommended install"
    menu_line "2) Edit install directory"
    menu_line "3) Edit ports"
    menu_line "4) Change source repository"
    menu_line "5) Advanced install options (profile/name/hostname/toggles)"
    menu_line "6) Troubleshooting center"
    menu_line "7) Doctor diagnostics instead"
    menu_line "q) Quit"
    echo "└──────────────────────────────────────────────────────────────────────────────┘"
    printf "Selection [1]: "
    read -r choice
    choice="$(trim "$choice")"
    if [[ -z "$choice" ]]; then
      choice="1"
    fi
    case "$(printf '%s' "$choice" | tr '[:upper:]' '[:lower:]')" in
      1|continue|install)
        return
        ;;
      2|dir|prefix)
        PREFIX="$(prompt_default 'Install directory' "$PREFIX")"
        ;;
      3|ports|port)
        SSH_PORT="$(prompt_default 'SSH port' "$SSH_PORT")"
        WEB_PORT="$(prompt_default 'Web port' "$WEB_PORT")"
        IRC_PORT="$(prompt_default 'IRC port' "$IRC_PORT")"
        IRC_TLS_PORT="$(prompt_default 'IRC TLS port' "$IRC_TLS_PORT")"
        MAILIN_PORT="$(prompt_default 'Mail ingest port' "$MAILIN_PORT")"
        ;;
      4|repo|source)
        REPO_URL="$(normalize_repo_input "$(prompt_default 'Source repo (owner/repo or git URL)' "$repo_display")")"
        ;;
      5|advanced|options)
        show_interactive_advanced_install_menu
        ;;
      6|trouble|troubleshoot|diagnostics)
        show_interactive_troubleshooting_menu
        if action_selected; then
          return
        fi
        ;;
      7|doctor)
        DOCTOR=true
        return
        ;;
      q|quit|exit)
        echo "Aborted."
        exit 0
        ;;
      *)
        echo "Unknown selection: ${choice}"
        echo
        ;;
    esac
  done
}

print_splash() {
  local c1=""
  local c2=""
  local c3=""
  local dim=""
  local reset=""

  if supports_color; then
    c1=$'\033[1;36m'
    c2=$'\033[1;34m'
    c3=$'\033[1;33m'
    dim=$'\033[2m'
    reset=$'\033[0m'
  fi

  printf '\n'
  printf '%b\n' "${c1} __          __   _  __ ____  ____   ____   ____${reset}"
  printf '%b\n' "${c1} \\ \\        / /__| |/ // __ )| __ ) / ___| / ___|${reset}"
  printf '%b\n' "${c2}  \\ \\  /\\  / / _ \\ ' /|  _ \\|  _ \\ \\___ \\ \\___ \\${reset}"
  printf '%b\n' "${c2}   \\ \\/  \\/ /  __/ . \\| |_) | |_) | ___) | ___) |${reset}"
  printf '%b\n' "${c3}    \\__/\\__/ \\___|_|\\_\\____/|____/ |____/ |____/${reset}"
  printf '%b\n' "${c3}                     /\\_/\\    Installer Control Center${reset}"
  printf '%b\n' "${c3}                    ( o.o )   ${DEFAULT_BBS_NAME} rapid setup${reset}"
  printf '%b\n' "${c3}                     > ^ <    ops + troubleshooting${reset}"
  printf '%b\n' "${dim}Target: ${PREFIX} | Platform: ${OS}/${ARCH} | Profile: ${SETUP_PROFILE}${reset}"
  printf '%b\n' "${dim}Tip: choose Troubleshooting Center in the menu for guided recovery actions.${reset}"
  printf '\n'
}

read_env_value() {
  local key="$1"
  local file_path="$2"
  if [[ ! -f "$file_path" ]]; then
    return 0
  fi
  awk -F= -v lookup="$key" '$1 == lookup {sub(/^[^=]*=/, "", $0); print; exit}' "$file_path"
}

quote_env_literal() {
  local value="${1:-}"
  local escaped=""
  escaped="$(printf '%s' "$value" | sed "s/'/'\"'\"'/g")"
  printf "'%s'" "$escaped"
}

detect_docker_socket_path() {
  local host_value=""
  local candidate=""

  if [[ -n "${DOCKER_SOCKET_PATH:-}" ]]; then
    printf '%s' "$DOCKER_SOCKET_PATH"
    return
  fi

  host_value="${DOCKER_HOST:-}"
  if [[ "$host_value" == unix://* ]]; then
    candidate="${host_value#unix://}"
    if [[ -n "$candidate" ]]; then
      printf '%s' "$(mount_safe_docker_socket_path "$candidate")"
      return
    fi
  fi

  for candidate in \
    "/var/run/docker.sock" \
    "${HOME}/.colima/docker.sock" \
    "${HOME}/.colima/default/docker.sock" \
    "${HOME}/.docker/run/docker.sock"; do
    if [[ -S "$candidate" || -e "$candidate" ]]; then
      printf '%s' "$(mount_safe_docker_socket_path "$candidate")"
      return
    fi
  done

  printf '%s' "/var/run/docker.sock"
}

mount_safe_docker_socket_path() {
  local candidate="${1:-}"
  if [[ -z "$candidate" ]]; then
    printf '%s' "/var/run/docker.sock"
    return
  fi
  if is_macos; then
    case "$candidate" in
      "${HOME}/.colima/"*|\
      "${HOME}/.docker/run/"*|\
      "${HOME}/Library/Containers/"*)
        printf '%s' "/var/run/docker.sock"
        return
        ;;
    esac
  fi
  printf '%s' "$candidate"
}

append_env_value_if_missing() {
  local file_path="$1"
  local key="$2"
  local value="$3"

  if [[ ! -f "$file_path" ]]; then
    return
  fi
  if grep -q "^${key}=" "$file_path"; then
    return
  fi
  printf '%s=%s\n' "$key" "$value" >>"$file_path"
}

upsert_env_value() {
  local file_path="$1"
  local key="$2"
  local value="$3"
  local tmp_file=""

  if [[ ! -f "$file_path" ]]; then
    return
  fi

  tmp_file="$(mktemp "${TMPDIR:-/tmp}/wolfbbs-env.XXXXXX")"
  awk -v key="$key" -v value="$value" '
    BEGIN { replaced = 0 }
    index($0, key "=") == 1 {
      print key "=" value
      replaced = 1
      next
    }
    { print }
    END {
      if (replaced == 0) {
        print key "=" value
      }
    }
  ' "$file_path" >"$tmp_file"
  cat "$tmp_file" >"$file_path"
  rm -f "$tmp_file"
}

docs_root_path() {
  local candidates=(
    "${WORK_DIR}/docs"
    "$(managed_checkout_dir)/docs"
    "${PREFIX}/docs"
  )
  local candidate=""
  for candidate in "${candidates[@]}"; do
    if [[ -d "$candidate" ]]; then
      printf '%s' "$candidate"
      return 0
    fi
  done
  return 1
}

launch_brief_path() {
  printf '%s/%s' "$PREFIX" "FIRST_STEPS.txt"
}

status_snapshot_path() {
  printf '%s/%s' "$PREFIX" "SERVICE_STATUS.txt"
}

write_file_secure() {
  local target="$1"
  local mode="${2:-600}"
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would write ${target}"
    return 0
  fi
  chmod "$mode" "$target" >/dev/null 2>&1 || true
}

write_launch_brief() {
  local host="${1:-${BBS_HOSTNAME:-localhost}}"
  local bbs_name="${2:-${BBS_NAME:-$DEFAULT_BBS_NAME}}"
  local admin_handle="${3:-${BOOTSTRAP_ADMIN_HANDLE:-sysop}}"
  local docs_root=""
  local admin_login_url="http://${host}:${WEB_PORT}/admin/login"
  local admin_setup_url="http://${host}:${WEB_PORT}/admin/setup"
  local admin_system_url="http://${host}:${WEB_PORT}/admin/system"
  local boards_url="http://${host}:${WEB_PORT}/boards"
  local chat_url="http://${host}:${WEB_PORT}/chat"
  local doors_url="http://${host}:${WEB_PORT}/doors"
  local scores_url="http://${host}:${WEB_PORT}/scores"
  local out_file=""

  out_file="$(launch_brief_path)"
  docs_root="$(docs_root_path || true)"

  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would write launch brief to ${out_file}"
    return 0
  fi

  mkdir -p "$PREFIX"
  cat >"$out_file" <<EOF
WolfBBS First Steps
Generated: $(date -u +'%Y-%m-%dT%H:%M:%SZ')

Board:
- Name: ${bbs_name}
- Host: ${host}

Install layout:
- Prefix: ${PREFIX}
- Managed checkout: $(managed_checkout_dir)
- Env file: ${ENV_FILE:-${PREFIX}/.env}
- Compose file: ${compose_file:-$(managed_checkout_dir)/docker-compose.yml}
- Installer log: ${LOG_FILE}

Launch URLs:
- Admin login: ${admin_login_url}
- Admin setup: ${admin_setup_url}
- System dashboard: ${admin_system_url}
- Boards: ${boards_url}
- Chat: ${chat_url}
- Doors: ${doors_url}
- Scores: ${scores_url}
- SSH: ssh ${host} -p ${SSH_PORT}
- IRC: ${host}:${IRC_PORT} (TLS: ${host}:${IRC_TLS_PORT})
- Mail ingest: http://${host}:${MAILIN_PORT}/ingest

Bootstrap sysop:
- Handle: ${admin_handle}
- Password source: ${ENV_FILE:-${PREFIX}/.env}
- Password command: grep '^WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD=' '${ENV_FILE:-${PREFIX}/.env}' | cut -d= -f2-

10-minute launch path:
1. Open ${admin_login_url}
2. Finish ${admin_setup_url}
3. Review http://${host}:${WEB_PORT}/admin/config
4. Create a non-sysop user in http://${host}:${WEB_PORT}/admin/users
5. Validate ${boards_url}, ${chat_url}, ${doors_url}, and ${scores_url}
6. Run: bash install.sh --status

Recovery commands:
- bash install.sh --status
- bash install.sh --doctor
- bash install.sh --repair
- bash install.sh --logs

Docs:
EOF
  if [[ -n "$docs_root" ]]; then
    {
      [[ -f "${docs_root}/START_HERE.md" ]] && printf '%s\n' "- ${docs_root}/START_HERE.md"
      [[ -f "${docs_root}/LAUNCH_CHECKLIST.md" ]] && printf '%s\n' "- ${docs_root}/LAUNCH_CHECKLIST.md"
      [[ -f "${docs_root}/TROUBLESHOOTING.md" ]] && printf '%s\n' "- ${docs_root}/TROUBLESHOOTING.md"
      [[ -f "${docs_root}/OPERATIONS.md" ]] && printf '%s\n' "- ${docs_root}/OPERATIONS.md"
      [[ -f "${docs_root}/INSTALL.md" ]] && printf '%s\n' "- ${docs_root}/INSTALL.md"
    } >>"$out_file"
  else
    printf '%s\n' "- docs/START_HERE.md" "- docs/LAUNCH_CHECKLIST.md" "- docs/TROUBLESHOOTING.md" "- docs/OPERATIONS.md" "- docs/INSTALL.md" >>"$out_file"
  fi
  write_file_secure "$out_file" 600
}

write_status_snapshot() {
  local content="$1"
  local out_file=""

  out_file="$(status_snapshot_path)"
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would write service snapshot to ${out_file}"
    return 0
  fi
  mkdir -p "$PREFIX"
  printf '%s\n' "$content" >"$out_file"
  write_file_secure "$out_file" 600
}

default_hostname() {
  local value=""
  if command -v hostname >/dev/null 2>&1; then
    value="$(hostname -f 2>/dev/null || hostname 2>/dev/null || true)"
  fi
  value="${value%% *}"
  if [[ -z "$value" ]]; then
    value="localhost"
  fi
  printf '%s' "$value"
}

normalize_setup_profile() {
  local profile="${1:-}"
  local profile_lc
  profile_lc="$(printf '%s' "$profile" | tr '[:upper:]' '[:lower:]')"
  case "$profile_lc" in
    basic|critical|expert)
      printf '%s' "$profile_lc"
      ;;
    *)
      printf '%s' "basic"
      ;;
  esac
}

is_valid_setup_profile() {
  local profile="${1:-}"
  local profile_lc
  profile_lc="$(printf '%s' "$profile" | tr '[:upper:]' '[:lower:]')"
  case "$profile_lc" in
    basic|critical|expert)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

run_setup_wizard() {
  if [[ "$SETUP_WIZARD_RAN" == "true" ]]; then
    return
  fi
  if [[ "$NON_INTERACTIVE" == "true" || "$DRY_RUN" == "true" ]]; then
    return
  fi
  if [[ ! -t 0 || ! -t 1 ]]; then
    return
  fi

  SETUP_WIZARD_RAN=true

  local handle_input=""
  local handle_candidate="sysop"
  local password_mode="Y"
  local password_one=""
  local password_two=""
  local setup_profile_candidate
  setup_profile_candidate="$(normalize_setup_profile "${SETUP_PROFILE:-$DEFAULT_SETUP_PROFILE}")"

  echo "┌──────────────────────────────────────────────────────────────┐"
  echo "│ WolfBBS First-Run Setup Wizard                              │"
  echo "│ Bootstrap SYSOP credentials; configure everything else in UI │"
  echo "└──────────────────────────────────────────────────────────────┘"
  echo
  echo "Site name, hostname, setup profile, and runtime features are configured in:"
  echo "  Web UI -> /admin/setup and /admin/config"
  echo

  read -r -p "SYSOP handle [sysop]: " handle_input
  if [[ -n "$handle_input" ]]; then
    handle_candidate="$handle_input"
  fi
  if [[ "$handle_candidate" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{1,31}$ ]]; then
    WIZARD_BOOTSTRAP_ADMIN_HANDLE="$handle_candidate"
  else
    echo "Invalid handle format. Using default: sysop"
    WIZARD_BOOTSTRAP_ADMIN_HANDLE="sysop"
  fi

  read -r -p "Generate a random SYSOP password? [Y/n]: " password_mode
  if [[ "$password_mode" =~ ^[Nn]$ ]]; then
    while true; do
      read -r -s -p "Enter SYSOP password: " password_one
      echo
      read -r -s -p "Confirm SYSOP password: " password_two
      echo
      if [[ -z "$password_one" ]]; then
        echo "Password cannot be empty."
        continue
      fi
      if [[ "$password_one" != "$password_two" ]]; then
        echo "Passwords do not match. Try again."
        continue
      fi
      WIZARD_BOOTSTRAP_ADMIN_PASSWORD="$password_one"
      break
    done
  fi

  echo
  echo "Wizard summary:"
  echo "  Setup profile (env default): ${setup_profile_candidate}"
  echo "  SYSOP handle: ${WIZARD_BOOTSTRAP_ADMIN_HANDLE}"
  if [[ -n "$WIZARD_BOOTSTRAP_ADMIN_PASSWORD" ]]; then
    echo "  SYSOP password: custom (hidden)"
  else
    echo "  SYSOP password: auto-generated"
  fi
  echo "  UI setup path: /admin/setup (basic/critical/expert)"
  echo
}

print_first_login_wizard() {
  local host="${1:-$BBS_HOSTNAME}"
  local bbs_name="${BBS_NAME:-$DEFAULT_BBS_NAME}"
  local admin_handle="$BOOTSTRAP_ADMIN_HANDLE"
  local admin_password="$BOOTSTRAP_ADMIN_PASSWORD"
  local admin_users_url=""
  local boards_url=""
  local chat_url=""
  local doors_url=""
  local scores_url=""
  local docs_root=""
  local start_here_doc=""
  local ops_doc=""
  if [[ -z "$host" ]]; then
    host="localhost"
  fi
  if [[ -z "$bbs_name" ]]; then
    bbs_name="$DEFAULT_BBS_NAME"
  fi
  local admin_login_url="http://${host}:${WEB_PORT}/admin/login"
  local admin_setup_url="http://${host}:${WEB_PORT}/admin/setup"
  local admin_system_url="http://${host}:${WEB_PORT}/admin/system"
  admin_users_url="http://${host}:${WEB_PORT}/admin/users"
  boards_url="http://${host}:${WEB_PORT}/boards"
  chat_url="http://${host}:${WEB_PORT}/chat"
  doors_url="http://${host}:${WEB_PORT}/doors"
  scores_url="http://${host}:${WEB_PORT}/scores"

  if [[ -z "$admin_handle" && -f "$ENV_FILE" ]]; then
    admin_handle="$(read_env_value "WOLFBBS_BOOTSTRAP_ADMIN_HANDLE" "$ENV_FILE")"
  fi
  if [[ -z "$admin_password" && -f "$ENV_FILE" ]]; then
    admin_password="$(read_env_value "WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD" "$ENV_FILE")"
  fi
  if [[ -f "$ENV_FILE" ]]; then
    local env_host
    local env_name
    env_host="$(read_env_value "WOLFBBS_HOSTNAME" "$ENV_FILE")"
    env_name="$(read_env_value "WOLFBBS_BBS_NAME" "$ENV_FILE")"
    if [[ -n "$env_host" ]]; then
      host="$env_host"
    fi
    if [[ -n "$env_name" ]]; then
      bbs_name="$env_name"
    fi
  fi
  docs_root="${WORK_DIR}/docs"
  start_here_doc="${docs_root}/START_HERE.md"
  ops_doc="${docs_root}/OPERATIONS.md"

  echo
  echo "=================== First Login Wizard ==================="
  echo "BBS: ${bbs_name} (${host})"
  echo "1) Log in to SYSOP web panel:"
  echo "   URL: ${admin_login_url}"
  echo "   Handle: ${admin_handle:-sysop}"
  if [[ "$DRY_RUN" == "true" ]]; then
    echo "   Password: will be generated/stored in ${ENV_FILE} on real install"
  elif [[ "$ENV_CREATED_THIS_RUN" == "true" && -n "$admin_password" ]]; then
    echo "   Password: ${admin_password}"
  else
    echo "   Password: stored in ${ENV_FILE}"
    if [[ -n "$ENV_FILE" ]]; then
      echo "   View password: grep '^WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD=' '${ENV_FILE}' | cut -d= -f2-"
    fi
  fi
  echo
  echo "2) Complete initial SYSOP setup:"
  echo "   - Open ${admin_setup_url}"
  echo "   - Confirm health checks and baseline config"
  echo "   - Change bootstrap password after first login"
  echo "   - Seed default boards and verify bootstrap actions"
  echo
  echo "3) Verify caller access paths:"
  echo "   - SSH BBS: ssh ${host} -p ${SSH_PORT}"
  echo "   - Web Chat: ${chat_url}"
  echo "   - IRC: ${host}:${IRC_PORT}"
  echo
  echo "4) Check runtime status dashboard:"
  echo "   - ${admin_system_url}"
  echo
  echo "5) Walk the public product once before inviting users:"
  echo "   - Boards: ${boards_url}"
  echo "   - Doors: ${doors_url}"
  echo "   - Scores: ${scores_url}"
  echo "   - Users: ${admin_users_url}"
  if [[ -f "$start_here_doc" ]]; then
    echo
    echo "6) Read the operator guides in this checkout:"
    echo "   - Start here: ${start_here_doc}"
    if [[ -f "$ops_doc" ]]; then
      echo "   - Operations: ${ops_doc}"
    fi
  fi
  echo "=========================================================="
  echo "Saved first-steps brief: $(launch_brief_path)"
  echo
}

print_install_summary() {
  local host="${BBS_HOSTNAME:-localhost}"
  local bbs_name="${BBS_NAME:-$DEFAULT_BBS_NAME}"
  if [[ -f "$ENV_FILE" ]]; then
    local env_host
    local env_name
    env_host="$(read_env_value "WOLFBBS_HOSTNAME" "$ENV_FILE")"
    env_name="$(read_env_value "WOLFBBS_BBS_NAME" "$ENV_FILE")"
    if [[ -n "$env_host" ]]; then
      host="$env_host"
    fi
    if [[ -n "$env_name" ]]; then
      bbs_name="$env_name"
    fi
  fi
  write_launch_brief "$host" "$bbs_name" "${BOOTSTRAP_ADMIN_HANDLE:-sysop}"
  echo "WolfBBS installation complete."
  echo "BBS: ${bbs_name}"
  echo "Host: ${host}"
  echo "SSH: ssh ${host} -p ${SSH_PORT}"
  echo "Web Admin: http://${host}:${WEB_PORT}/admin"
  echo "Web Chat: http://${host}:${WEB_PORT}/chat"
  echo "IRC: ${host}:${IRC_PORT} (TLS: ${IRC_TLS_PORT})"
  echo "Mail Ingest: http://${host}:${MAILIN_PORT}/ingest"
  echo "First-steps brief: $(launch_brief_path)"
  print_first_login_wizard "$host"
}

usage() {
  cat <<'USAGE'
WolfBBS installer
Supports Linux (apt/dnf/yum/pacman) and macOS (Docker Desktop or Colima).

Usage:
  bash install.sh [options]
  bash install.sh           (interactive action menu)

Options:
  --prefix <dir>            install directory (default: Linux=/opt/wolfbbs, macOS=$HOME/.local/share/wolfbbs)
  --with-docker             use docker mode (default)
  --dry-run                 print actions without applying
  --yes, --non-interactive  run non-interactively
  --install-brew            on macOS, install Homebrew when missing (requires explicit flag)
  --force                   overwrite existing generated config
  --bbs-name <name>         ADVANCED: set BBS display name at install (prefer /admin/setup)
  --hostname <name>         ADVANCED: set public hostname at install (prefer /admin/setup)
  --setup-profile <name>    ADVANCED: basic|critical|expert baseline (prefer /admin/setup)
  --ssh-port <port>         SSH BBS port (default: 2222)
  --web-port <port>         web port (default: 8080)
  --irc-port <port>         IRC port (default: 6667)
  --irc-tls-port <port>     IRC TLS port suggestion (default: 6697)
  --mailin-port <port>      inbound mail webhook port (default: 8091)
  --uninstall               stop/remove services
  --upgrade                 pull/restart services in existing install
  --rapid-upgrade           local rebuild/restart for fast dev iteration
  --status                  show service status and endpoints
  --doctor                  run non-mutating preflight + install health diagnostics
  --debug-bundle            write a detailed diagnostics bundle under <prefix>
  --port-audit              inspect configured WolfBBS ports and listener ownership
  --start                   start existing WolfBBS services
  --stop                    stop existing WolfBBS services
  --restart                 restart existing WolfBBS services
  --logs                    show recent service logs (tail)
  --repair                  self-heal install: ensure deps/env, rebuild + verify stack
  --deps-only               install/check prerequisites and docker runtime, then exit
  --clean-uninstall         uninstall + purge + remove install directory (git checkout protected)
  --purge                   remove docker volumes/instance on uninstall
  --repo <owner/repo|url>   GitHub slug or URL to fetch if installer is run standalone
  --repo-url <url>          alias of --repo
  -h, --help                show this help

Environment shortcuts:
  WOLFBBS_GH=<owner/repo>         e.g. Awassee/wolfbbs
  WOLFBBS_REPO_URL=<repo-url>     e.g. https://github.com/Awassee/wolfbbs.git
  WOLFBBS_BBS_NAME=<name>         optional installer identity override (prefer /admin/setup)
  WOLFBBS_HOSTNAME=<host>         optional installer hostname override (prefer /admin/setup)
  WOLFBBS_SETUP_PROFILE=<profile> basic|critical|expert baseline (prefer /admin/setup)
  WOLFBBS_REPO_URL defaults to:   https://github.com/Awassee/wolfbbs.git

Operator files written under the install prefix:
  FIRST_STEPS.txt                 exact first-login and launch checklist summary
  SERVICE_STATUS.txt              last machine-readable-ish status snapshot from --status
USAGE
}

action_selected() {
  [[ "$UNINSTALL" == "true" ||
    "$UPGRADE" == "true" ||
    "$RAPID_UPGRADE" == "true" ||
    "$STATUS" == "true" ||
    "$DOCTOR" == "true" ||
    "$DEBUG_BUNDLE" == "true" ||
    "$PORT_AUDIT" == "true" ||
    "$START" == "true" ||
    "$STOP" == "true" ||
    "$RESTART" == "true" ||
    "$LOGS" == "true" ||
    "$REPAIR" == "true" ||
    "$DEPS_ONLY" == "true" ]]
}

show_interactive_action_menu() {
  local args_count="${1:-0}"
  local choice=""

  if [[ "$args_count" -gt 0 ]]; then
    return
  fi
  if [[ "$NON_INTERACTIVE" == "true" || ! -t 0 || ! -t 1 ]]; then
    return
  fi
  if action_selected; then
    return
  fi

  while true; do
    echo "┌──────────────────────── WolfBBS Installer Command Center ───────────────────┐"
    menu_line "Setup and Upgrade"
    menu_line "1) Easy install / first setup        Recommended for first-time operators"
    menu_line "2) Guided install options            Tune install profile and advanced defaults"
    menu_line "3) Rapid upgrade                     Rebuild and restart this checkout"
    menu_line "4) Upgrade                           Pull latest shipped images"
    menu_line "5) Repair                            Fix deps/env and verify the stack"
    menu_divider
    menu_line "Run"
    menu_line "6) Start services                    Bring the stack up"
    menu_line "7) Stop services                     Bring the stack down"
    menu_line "8) Restart services                  Restart all services"
    menu_line "9) Status                            Show endpoints, probes, next steps"
    menu_line "10) Logs                             Tail recent service logs"
    menu_divider
    menu_line "Troubleshoot"
    menu_line "11) Troubleshooting center           Guided diagnostics + recovery actions"
    menu_line "12) Doctor diagnostics               Safe preflight and health checks"
    menu_line "13) Port audit                       Show listener ownership for core ports"
    menu_line "14) Debug bundle                     Save deep diagnostics to a report file"
    menu_divider
    menu_line "Maintenance"
    menu_line "15) Uninstall                        Remove services, keep data"
    menu_line "16) Uninstall + purge                Remove services and data volumes"
    menu_line "17) Clean uninstall                  Remove services, data, and install files"
    menu_line "18) Dependencies only                Install/check prerequisites only"
    menu_divider
    menu_line "Docs"
    menu_line "Read docs/START_HERE.md for first launch and docs/OPERATIONS.md for day-two ops"
    menu_line "q) Quit"
    echo "└──────────────────────────────────────────────────────────────────────────────┘"
    printf "Selection [1]: "
    read -r choice
    choice="$(trim "$choice")"
    if [[ -z "$choice" ]]; then
      choice="1"
    fi
    choice="$(printf '%s' "$choice" | tr '[:upper:]' '[:lower:]')"

    case "$choice" in
      1|install)
        return
        ;;
      2|guided|options)
        AUTO_OPEN_ADVANCED_INSTALL=true
        return
        ;;
      3|rapid|rapid-upgrade)
        RAPID_UPGRADE=true
        return
        ;;
      4|upgrade)
        UPGRADE=true
        return
        ;;
      5|repair)
        REPAIR=true
        return
        ;;
      6|start)
        START=true
        return
        ;;
      7|stop)
        STOP=true
        return
        ;;
      8|restart)
        RESTART=true
        return
        ;;
      9|status)
        STATUS=true
        return
        ;;
      10|logs|log)
        LOGS=true
        return
        ;;
      11|trouble|troubleshoot|diagnostics)
        show_interactive_troubleshooting_menu
        if action_selected; then
          return
        fi
        ;;
      12|doctor)
        DOCTOR=true
        return
        ;;
      13|ports|port|port-audit)
        PORT_AUDIT=true
        return
        ;;
      14|debug|bundle|debug-bundle)
        DEBUG_BUNDLE=true
        return
        ;;
      15|uninstall)
        UNINSTALL=true
        return
        ;;
      16|purge|uninstall-purge|uninstall+purge)
        UNINSTALL=true
        PURGE=true
        return
        ;;
      17|clean|clean-uninstall|wipe|reset)
        UNINSTALL=true
        PURGE=true
        CLEAN_UNINSTALL=true
        return
        ;;
      18|deps|deps-only)
        DEPS_ONLY=true
        return
        ;;
      q|quit|exit)
        echo "Aborted."
        exit 0
        ;;
      *)
        echo "Unknown selection: ${choice}"
        echo
        ;;
    esac
  done
}

require_value() {
  local flag="$1"
  local value="${2:-}"
  if [[ -z "$value" ]]; then
    echo "Missing value for ${flag}"
    usage
    exit 1
  fi
}

prompt_repo_url() {
  local input=""
  if [[ "$NON_INTERACTIVE" == "true" ]]; then
    return
  fi
  printf "No local docker-compose file found. Enter repository (owner/repo or GitHub URL): "
  read -r input
  if [[ -n "$input" ]]; then
    REPO_URL="$(normalize_repo_input "$input")"
  fi
}

trim() {
  local value="${1:-}"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

normalize_repo_input() {
  local raw
  raw="$(trim "${1:-}")"
  if [[ -z "$raw" ]]; then
    printf '%s' ""
    return 0
  fi

  # Accept owner/repo shorthand and expand to GitHub HTTPS clone URL.
  if [[ "$raw" =~ ^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$ ]]; then
    printf 'https://github.com/%s.git' "$raw"
    return 0
  fi
  if [[ "$raw" =~ ^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+\.git$ ]]; then
    printf 'https://github.com/%s' "$raw"
    return 0
  fi

  if [[ "$raw" == github.com/* ]]; then
    raw="https://${raw}"
  fi

  # Keep canonical GitHub clone URLs untouched.
  if [[ "$raw" =~ ^https?://github\.com/[^/]+/[^/]+\.git/?$ ]]; then
    raw="${raw%/}"
    printf '%s' "$raw"
    return 0
  fi

  # Normalize bare GitHub https URLs to include .git suffix.
  if [[ "$raw" =~ ^https?://github\.com/[^/]+/[^/]+/?$ ]]; then
    raw="${raw%/}.git"
  fi

  printf '%s' "$raw"
}

repo_slug_from_url() {
  local raw
  raw="$(trim "${1:-}")"
  raw="${raw%/}"
  if [[ "$raw" =~ ^https?://github\.com/([^/]+/[^/]+)(\.git)?$ ]]; then
    printf '%s' "${BASH_REMATCH[1]}"
    return 0
  fi
  if [[ "$raw" =~ ^git@github\.com:([^/]+/[^/]+)(\.git)?$ ]]; then
    printf '%s' "${BASH_REMATCH[1]}"
    return 0
  fi
  if [[ "$raw" =~ ^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+(\.git)?$ ]]; then
    printf '%s' "${raw%.git}"
    return 0
  fi
  return 1
}

repo_archive_url() {
  local slug=""
  slug="$(repo_slug_from_url "${1:-}")" || return 1
  printf 'https://codeload.github.com/%s/tar.gz/refs/heads/main' "$slug"
}

has_working_git() {
  command -v git >/dev/null 2>&1 || return 1
  git --version >/dev/null 2>&1
}

download_repo_archive() {
  local source_repo="$1"
  local checkout_dir="$2"
  local archive_url=""
  local parent_dir=""
  local tmp_dir=""
  local archive_path=""
  local extracted_dir=""

  archive_url="$(repo_archive_url "$source_repo")" || {
    echo "Archive download fallback supports GitHub repositories only."
    echo "Install git, or use a GitHub owner/repo for --repo."
    exit 1
  }
  parent_dir="$(dirname "$checkout_dir")"
  tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/wolfbbs-repo.XXXXXX")"
  archive_path="${tmp_dir}/repo.tar.gz"

  run "mkdir -p '$parent_dir'"
  run_retry 3 3 "curl -fsSL '$archive_url' -o '$archive_path'"
  run "tar -xzf '$archive_path' -C '$tmp_dir'"
  extracted_dir="$(find "$tmp_dir" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
  if [[ -z "$extracted_dir" ]]; then
    echo "Unable to extract repository archive from ${archive_url}."
    exit 1
  fi
  run "rm -rf '$checkout_dir'"
  run "mv '$extracted_dir' '$checkout_dir'"
  run "rm -rf '$tmp_dir'"
}

resolve_repo_url() {
  if [[ -n "$REPO_URL" ]]; then
    REPO_URL="$(normalize_repo_input "$REPO_URL")"
    return
  fi

  # If installer is executed from a git checkout, prefer that remote.
  if [[ -d "${WORK_DIR}/.git" ]] && has_working_git; then
    local origin
    origin="$(git -C "$WORK_DIR" remote get-url origin 2>/dev/null || true)"
    if [[ -n "$origin" ]]; then
      REPO_URL="$(normalize_repo_input "$origin")"
      return
    fi
  fi

  # Fallback to project default so curl|bash stays one-command.
  REPO_URL="$(normalize_repo_input "$DEFAULT_REPO_URL")"
}

log() {
  local msg="$1"
  printf '[%s] %s\n' "$(date -u +'%Y-%m-%dT%H:%M:%SZ')" "$msg" | tee -a "$LOG_FILE"
}

run() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: $*"
    return 0
  fi
  log "RUN: $*"
  eval "$*"
}

run_retry() {
  local attempts="$1"
  local delay_seconds="$2"
  shift 2
  local cmd="$*"
  local attempt=1

  while true; do
    if run "$cmd"; then
      return 0
    fi
    if (( attempt >= attempts )); then
      log "Command failed after ${attempts} attempts: ${cmd}"
      return 1
    fi
    log "Retrying in ${delay_seconds}s (${attempt}/${attempts}): ${cmd}"
    sleep "$delay_seconds"
    attempt=$((attempt + 1))
  done
}

run_root() {
  local cmd="$*"
  if [[ "$(id -u)" -eq 0 ]]; then
    run "$cmd"
    return
  fi
  if ! command -v sudo >/dev/null 2>&1; then
    echo "This step requires elevated privileges, but sudo is not available."
    exit 1
  fi
  run "sudo $cmd"
}

run_root_retry() {
  local attempts="$1"
  local delay_seconds="$2"
  shift 2
  local cmd="$*"
  local attempt=1

  while true; do
    if run_root "$cmd"; then
      return 0
    fi
    if (( attempt >= attempts )); then
      log "Root command failed after ${attempts} attempts: ${cmd}"
      return 1
    fi
    log "Retrying root command in ${delay_seconds}s (${attempt}/${attempts}): ${cmd}"
    sleep "$delay_seconds"
    attempt=$((attempt + 1))
  done
}

confirm() {
  local prompt="$1"
  if [[ "$NON_INTERACTIVE" == "true" ]]; then
    return 0
  fi
  printf '%s [y/N] ' "$prompt"
  read -r reply
  [[ "$reply" =~ ^[Yy]$ ]]
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd"
    return 1
  fi
}

is_linux() {
  [[ "$(uname -s)" == "Linux" ]]
}

is_macos() {
  [[ "$(uname -s)" == "Darwin" ]]
}

find_compose_file() {
  find_compose_file_in_dir "$WORK_DIR"
}

compose_file="$(find_compose_file || true)"

init_install_dir() {
  if [[ -d "$PREFIX" ]]; then
    return
  fi
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: create directory $PREFIX"
  else
    run "mkdir -p '$PREFIX'"
    run "chmod 755 '$PREFIX'"
  fi
}

ensure_rootless_permissions() {
  if [[ "$(id -u)" -eq 0 ]]; then
    echo "Warning: running as root. Running as a normal user is preferred."
  fi
}

random_secret() {
  openssl rand -hex 32
}

detect_platform() {
  if is_linux; then
    OS="linux"
    if [[ -f /etc/os-release ]]; then
      # shellcheck disable=SC1091
      . /etc/os-release
      DISTRO="${ID,,}"
      ID_LIKE="${ID_LIKE,,}"
    else
      DISTRO="unknown"
      ID_LIKE=""
    fi
    return 0
  fi
  if is_macos; then
    OS="macos"
    DISTRO="macos"
    return 0
  fi
  OS="unknown"
  DISTRO="unknown"
}

set_default_prefix() {
  if [[ -n "$PREFIX" ]]; then
    return
  fi
  if is_macos; then
    PREFIX="$DEFAULT_PREFIX_MACOS"
    return
  fi
  PREFIX="$DEFAULT_PREFIX_LINUX"
}

detect_arch() {
  ARCH="$(uname -m 2>/dev/null || echo unknown)"
}

detect_package_manager() {
  if is_linux; then
    if command -v apt-get >/dev/null 2>&1; then
      PKG_MGR="apt"
    elif command -v dnf >/dev/null 2>&1; then
      PKG_MGR="dnf"
    elif command -v yum >/dev/null 2>&1; then
      PKG_MGR="yum"
    elif command -v pacman >/dev/null 2>&1; then
      PKG_MGR="pacman"
    else
      PKG_MGR=""
    fi
    return
  fi
  if is_macos; then
    PKG_MGR="brew"
    return
  fi
}

check_macos_prereqs() {
  if ! is_macos; then
    return
  fi
  if ! command -v xcode-select >/dev/null 2>&1; then
    echo "xcode-select is not available. Install Xcode Command Line Tools first:"
    echo "  xcode-select --install"
    exit 1
  fi
  if ! xcode-select -p >/dev/null 2>&1; then
    echo "Xcode Command Line Tools are required on macOS."
    echo "Run: xcode-select --install"
    exit 1
  fi
}

require_ports_free() {
  local ports=("$@")
  local port
  for port in "${ports[@]}"; do
    if [[ "$DRY_RUN" == "true" ]]; then
      log "DRY-RUN: would verify tcp port $port is free"
      continue
    fi
    local in_use=""
    if command -v ss >/dev/null 2>&1; then
      if ss -ltn | awk '{print $4}' | grep -q ":$port$"; then
        in_use=1
      fi
    elif command -v lsof >/dev/null 2>&1; then
      if lsof -iTCP -sTCP:LISTEN -P -n | grep -qE "[:.]${port}[[:space:]]"; then
        in_use=1
      fi
    elif command -v nc >/dev/null 2>&1; then
      if nc -z 127.0.0.1 "$port" >/dev/null 2>&1; then
        in_use=1
      fi
    fi
    if [[ -n "$in_use" ]]; then
      echo "Port $port is in use."
      if [[ "$NON_INTERACTIVE" == "true" ]]; then
        echo "Use --yes with different ports or stop the service on that port."
        exit 1
      fi
      if ! confirm "Continue anyway?"; then
        exit 1
      fi
    fi
  done
}

check_space() {
  local dir="$1"
  if [[ ! -d "$dir" ]]; then
    dir="$(dirname "$dir")"
  fi
  local free_kb
  free_kb=$(df -Pk "$dir" 2>/dev/null | awk 'NR==2 {print $4}')
  if [[ -z "${free_kb}" ]]; then
    return
  fi
  if (( free_kb < 2097152 )); then
    echo "Low disk in $dir: $((free_kb / 1024))MB free. At least 2GB is recommended."
    if [[ "$NON_INTERACTIVE" == "true" ]]; then
      exit 1
    fi
    if ! confirm "Continue anyway?"; then
      exit 1
    fi
  fi
}

linux_pkg_for_cmd() {
  local cmd="$1"
  case "$PKG_MGR" in
    apt)
      case "$cmd" in
        nc) echo "netcat-openbsd" ;;
        *) echo "$cmd" ;;
      esac
      ;;
    dnf|yum)
      case "$cmd" in
        nc) echo "nmap-ncat" ;;
        *) echo "$cmd" ;;
      esac
      ;;
    pacman)
      case "$cmd" in
        nc) echo "openbsd-netcat" ;;
        awk) echo "gawk" ;;
        *) echo "$cmd" ;;
      esac
      ;;
    *)
      echo "$cmd"
      ;;
  esac
}

macos_pkg_for_cmd() {
  local cmd="$1"
  case "$cmd" in
    openssl) echo "openssl@3" ;;
    nc) echo "netcat" ;;
    *) echo "$cmd" ;;
  esac
}

ensure_brew() {
  if ! is_macos; then
    return
  fi
  if command -v brew >/dev/null 2>&1; then
    return
  fi
  if [[ "$INSTALL_BREW" != "true" ]]; then
    if [[ "$NON_INTERACTIVE" != "true" ]] && confirm "Homebrew not found. Install Homebrew now?"; then
      INSTALL_BREW=true
    else
      echo "Homebrew not found."
      echo "Install Homebrew manually, or rerun with --install-brew."
      echo "  /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
      exit 1
    fi
  fi
  run_retry 3 3 "/bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
  if [[ -x /opt/homebrew/bin/brew ]]; then
    eval "$(/opt/homebrew/bin/brew shellenv)"
  elif [[ -x /usr/local/bin/brew ]]; then
    eval "$(/usr/local/bin/brew shellenv)"
  fi
  if ! command -v brew >/dev/null 2>&1; then
    echo "Failed to install Homebrew automatically."
    exit 1
  fi
}

install_base_prereqs() {
  local missing_cmds=("$@")
  if (( ${#missing_cmds[@]} == 0 )); then
    return
  fi

  log "Missing prerequisites: ${missing_cmds[*]}"
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would install missing prerequisites via ${PKG_MGR}"
    return
  fi

  local packages=()
  local cmd pkg
  for cmd in "${missing_cmds[@]}"; do
    if is_macos; then
      pkg="$(macos_pkg_for_cmd "$cmd")"
    else
      pkg="$(linux_pkg_for_cmd "$cmd")"
    fi
    packages+=("$pkg")
  done

  if is_macos; then
    ensure_brew
    run_retry 3 3 "brew install ${packages[*]}"
    return
  fi

  case "$PKG_MGR" in
    apt)
      run_root_retry 3 3 "apt-get update"
      run_root_retry 3 3 "apt-get install -y ${packages[*]}"
      ;;
    dnf)
      run_root_retry 3 3 "dnf -y install ${packages[*]}"
      ;;
    yum)
      run_root_retry 3 3 "yum -y install ${packages[*]}"
      ;;
    pacman)
      run_root_retry 3 3 "pacman -Sy --noconfirm --needed ${packages[*]}"
      ;;
    *)
      echo "Unsupported package manager for automated dependency install."
      exit 1
      ;;
  esac
}

ensure_base_prereqs() {
  local required=(curl tar sed awk grep openssl)
  if [[ "$DRY_RUN" == "false" ]]; then
    required+=(nc)
  fi
  local missing=()
  local cmd
  for cmd in "${required[@]}"; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      missing+=("$cmd")
    fi
  done
  if (( ${#missing[@]} == 0 )); then
    return
  fi
  install_base_prereqs "${missing[@]}"
  for cmd in "${required[@]}"; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      echo "Missing required command after install attempt: $cmd"
      exit 1
    fi
  done
}

compose_cmd() {
  if command -v docker >/dev/null 2>&1; then
    if eval "$DOCKER_BIN compose version" >/dev/null 2>&1; then
      echo "$DOCKER_BIN compose"
      return
    fi
  fi
  if command -v docker-compose >/dev/null 2>&1; then
    echo "docker-compose"
    return
  fi
  echo ""
}

install_colima_stack() {
  ensure_brew
  run_retry 3 3 "brew install docker docker-compose colima"
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would start Colima runtime"
    return
  fi
  if ! colima status >/dev/null 2>&1; then
    run "colima start"
  fi
}

wait_for_docker_daemon() {
  local timeout_seconds="${1:-90}"
  local elapsed=0
  while (( elapsed < timeout_seconds )); do
    if docker info >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  return 1
}

ensure_docker_linux() {
  if command -v docker >/dev/null 2>&1; then
    return
  fi
  echo "Docker is not installed."
  if [[ "$NON_INTERACTIVE" != "true" ]] && ! confirm "Install Docker Engine and compose plugin now?"; then
    echo "Install Docker manually: https://docs.docker.com/engine/install/"
    exit 1
  fi

  case "$PKG_MGR" in
    apt)
      run_root_retry 3 3 "apt-get update"
      run_root_retry 3 3 "apt-get install -y ca-certificates curl gnupg lsb-release"
      run_root "mkdir -p /etc/apt/keyrings"
      local docker_repo_distro="ubuntu"
      if [[ "$DISTRO" == "debian" ]] || [[ "$ID_LIKE" == *"debian"* && "$DISTRO" != "ubuntu" ]]; then
        docker_repo_distro="debian"
      fi
      run_root "bash -c 'curl -fsSL https://download.docker.com/linux/${docker_repo_distro}/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg'"
      local codename="${VERSION_CODENAME:-}"
      local apt_arch
      if [[ -z "$codename" ]] && command -v lsb_release >/dev/null 2>&1; then
        codename="$(lsb_release -cs)"
      fi
      if [[ -z "$codename" ]]; then
        echo "Could not determine Linux codename for Docker apt repo."
        exit 1
      fi
      apt_arch="$(dpkg --print-architecture)"
      run_root "bash -c 'echo \"deb [arch=${apt_arch} signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/${docker_repo_distro} ${codename} stable\" > /etc/apt/sources.list.d/docker.list'"
      run_root_retry 3 3 "apt-get update"
      run_root_retry 3 3 "apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin"
      ;;
    dnf|yum)
      if [[ "$PKG_MGR" == "dnf" ]]; then
        run_root "dnf -y install dnf-plugins-core"
        run_root "dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo"
      else
        run_root "yum -y install yum-utils"
        run_root "yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo"
      fi
      run_root_retry 3 3 "${PKG_MGR} -y install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin"
      ;;
    pacman)
      run_root_retry 3 3 "pacman -Sy --noconfirm docker docker-compose"
      ;;
    *)
      echo "Please install Docker manually and rerun the installer."
      echo "Linux docs: https://docs.docker.com/engine/install/"
      exit 1
      ;;
  esac

  if command -v systemctl >/dev/null 2>&1; then
    run_root "systemctl enable --now docker"
  fi
}

ensure_docker_macos() {
  check_macos_prereqs
  if command -v docker >/dev/null 2>&1; then
    if docker info >/dev/null 2>&1; then
      return
    fi
    if command -v open >/dev/null 2>&1; then
      if [[ -d "/Applications/Docker.app" ]]; then
        if [[ "$NON_INTERACTIVE" == "true" ]] || confirm "Docker daemon is down. Start Docker Desktop now?"; then
          run "open -a Docker"
          if wait_for_docker_daemon 120; then
            return
          fi
          log "Docker Desktop did not become ready in time."
        fi
      fi
    fi
    if command -v colima >/dev/null 2>&1; then
      if colima status >/dev/null 2>&1; then
        log "Docker CLI present and Colima is running."
        return
      fi
      if [[ "$NON_INTERACTIVE" == "true" ]]; then
        run "colima start"
      elif confirm "Docker daemon is down. Start Colima now?"; then
        run "colima start"
      else
        echo "Start Colima with: colima start"
        exit 1
      fi
      if wait_for_docker_daemon 120; then
        return
      fi
      log "Colima started but docker daemon is still unreachable."
      return
    fi
    echo "Docker CLI is present but Docker daemon is not reachable."
    echo "Start Docker Desktop (open -a Docker) or install Colima."
    exit 1
  fi

  echo "Docker is missing on macOS."
  if [[ "$NON_INTERACTIVE" == "true" ]]; then
    if [[ "$INSTALL_BREW" != "true" ]]; then
      echo "Rerun with --install-brew for automatic dependency setup on macOS."
      echo "Or install manually:"
      echo "  brew install docker colima docker-compose && colima start"
      exit 1
    fi
    install_colima_stack
    return
  fi

  if confirm "Install recommended Colima Docker stack now (docker + colima)?"; then
    install_colima_stack
    return
  fi
  if confirm "Install Docker Desktop via Homebrew cask instead?"; then
    ensure_brew
    run "brew install --cask docker"
    echo "Start Docker Desktop: open -a Docker"
    exit 1
  fi
  echo "Please install Docker Desktop or Colima and rerun."
  exit 1
}

ensure_compose_runtime() {
  if [[ -n "$(compose_cmd)" ]]; then
    return
  fi
  log "Docker Compose not found; installing compose runtime."
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would install Docker Compose runtime"
    return
  fi
  if is_macos; then
    ensure_brew
    run "brew install docker-compose"
  else
    case "$PKG_MGR" in
      apt)
        run_root_retry 3 3 "apt-get update"
        run_root_retry 3 3 "apt-get install -y docker-compose-plugin"
        ;;
      dnf|yum)
        run_root_retry 3 3 "${PKG_MGR} -y install docker-compose-plugin"
        ;;
      pacman)
        run_root_retry 3 3 "pacman -Sy --noconfirm docker-compose"
        ;;
      *)
        echo "Unsupported package manager for compose install."
        exit 1
        ;;
    esac
  fi
  if [[ -z "$(compose_cmd)" ]]; then
    echo "Docker Compose still unavailable after install attempt."
    exit 1
  fi
}

ensure_docker_access() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: skip docker daemon accessibility check"
    return
  fi
  if docker info >/dev/null 2>&1; then
    DOCKER_BIN="docker"
    return
  fi
  if is_linux && command -v systemctl >/dev/null 2>&1; then
    run_root "systemctl start docker" || true
    if docker info >/dev/null 2>&1; then
      DOCKER_BIN="docker"
      return
    fi
  fi
  if command -v sudo >/dev/null 2>&1; then
    if sudo -n docker info >/dev/null 2>&1; then
      DOCKER_BIN="sudo docker"
      return
    fi
    if [[ "$NON_INTERACTIVE" != "true" ]] && confirm "Docker requires elevated permissions. Use sudo for Docker commands?"; then
      DOCKER_BIN="sudo docker"
      return
    fi
  fi
  echo "Docker daemon is not reachable for the current user."
  echo "Start Docker and/or add this user to the docker group (Linux), then rerun."
  exit 1
}

ensure_docker() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: skip Docker install checks"
    return 0
  fi
  if is_macos; then
    ensure_docker_macos
  else
    ensure_docker_linux
  fi
  if ! command -v docker >/dev/null 2>&1; then
    echo "docker command still unavailable after setup."
    exit 1
  fi
  ensure_docker_access
  ensure_compose_runtime
}

resolve_env_file() {
  if [[ -f "${PREFIX}/.env" ]]; then
    echo "${PREFIX}/.env"
    return
  fi
  if [[ -f "${WORK_DIR}/.env" ]]; then
    echo "${WORK_DIR}/.env"
    return
  fi
}

ensure_compose_file() {
  local checkout_dir=""
  if [[ -n "$compose_file" ]]; then
    return
  fi

  if [[ -z "$REPO_URL" ]]; then
    resolve_repo_url
    prompt_repo_url
  fi

  if [[ -n "$REPO_URL" ]]; then
    checkout_dir="$(managed_checkout_dir)"
    if [[ -d "$checkout_dir" && -n "$(ls -A "$checkout_dir" 2>/dev/null)" && "$FORCE" != "true" ]]; then
      if [[ -d "$checkout_dir/.git" ]]; then
        echo "Managed code checkout will be updated: $checkout_dir"
      elif find_compose_file_in_dir "$checkout_dir" >/dev/null 2>&1; then
        echo "Managed code checkout will be refreshed from archive: $checkout_dir"
      else
        echo "Managed code checkout exists and is not empty: $checkout_dir"
        echo "Use --force to replace it, or choose a different --prefix."
        exit 1
      fi
    fi
    log "No local compose file found. Fetching repository from ${REPO_URL} into ${checkout_dir}."
    init_install_dir
    if [[ "$DRY_RUN" == "true" ]]; then
      WORK_DIR="$checkout_dir"
      compose_file="${checkout_dir}/docker-compose.yml"
      if has_working_git; then
        log "DRY-RUN: would clone repository to ${checkout_dir} and use ${compose_file}"
      else
        log "DRY-RUN: would download repository archive to ${checkout_dir} and use ${compose_file}"
      fi
      return
    fi
    run "mkdir -p '$(dirname "$checkout_dir")'"
    if [[ -d "$checkout_dir/.git" ]] && has_working_git; then
      run_retry 3 3 "git -C '$checkout_dir' pull --ff-only"
    elif [[ -d "$checkout_dir" && -n "$(ls -A "$checkout_dir" 2>/dev/null)" ]]; then
      download_repo_archive "$REPO_URL" "$checkout_dir"
    elif has_working_git; then
      run_retry 3 3 "git clone '$REPO_URL' '$checkout_dir'"
    else
      download_repo_archive "$REPO_URL" "$checkout_dir"
    fi
    WORK_DIR="$checkout_dir"
    compose_file="$(find_compose_file || true)"
    if [[ -n "$compose_file" ]]; then
      return
    fi
    echo "compose file still not found after clone."
    exit 1
  fi

  echo "Could not find docker-compose.yml or compose.yml."
  echo "Run from repository root, or pass --repo/--repo-url."
  echo "Examples:"
  echo "  bash install.sh --with-docker --repo Awassee/wolfbbs --yes"
  echo "  WOLFBBS_GH=Awassee/wolfbbs bash install.sh --with-docker --yes"
  exit 1
}

ensure_runtime_env_defaults() {
  local file_path="$1"
  local install_workdir=""
  local docker_socket=""
  local app_upgrade_workdir=""
  local app_upgrade_command=""
  local app_upgrade_timeout=""

  if [[ ! -f "$file_path" ]]; then
    return 0
  fi
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would ensure runtime env defaults in ${file_path}"
    return 0
  fi

  install_workdir="${WORK_DIR:-}"
  if [[ -z "$install_workdir" ]]; then
    install_workdir="$(managed_checkout_dir)"
  fi
  docker_socket="$(detect_docker_socket_path)"
  app_upgrade_workdir="/wolfbbs-host"
  app_upgrade_timeout="${WOLFBBS_APP_UPGRADE_TIMEOUT_SECONDS:-900}"
  app_upgrade_command='docker compose -f /wolfbbs-host/docker-compose.yml --env-file /wolfbbs-prefix/.env up -d --build --remove-orphans'

  append_env_value_if_missing "$file_path" "WOLFBBS_INSTALL_PREFIX" "$(quote_env_literal "${PREFIX}")"
  append_env_value_if_missing "$file_path" "WOLFBBS_INSTALL_WORKDIR" "$(quote_env_literal "${install_workdir}")"
  upsert_env_value "$file_path" "WOLFBBS_DOCKER_SOCKET" "$(quote_env_literal "${docker_socket}")"
  append_env_value_if_missing "$file_path" "WOLFBBS_APP_UPGRADE_WORKDIR" "$(quote_env_literal "${app_upgrade_workdir}")"
  append_env_value_if_missing "$file_path" "WOLFBBS_APP_UPGRADE_COMMAND" "$(quote_env_literal "${app_upgrade_command}")"
  append_env_value_if_missing "$file_path" "WOLFBBS_APP_UPGRADE_TIMEOUT_SECONDS" "$app_upgrade_timeout"
  chmod 600 "$file_path" >/dev/null 2>&1 || true
}

write_env_file() {
  ENV_FILE="${PREFIX}/.env"
  local db_pass db_user db_name db_seed bootstrap_admin_handle bootstrap_admin_password
  local bbs_name bbs_hostname setup_profile secure_cookie require_verified_email menu_enable term_encoding
  local install_workdir docker_socket app_upgrade_workdir app_upgrade_command app_upgrade_timeout

  if [[ -f "$ENV_FILE" && "$FORCE" != "true" ]]; then
    log "Using existing env file: $ENV_FILE"
    ensure_runtime_env_defaults "$ENV_FILE"
    ENV_CREATED_THIS_RUN=false
    BBS_NAME="$(read_env_value "WOLFBBS_BBS_NAME" "$ENV_FILE")"
    BBS_HOSTNAME="$(read_env_value "WOLFBBS_HOSTNAME" "$ENV_FILE")"
    SETUP_PROFILE="$(normalize_setup_profile "$(read_env_value "WOLFBBS_SETUP_PROFILE" "$ENV_FILE")")"
    BOOTSTRAP_ADMIN_HANDLE="$(read_env_value "WOLFBBS_BOOTSTRAP_ADMIN_HANDLE" "$ENV_FILE")"
    BOOTSTRAP_ADMIN_PASSWORD="$(read_env_value "WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD" "$ENV_FILE")"
    if [[ -z "$BBS_NAME" ]]; then
      BBS_NAME="$DEFAULT_BBS_NAME"
    fi
    if [[ -z "$BBS_HOSTNAME" ]]; then
      BBS_HOSTNAME="localhost"
    fi
    return
  fi
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would write ${ENV_FILE}"
    return
  fi
  if [[ -f "$ENV_FILE" && "$FORCE" == "true" ]]; then
    if ! confirm "Overwrite existing env file at ${ENV_FILE}?"; then
      log "Keeping existing env file."
      ensure_runtime_env_defaults "$ENV_FILE"
      ENV_CREATED_THIS_RUN=false
      BBS_NAME="$(read_env_value "WOLFBBS_BBS_NAME" "$ENV_FILE")"
      BBS_HOSTNAME="$(read_env_value "WOLFBBS_HOSTNAME" "$ENV_FILE")"
      SETUP_PROFILE="$(normalize_setup_profile "$(read_env_value "WOLFBBS_SETUP_PROFILE" "$ENV_FILE")")"
      BOOTSTRAP_ADMIN_HANDLE="$(read_env_value "WOLFBBS_BOOTSTRAP_ADMIN_HANDLE" "$ENV_FILE")"
      BOOTSTRAP_ADMIN_PASSWORD="$(read_env_value "WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD" "$ENV_FILE")"
      if [[ -z "$BBS_NAME" ]]; then
        BBS_NAME="$DEFAULT_BBS_NAME"
      fi
      if [[ -z "$BBS_HOSTNAME" ]]; then
        BBS_HOSTNAME="localhost"
      fi
      return
    fi
  fi

  db_user="wolfbbs"
  db_name="wolfbbs"
  db_pass="$(random_secret)"
  db_seed="$(random_secret)"
  bbs_name="${WOLFBBS_BBS_NAME:-${BBS_NAME:-$DEFAULT_BBS_NAME}}"
  bbs_hostname="${WOLFBBS_HOSTNAME:-${BBS_HOSTNAME:-}}"
  if [[ -z "$bbs_hostname" ]]; then
    bbs_hostname="$(default_hostname)"
  fi
  setup_profile="$(normalize_setup_profile "${WOLFBBS_SETUP_PROFILE:-${SETUP_PROFILE:-$DEFAULT_SETUP_PROFILE}}")"
  secure_cookie="${WOLFBBS_SECURE_COOKIE:-false}"
  require_verified_email="${WOLFBBS_REQUIRE_VERIFIED_EMAIL:-true}"
  menu_enable="${WOLFBBS_MENU_ENABLE:-false}"
  term_encoding="${WOLFBBS_TERM_ENCODING:-utf-8}"
  bootstrap_admin_handle="${WOLFBBS_BOOTSTRAP_ADMIN_HANDLE:-sysop}"
  bootstrap_admin_password="${WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD:-$(random_secret)}"

  run_setup_wizard
  if [[ -n "$WIZARD_BBS_NAME" ]]; then
    bbs_name="$WIZARD_BBS_NAME"
  fi
  if [[ -n "$WIZARD_BBS_HOSTNAME" ]]; then
    bbs_hostname="$WIZARD_BBS_HOSTNAME"
  fi
  if [[ -n "$WIZARD_SETUP_PROFILE" ]]; then
    setup_profile="$WIZARD_SETUP_PROFILE"
  fi
  if [[ -n "$WIZARD_BOOTSTRAP_ADMIN_HANDLE" ]]; then
    bootstrap_admin_handle="$WIZARD_BOOTSTRAP_ADMIN_HANDLE"
  fi
  if [[ -n "$WIZARD_BOOTSTRAP_ADMIN_PASSWORD" ]]; then
    bootstrap_admin_password="$WIZARD_BOOTSTRAP_ADMIN_PASSWORD"
  fi
  if [[ -n "$WIZARD_SECURE_COOKIE" ]]; then
    secure_cookie="$WIZARD_SECURE_COOKIE"
  fi
  if [[ -n "$WIZARD_REQUIRE_VERIFIED_EMAIL" ]]; then
    require_verified_email="$WIZARD_REQUIRE_VERIFIED_EMAIL"
  fi
  if [[ -n "$WIZARD_MENU_ENABLE" ]]; then
    menu_enable="$WIZARD_MENU_ENABLE"
  fi
  if [[ -n "$WIZARD_TERM_ENCODING" ]]; then
    term_encoding="$WIZARD_TERM_ENCODING"
  fi

  install_workdir="${WORK_DIR:-}"
  if [[ -z "$install_workdir" ]]; then
    install_workdir="$(managed_checkout_dir)"
  fi
  docker_socket="$(detect_docker_socket_path)"
  app_upgrade_workdir="/wolfbbs-host"
  app_upgrade_timeout="${WOLFBBS_APP_UPGRADE_TIMEOUT_SECONDS:-900}"
  app_upgrade_command='docker compose -f /wolfbbs-host/docker-compose.yml --env-file /wolfbbs-prefix/.env up -d --build --remove-orphans'

  cat > "$ENV_FILE" <<EOF
# Basic setup profile
WOLFBBS_SETUP_PROFILE=${setup_profile}
WOLFBBS_BBS_NAME=${bbs_name}
WOLFBBS_HOSTNAME=${bbs_hostname}

# Core data services
WOLFBBS_DATABASE_URL=postgres://$db_user:$db_pass@postgres:5432/$db_name?sslmode=disable
POSTGRES_USER=$db_user
POSTGRES_PASSWORD=$db_pass
POSTGRES_DB=$db_name
WOLFBBS_DB_CONNECT_RETRIES=15
WOLFBBS_DB_CONNECT_DELAY_MS=500

# Critical security
WOLFBBS_SESSION_SECRET=$db_seed
WOLFBBS_INBOUND_TOKEN=$(random_secret)
WOLFBBS_SECURE_COOKIE=${secure_cookie}
WOLFBBS_READ_ONLY=false
WOLFBBS_REQUIRE_VERIFIED_EMAIL=${require_verified_email}

# Network ports
WOLFBBS_OFFLINE_DIR=/app/.wolfbbs/offline
WOLFBBS_SSH_PORT=${SSH_PORT}
WOLFBBS_WEB_PORT=${WEB_PORT}
WOLFBBS_IRC_PORT=${IRC_PORT}
WOLFBBS_IRC_TLS_PORT=${IRC_TLS_PORT}
WOLFBBS_MAILIN_PORT=${MAILIN_PORT}

# Expert runtime
WOLFBBS_TERM_ENCODING=${term_encoding}
WOLFBBS_MENU_ENABLE=${menu_enable}
WOLFBBS_MENU_FILE=menus/main.hjson
WOLFBBS_INSTALL_PREFIX=$(quote_env_literal "${PREFIX}")
WOLFBBS_INSTALL_WORKDIR=$(quote_env_literal "${install_workdir}")
WOLFBBS_DOCKER_SOCKET=$(quote_env_literal "${docker_socket}")
WOLFBBS_APP_UPGRADE_WORKDIR=$(quote_env_literal "${app_upgrade_workdir}")
WOLFBBS_APP_UPGRADE_COMMAND=$(quote_env_literal "${app_upgrade_command}")
WOLFBBS_APP_UPGRADE_TIMEOUT_SECONDS=${app_upgrade_timeout}

# Bootstrap users
WOLFBBS_BOOTSTRAP_ADMIN_HANDLE=${bootstrap_admin_handle}
WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD=${bootstrap_admin_password}
WOLFBBS_BOOTSTRAP_MODERATOR_HANDLE=
WOLFBBS_BOOTSTRAP_MODERATOR_PASSWORD=
WOLFBBS_BOOTSTRAP_USER_HANDLE=
WOLFBBS_BOOTSTRAP_USER_PASSWORD=
EOF
  chmod 600 "$ENV_FILE"
  log "Wrote ${ENV_FILE}"
  ENV_CREATED_THIS_RUN=true
  BBS_NAME="$bbs_name"
  BBS_HOSTNAME="$bbs_hostname"
  SETUP_PROFILE="$setup_profile"
  BOOTSTRAP_ADMIN_HANDLE="$bootstrap_admin_handle"
  BOOTSTRAP_ADMIN_PASSWORD="$bootstrap_admin_password"
}

docker_compose_up() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would start compose services"
    return 0
  fi
  local cmd
  cmd="$(compose_cmd)"
  if [[ -z "$cmd" ]]; then
    echo "Docker Compose not found."
    exit 1
  fi
  local env_flag=""
  if [[ -n "${ENV_FILE:-}" && -f "$ENV_FILE" ]]; then
    env_flag=" --env-file \"$ENV_FILE\""
  fi
  run_retry 3 5 "cd '$WORK_DIR' && $cmd -f \"$compose_file\"${env_flag} up -d --build"
}

docker_compose_pull_restart() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would pull and restart compose services"
    return 0
  fi
  local cmd
  cmd="$(compose_cmd)"
  if [[ -z "$cmd" ]]; then
    echo "Docker Compose not found."
    exit 1
  fi
  local env_flag=""
  if [[ -n "${ENV_FILE:-}" && -f "$ENV_FILE" ]]; then
    env_flag=" --env-file \"$ENV_FILE\""
  fi
  run_retry 3 5 "cd '$WORK_DIR' && $cmd -f \"$compose_file\"${env_flag} pull"
  run_retry 3 5 "cd '$WORK_DIR' && $cmd -f \"$compose_file\"${env_flag} up -d --build --remove-orphans"
}

docker_compose_rapid_upgrade() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would rebuild and restart compose services from local source"
    return 0
  fi
  local cmd
  cmd="$(compose_cmd)"
  if [[ -z "$cmd" ]]; then
    echo "Docker Compose not found."
    exit 1
  fi
  local env_flag=""
  if [[ -n "${ENV_FILE:-}" && -f "$ENV_FILE" ]]; then
    env_flag=" --env-file \"$ENV_FILE\""
  fi
  run_retry 3 5 "cd '$WORK_DIR' && $cmd -f \"$compose_file\"${env_flag} up -d --build --remove-orphans"
}

docker_compose_down() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would stop compose services"
    return 0
  fi
  local cmd
  cmd="$(compose_cmd)"
  if [[ -z "$cmd" ]]; then
    echo "Docker Compose not found."
    exit 1
  fi
  local env_flag=""
  if [[ -n "${ENV_FILE:-}" && -f "$ENV_FILE" ]]; then
    env_flag=" --env-file \"$ENV_FILE\""
  fi
  run "cd '$WORK_DIR' && $cmd -f \"$compose_file\"${env_flag} down --remove-orphans"
}

docker_compose_down_purge() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would stop and purge compose services"
    return 0
  fi
  local cmd
  cmd="$(compose_cmd)"
  if [[ -z "$cmd" ]]; then
    echo "Docker Compose not found."
    exit 1
  fi
  local env_flag=""
  if [[ -n "${ENV_FILE:-}" && -f "$ENV_FILE" ]]; then
    env_flag=" --env-file \"$ENV_FILE\""
  fi
  run "cd '$WORK_DIR' && $cmd -f \"$compose_file\"${env_flag} down -v --remove-orphans"
}

docker_compose_status() {
  local cmd
  cmd="$(compose_cmd)"
  if [[ -z "$cmd" ]]; then
    echo "Docker Compose not found."
    return 1
  fi
  if [[ -n "${ENV_FILE:-}" && -f "$ENV_FILE" ]]; then
    run "$cmd -f '$compose_file' --env-file '$ENV_FILE' ps"
    return
  fi
  run "$cmd -f '$compose_file' ps"
}

docker_compose_start() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would start compose services"
    return 0
  fi
  local cmd
  cmd="$(compose_cmd)"
  if [[ -z "$cmd" ]]; then
    echo "Docker Compose not found."
    exit 1
  fi
  local env_flag=""
  if [[ -n "${ENV_FILE:-}" && -f "$ENV_FILE" ]]; then
    env_flag=" --env-file \"$ENV_FILE\""
  fi
  run "cd '$WORK_DIR' && $cmd -f \"$compose_file\"${env_flag} up -d"
}

docker_compose_stop() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would stop compose services"
    return 0
  fi
  local cmd
  cmd="$(compose_cmd)"
  if [[ -z "$cmd" ]]; then
    echo "Docker Compose not found."
    exit 1
  fi
  local env_flag=""
  if [[ -n "${ENV_FILE:-}" && -f "$ENV_FILE" ]]; then
    env_flag=" --env-file \"$ENV_FILE\""
  fi
  run "cd '$WORK_DIR' && $cmd -f \"$compose_file\"${env_flag} stop"
}

docker_compose_restart() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would restart compose services"
    return 0
  fi
  local cmd
  cmd="$(compose_cmd)"
  if [[ -z "$cmd" ]]; then
    echo "Docker Compose not found."
    exit 1
  fi
  local env_flag=""
  if [[ -n "${ENV_FILE:-}" && -f "$ENV_FILE" ]]; then
    env_flag=" --env-file \"$ENV_FILE\""
  fi
  run "cd '$WORK_DIR' && $cmd -f \"$compose_file\"${env_flag} restart"
}

docker_compose_logs() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would show compose service logs"
    return 0
  fi
  local cmd
  cmd="$(compose_cmd)"
  if [[ -z "$cmd" ]]; then
    echo "Docker Compose not found."
    exit 1
  fi
  local env_flag=""
  if [[ -n "${ENV_FILE:-}" && -f "$ENV_FILE" ]]; then
    env_flag=" --env-file \"$ENV_FILE\""
  fi
  run "cd '$WORK_DIR' && $cmd -f \"$compose_file\"${env_flag} logs --tail=200"
}

seed_admin_check() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: skipping sysop user seeding check"
    return
  fi
  if [[ ! -f "$ENV_FILE" ]]; then
    log "No .env file found for sysop bootstrap check."
    return
  fi
  local handle
  handle="$(grep '^WOLFBBS_BOOTSTRAP_ADMIN_HANDLE=' "$ENV_FILE" | head -n1 | cut -d= -f2- || true)"
  if [[ -z "$handle" ]]; then
    log "Warning: no bootstrap sysop configured in ${ENV_FILE}."
  else
    log "Bootstrap sysop account configured: ${handle}"
  fi
}

wait_for_port() {
  local host="$1"
  local port="$2"
  local label="$3"
  local attempts=20
  local i
  for ((i=1; i<=attempts; i++)); do
    if nc -z "$host" "$port" >/dev/null 2>&1; then
      log "${label} is reachable on ${host}:${port}"
      return 0
    fi
    if [[ "$DRY_RUN" == "true" ]]; then
      break
    fi
    sleep 1
  done
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would verify ${label} on ${host}:${port}"
    return 0
  fi
  echo "Timeout waiting for ${label} on ${host}:${port}"
  return 1
}

verify_install() {
  local runtime_ssh_port="$SSH_PORT"
  local runtime_web_port="$WEB_PORT"
  local runtime_irc_port="$IRC_PORT"
  local runtime_mailin_port="$MAILIN_PORT"
  local env_ssh=""
  local env_web=""
  local env_irc=""
  local env_mailin=""
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: skip runtime checks."
    return
  fi
  resolve_runtime_context_if_available
  if [[ -n "${ENV_FILE:-}" && -f "$ENV_FILE" ]]; then
    env_ssh="$(read_env_value "WOLFBBS_SSH_PORT" "$ENV_FILE")"
    env_web="$(read_env_value "WOLFBBS_WEB_PORT" "$ENV_FILE")"
    env_irc="$(read_env_value "WOLFBBS_IRC_PORT" "$ENV_FILE")"
    env_mailin="$(read_env_value "WOLFBBS_MAILIN_PORT" "$ENV_FILE")"
    if [[ -n "$env_ssh" ]]; then
      runtime_ssh_port="$env_ssh"
    fi
    if [[ -n "$env_web" ]]; then
      runtime_web_port="$env_web"
    fi
    if [[ -n "$env_irc" ]]; then
      runtime_irc_port="$env_irc"
    fi
    if [[ -n "$env_mailin" ]]; then
      runtime_mailin_port="$env_mailin"
    fi
  fi
  if ! command -v curl >/dev/null 2>&1; then
    echo "curl missing; cannot verify services."
    return 1
  fi
  if ! curl -fsS "http://127.0.0.1:${runtime_web_port}/healthz" >/dev/null; then
    echo "web service health check failed"
    return 1
  fi
  if ! curl -fsS "http://127.0.0.1:${runtime_web_port}/readyz" >/dev/null; then
    echo "web service readiness check failed"
    return 1
  fi
  wait_for_port 127.0.0.1 "$runtime_ssh_port" "SSH BBS"
  wait_for_port 127.0.0.1 "$runtime_irc_port" "IRC"
  wait_for_port 127.0.0.1 "$runtime_mailin_port" "Mail Ingest"
}

status_view() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: status skipped"
    return
  fi
  if [[ ! -f "$ENV_FILE" ]]; then
    echo "No install found in ${PREFIX}. Missing ${ENV_FILE}."
    exit 1
  fi
  echo "WolfBBS install status: ${PREFIX}"
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  local status_host="${WOLFBBS_HOSTNAME:-localhost}"
  local status_name="${WOLFBBS_BBS_NAME:-$DEFAULT_BBS_NAME}"
  local runtime_ssh_port="${WOLFBBS_SSH_PORT:-$SSH_PORT}"
  local runtime_web_port="${WOLFBBS_WEB_PORT:-$WEB_PORT}"
  local runtime_irc_port="${WOLFBBS_IRC_PORT:-$IRC_PORT}"
  local runtime_irc_tls_port="${WOLFBBS_IRC_TLS_PORT:-$IRC_TLS_PORT}"
  local runtime_mailin_port="${WOLFBBS_MAILIN_PORT:-$MAILIN_PORT}"
  local docs_root=""
  local probe_lines=()
  local pass_count=0
  local warn_count=0
  local safety_lines=()
  local safety_pass=0
  local safety_warn=0
  local verdict="ATTENTION"
  local cmd=""
  local env_mode=""
  local snapshot=""

  docs_root="$(docs_root_path || true)"
  write_launch_brief "$status_host" "$status_name" "${WOLFBBS_BOOTSTRAP_ADMIN_HANDLE:-sysop}"

  echo "BBS Name: ${status_name}"
  echo "Setup Profile: ${WOLFBBS_SETUP_PROFILE:-basic}"
  echo "SSH: ssh ${status_host} -p ${runtime_ssh_port}"
  echo "Web: http://${status_host}:${runtime_web_port}/admin"
  echo "Chat: http://${status_host}:${runtime_web_port}/chat"
  echo "IRC: ${status_host}:${runtime_irc_port} (TLS: ${status_host}:${runtime_irc_tls_port})"
  echo "Mail Ingest: http://${status_host}:${runtime_mailin_port}/ingest"
  echo "Install layout:"
  echo "  Prefix: ${PREFIX}"
  echo "  Managed checkout: $(managed_checkout_dir)"
  echo "  Env file: ${ENV_FILE}"
  echo "  Compose file: ${compose_file}"
  echo "  First-steps brief: $(launch_brief_path)"
  echo "  Service snapshot: $(status_snapshot_path)"
  if [[ -n "${WOLFBBS_BOOTSTRAP_ADMIN_HANDLE:-}" ]]; then
    echo "Bootstrap sysop handle: ${WOLFBBS_BOOTSTRAP_ADMIN_HANDLE} (password stored in ${ENV_FILE})"
  fi
  cmd="$(compose_cmd)"
  if [[ -n "$cmd" ]]; then
    echo "Compose status:"
    eval "$cmd -f '$compose_file' --env-file '$ENV_FILE' ps" || true
  fi
  echo "Runtime probes:"
  if command -v curl >/dev/null 2>&1; then
    if curl -fsS "http://127.0.0.1:${runtime_web_port}/healthz" >/dev/null 2>&1; then
      echo "  PASS web healthz: http://127.0.0.1:${runtime_web_port}/healthz"
      probe_lines+=("PASS web healthz: http://127.0.0.1:${runtime_web_port}/healthz")
      pass_count=$((pass_count + 1))
    else
      echo "  WARN web healthz unreachable: http://127.0.0.1:${runtime_web_port}/healthz"
      probe_lines+=("WARN web healthz unreachable: http://127.0.0.1:${runtime_web_port}/healthz")
      warn_count=$((warn_count + 1))
    fi
    if curl -fsS "http://127.0.0.1:${runtime_web_port}/readyz" >/dev/null 2>&1; then
      echo "  PASS web readyz: http://127.0.0.1:${runtime_web_port}/readyz"
      probe_lines+=("PASS web readyz: http://127.0.0.1:${runtime_web_port}/readyz")
      pass_count=$((pass_count + 1))
    else
      echo "  WARN web readyz unreachable: http://127.0.0.1:${runtime_web_port}/readyz"
      probe_lines+=("WARN web readyz unreachable: http://127.0.0.1:${runtime_web_port}/readyz")
      warn_count=$((warn_count + 1))
    fi
  else
    echo "  WARN curl not found; skipping HTTP probes"
    probe_lines+=("WARN curl not found; skipping HTTP probes")
    warn_count=$((warn_count + 1))
  fi
  if command -v nc >/dev/null 2>&1; then
    if nc -z 127.0.0.1 "$runtime_ssh_port" >/dev/null 2>&1; then
      echo "  PASS ssh port ${runtime_ssh_port} reachable"
      probe_lines+=("PASS ssh port ${runtime_ssh_port} reachable")
      pass_count=$((pass_count + 1))
    else
      echo "  WARN ssh port ${runtime_ssh_port} unreachable"
      probe_lines+=("WARN ssh port ${runtime_ssh_port} unreachable")
      warn_count=$((warn_count + 1))
    fi
    if nc -z 127.0.0.1 "$runtime_irc_port" >/dev/null 2>&1; then
      echo "  PASS irc port ${runtime_irc_port} reachable"
      probe_lines+=("PASS irc port ${runtime_irc_port} reachable")
      pass_count=$((pass_count + 1))
    else
      echo "  WARN irc port ${runtime_irc_port} unreachable"
      probe_lines+=("WARN irc port ${runtime_irc_port} unreachable")
      warn_count=$((warn_count + 1))
    fi
    if nc -z 127.0.0.1 "$runtime_mailin_port" >/dev/null 2>&1; then
      echo "  PASS mail ingest port ${runtime_mailin_port} reachable"
      probe_lines+=("PASS mail ingest port ${runtime_mailin_port} reachable")
      pass_count=$((pass_count + 1))
    else
      echo "  WARN mail ingest port ${runtime_mailin_port} unreachable"
      probe_lines+=("WARN mail ingest port ${runtime_mailin_port} unreachable")
      warn_count=$((warn_count + 1))
    fi
  else
    echo "  WARN nc not found; skipping TCP probes"
    probe_lines+=("WARN nc not found; skipping TCP probes")
    warn_count=$((warn_count + 1))
  fi
  if [[ -f "$ENV_FILE" ]]; then
    env_mode="$(stat -f '%Lp' "$ENV_FILE" 2>/dev/null || stat -c '%a' "$ENV_FILE" 2>/dev/null || true)"
  fi
  echo "Upgrade safety dashboard:"
  if [[ -f "$ENV_FILE" ]]; then
    echo "  PASS env file present: ${ENV_FILE}"
    safety_lines+=("PASS env file present: ${ENV_FILE}")
    safety_pass=$((safety_pass + 1))
  else
    echo "  WARN env file missing: ${ENV_FILE}"
    safety_lines+=("WARN env file missing: ${ENV_FILE}")
    safety_warn=$((safety_warn + 1))
  fi
  if [[ -n "${env_mode:-}" ]] && [[ "$env_mode" == "600" ]]; then
    echo "  PASS env permissions: ${env_mode}"
    safety_lines+=("PASS env permissions: ${env_mode}")
    safety_pass=$((safety_pass + 1))
  else
    echo "  WARN env permissions: ${env_mode:-unknown} (recommended 600)"
    safety_lines+=("WARN env permissions: ${env_mode:-unknown} (recommended 600)")
    safety_warn=$((safety_warn + 1))
  fi
  if [[ -f "$compose_file" ]]; then
    echo "  PASS compose file present: ${compose_file}"
    safety_lines+=("PASS compose file present: ${compose_file}")
    safety_pass=$((safety_pass + 1))
  else
    echo "  WARN compose file missing: ${compose_file}"
    safety_lines+=("WARN compose file missing: ${compose_file}")
    safety_warn=$((safety_warn + 1))
  fi
  if [[ -f "$(launch_brief_path)" ]]; then
    echo "  PASS first-steps brief present: $(launch_brief_path)"
    safety_lines+=("PASS first-steps brief present: $(launch_brief_path)")
    safety_pass=$((safety_pass + 1))
  else
    echo "  WARN first-steps brief missing: $(launch_brief_path)"
    safety_lines+=("WARN first-steps brief missing: $(launch_brief_path)")
    safety_warn=$((safety_warn + 1))
  fi
  if [[ -f "$(status_snapshot_path)" ]]; then
    echo "  PASS service snapshot present: $(status_snapshot_path)"
    safety_lines+=("PASS service snapshot present: $(status_snapshot_path)")
    safety_pass=$((safety_pass + 1))
  else
    echo "  WARN service snapshot missing: $(status_snapshot_path)"
    safety_lines+=("WARN service snapshot missing: $(status_snapshot_path)")
    safety_warn=$((safety_warn + 1))
  fi
  local menu_backup_count=0
  if [[ -d "${WORK_DIR}/menus" ]]; then
    menu_backup_count="$(find "${WORK_DIR}/menus" -type f -name '*.bak' 2>/dev/null | wc -l | awk '{print $1}')"
  fi
  if [[ "${menu_backup_count:-0}" -gt 0 ]]; then
    echo "  PASS menu backup files: ${menu_backup_count}"
    safety_lines+=("PASS menu backup files: ${menu_backup_count}")
    safety_pass=$((safety_pass + 1))
  else
    echo "  WARN menu backup files: 0 (save /admin/config at least once before risky changes)"
    safety_lines+=("WARN menu backup files: 0 (save /admin/config at least once before risky changes)")
    safety_warn=$((safety_warn + 1))
  fi
  echo "Upgrade safety verdict: ${safety_pass} pass / ${safety_warn} warn"
  if [[ "$warn_count" -eq 0 ]]; then
    verdict="READY"
  elif [[ "$pass_count" -gt 0 ]]; then
    verdict="PARTIAL"
  fi
  echo "Launch verdict: ${verdict} (${pass_count} pass / ${warn_count} warn)"
  echo "Recommended next actions:"
  echo "  1) Finish /admin/setup if this is a first install or recent rebuild"
  echo "  2) Review /admin/config for runtime flags and host identity"
  echo "  3) Walk /boards, /chat, /doors, and /scores as a real user"
  echo "  4) Use bash install.sh --doctor before changing ports or proxies"
  echo "  5) Open $(launch_brief_path) for the operator handoff summary"
  if [[ -n "$docs_root" ]]; then
    echo "Operator guides:"
    [[ -f "${docs_root}/START_HERE.md" ]] && echo "  - ${docs_root}/START_HERE.md"
    [[ -f "${docs_root}/LAUNCH_CHECKLIST.md" ]] && echo "  - ${docs_root}/LAUNCH_CHECKLIST.md"
    [[ -f "${docs_root}/TROUBLESHOOTING.md" ]] && echo "  - ${docs_root}/TROUBLESHOOTING.md"
    [[ -f "${docs_root}/OPERATIONS.md" ]] && echo "  - ${docs_root}/OPERATIONS.md"
  fi
  snapshot="WolfBBS Service Status
Generated: $(date -u +'%Y-%m-%dT%H:%M:%SZ')
Verdict: ${verdict}
Pass: ${pass_count}
Warn: ${warn_count}

Board:
- Name: ${status_name}
- Host: ${status_host}

Install layout:
- Prefix: ${PREFIX}
- Managed checkout: $(managed_checkout_dir)
- Env file: ${ENV_FILE}
- Env mode: ${env_mode:-unknown}
- Compose file: ${compose_file}
- First-steps brief: $(launch_brief_path)

Endpoints:
- SSH: ssh ${status_host} -p ${runtime_ssh_port}
- Admin: http://${status_host}:${runtime_web_port}/admin
- Chat: http://${status_host}:${runtime_web_port}/chat
- IRC: ${status_host}:${runtime_irc_port} (TLS: ${status_host}:${runtime_irc_tls_port})
- Mail ingest: http://${status_host}:${runtime_mailin_port}/ingest

Runtime probes:
"
  if (( ${#probe_lines[@]} > 0 )); then
    snapshot+=$(printf -- '- %s\n' "${probe_lines[@]}")
  else
    snapshot+="- No probes executed"$'\n'
  fi
  snapshot+=$'\nUpgrade safety:\n'
  if (( ${#safety_lines[@]} > 0 )); then
    snapshot+=$(printf -- '- %s\n' "${safety_lines[@]}")
  else
    snapshot+="- No safety checks executed"$'\n'
  fi
  snapshot+=$'\nDocs:\n'
  if [[ -n "$docs_root" ]]; then
    [[ -f "${docs_root}/START_HERE.md" ]] && snapshot+="- ${docs_root}/START_HERE.md"$'\n'
    [[ -f "${docs_root}/LAUNCH_CHECKLIST.md" ]] && snapshot+="- ${docs_root}/LAUNCH_CHECKLIST.md"$'\n'
    [[ -f "${docs_root}/TROUBLESHOOTING.md" ]] && snapshot+="- ${docs_root}/TROUBLESHOOTING.md"$'\n'
    [[ -f "${docs_root}/OPERATIONS.md" ]] && snapshot+="- ${docs_root}/OPERATIONS.md"$'\n'
  fi
  write_status_snapshot "$snapshot"
}

doctor_ok() {
  printf 'PASS doctor: %s\n' "$1"
}

doctor_warn() {
  printf 'WARN doctor: %s\n' "$1"
}

doctor_fail() {
  printf 'FAIL doctor: %s\n' "$1"
}

doctor_report() {
  local failures=0
  local compose_cmd_value=""
  local local_compose=""
  local install_compose=""
  local install_env=""
  local runtime_ssh_port="$SSH_PORT"
  local runtime_web_port="$WEB_PORT"
  local runtime_irc_port="$IRC_PORT"
  local runtime_mailin_port="$MAILIN_PORT"
  local env_ssh=""
  local env_web=""
  local env_irc=""
  local env_mailin=""
  local blockers=()
  local warnings=()
  local docs_root=""

  echo "WolfBBS doctor report"
  echo "  os=${OS} distro=${DISTRO} arch=${ARCH} pkg=${PKG_MGR:-none}"
  echo "  prefix=${PREFIX}"
  docs_root="$(docs_root_path || true)"

  if [[ "$OS" == "unknown" ]]; then
    doctor_fail "unsupported operating system"
    failures=$((failures + 1))
    blockers+=("Unsupported operating system.")
  else
    doctor_ok "supported OS detected"
  fi

  local required=(curl tar openssl sed awk grep)
  local missing=()
  local cmd
  for cmd in "${required[@]}"; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      missing+=("$cmd")
    fi
  done
  if (( ${#missing[@]} > 0 )); then
    doctor_fail "missing required commands: ${missing[*]}"
    failures=$((failures + 1))
    blockers+=("Install missing commands: ${missing[*]}.")
  else
    doctor_ok "required base commands are present"
  fi

  if command -v docker >/dev/null 2>&1; then
    doctor_ok "docker CLI found"
  else
    doctor_fail "docker CLI missing"
    failures=$((failures + 1))
    blockers+=("Docker CLI is missing.")
  fi

  compose_cmd_value="$(compose_cmd)"
  if [[ -n "$compose_cmd_value" ]]; then
    doctor_ok "docker compose command detected: ${compose_cmd_value}"
  else
    doctor_fail "docker compose command not found"
    failures=$((failures + 1))
    blockers+=("Docker Compose is not available.")
  fi

  if command -v docker >/dev/null 2>&1; then
    if docker info >/dev/null 2>&1; then
      doctor_ok "docker daemon reachable"
    else
      doctor_warn "docker daemon not reachable for current user"
      warnings+=("Docker daemon is not reachable for the current user.")
    fi
  fi

  local_compose="$(find_compose_file || true)"
  if [[ -n "$local_compose" ]]; then
    doctor_ok "compose file in working dir: ${local_compose}"
  else
    doctor_warn "no compose file in working dir (installer can clone via --repo)"
    warnings+=("No compose file in the current working directory.")
  fi

  install_compose="$(find_installed_compose_file || true)"
  if [[ -n "$install_compose" ]]; then
    doctor_ok "compose file in install layout: ${install_compose}"
  else
    doctor_warn "no compose file in install layout under ${PREFIX}"
    warnings+=("No compose file found under ${PREFIX}.")
  fi

  install_env="${PREFIX}/.env"
  if [[ -f "$install_env" ]]; then
    doctor_ok "env file found: ${install_env}"
    env_ssh="$(read_env_value "WOLFBBS_SSH_PORT" "$install_env")"
    env_web="$(read_env_value "WOLFBBS_WEB_PORT" "$install_env")"
    env_irc="$(read_env_value "WOLFBBS_IRC_PORT" "$install_env")"
    env_mailin="$(read_env_value "WOLFBBS_MAILIN_PORT" "$install_env")"
    if [[ -n "$env_ssh" ]]; then
      runtime_ssh_port="$env_ssh"
    fi
    if [[ -n "$env_web" ]]; then
      runtime_web_port="$env_web"
    fi
    if [[ -n "$env_irc" ]]; then
      runtime_irc_port="$env_irc"
    fi
    if [[ -n "$env_mailin" ]]; then
      runtime_mailin_port="$env_mailin"
    fi
    local mode
    mode="$(stat -f '%Lp' "$install_env" 2>/dev/null || stat -c '%a' "$install_env" 2>/dev/null || true)"
    if [[ "$mode" == "600" ]]; then
      doctor_ok ".env permissions are 600"
    elif [[ -n "$mode" ]]; then
      doctor_warn ".env permissions are ${mode} (recommended 600)"
      warnings+=(".env permissions are ${mode}; recommended 600.")
    fi
  else
    doctor_warn "no env file in install prefix (expected before first install)"
    warnings+=("No .env found in the install prefix yet.")
  fi

  if command -v nc >/dev/null 2>&1; then
    if nc -z 127.0.0.1 "$runtime_ssh_port" >/dev/null 2>&1; then
      doctor_ok "ssh port ${runtime_ssh_port} is reachable"
    else
      doctor_warn "ssh port ${runtime_ssh_port} is not reachable"
      warnings+=("SSH port ${runtime_ssh_port} is not reachable.")
    fi
    if nc -z 127.0.0.1 "$runtime_irc_port" >/dev/null 2>&1; then
      doctor_ok "irc port ${runtime_irc_port} is reachable"
    else
      doctor_warn "irc port ${runtime_irc_port} is not reachable"
      warnings+=("IRC port ${runtime_irc_port} is not reachable.")
    fi
    if nc -z 127.0.0.1 "$runtime_mailin_port" >/dev/null 2>&1; then
      doctor_ok "mail ingest port ${runtime_mailin_port} is reachable"
    else
      doctor_warn "mail ingest port ${runtime_mailin_port} is not reachable"
      warnings+=("Mail ingest port ${runtime_mailin_port} is not reachable.")
    fi
  else
    doctor_warn "netcat (nc) not found; skipping port checks"
    warnings+=("Netcat is not available, so TCP port checks were skipped.")
  fi

  if command -v curl >/dev/null 2>&1; then
    if curl -fsS "http://127.0.0.1:${runtime_web_port}/healthz" >/dev/null 2>&1; then
      doctor_ok "web health endpoint is reachable on port ${runtime_web_port}"
    else
      doctor_warn "web health endpoint not reachable on port ${runtime_web_port}"
      warnings+=("Web health endpoint on port ${runtime_web_port} is not reachable.")
    fi
  fi

  echo
  echo "Doctor summary:"
  if (( ${#blockers[@]} > 0 )); then
    echo "  Blocking:"
    printf '  - %s\n' "${blockers[@]}"
  else
    echo "  Blocking: none"
  fi
  if (( ${#warnings[@]} > 0 )); then
    echo "  Warnings:"
    printf '  - %s\n' "${warnings[@]}"
  else
    echo "  Warnings: none"
  fi
  echo "Recommended commands:"
  if (( failures > 0 )); then
    echo "  - Fix blocking issues, then rerun: bash install.sh --doctor"
    echo "  - Capture context for support: bash install.sh --debug-bundle"
    echo "  - If this is a clean machine, use: curl -fsSL https://raw.githubusercontent.com/Awassee/wolfbbs/main/bootstrap.sh | bash"
  else
    echo "  - Check runtime + next steps: bash install.sh --status"
    echo "  - If a service looks unhealthy: bash install.sh --repair"
    echo "  - Capture context for support: bash install.sh --debug-bundle"
  fi
  if [[ -n "$docs_root" ]]; then
    echo "Operator docs:"
    [[ -f "${docs_root}/START_HERE.md" ]] && echo "  - ${docs_root}/START_HERE.md"
    [[ -f "${docs_root}/LAUNCH_CHECKLIST.md" ]] && echo "  - ${docs_root}/LAUNCH_CHECKLIST.md"
    [[ -f "${docs_root}/TROUBLESHOOTING.md" ]] && echo "  - ${docs_root}/TROUBLESHOOTING.md"
    [[ -f "${docs_root}/OPERATIONS.md" ]] && echo "  - ${docs_root}/OPERATIONS.md"
  fi
  echo "First-steps brief: $(launch_brief_path)"
  echo "Service snapshot: $(status_snapshot_path)"

  if (( failures > 0 )); then
    echo "Doctor found ${failures} blocking issue(s)."
    return 1
  fi
  echo "Doctor completed with no blocking issues."
  return 0
}

resolve_runtime_context_if_available() {
  local detected_compose=""
  detected_compose="$(find_compose_file || true)"
  if [[ -z "$detected_compose" ]]; then
    detected_compose="$(find_installed_compose_file || true)"
  fi
  if [[ -n "$detected_compose" ]]; then
    compose_file="$detected_compose"
    WORK_DIR="$(dirname "$detected_compose")"
  fi
  ENV_FILE="$(resolve_env_file || true)"
}

compose_context_available() {
  [[ -n "${compose_file:-}" && -f "${compose_file:-}" ]]
}

ensure_action_compose_context() {
  resolve_runtime_context_if_available
  if compose_context_available; then
    return 0
  fi
  echo "No compose file found for this action."
  echo "Checked current directory and install prefix:"
  echo "  - work dir: ${WORK_DIR}"
  echo "  - prefix: ${PREFIX}"
  echo "If this is a first install, run: bash install.sh"
  echo "If this is an existing install, rerun with --prefix <install_dir>."
  return 1
}

derive_compose_project_candidates() {
  local seen=" "
  local raw=""
  local normalized=""
  local candidates=(
    "wolfbbs"
    "$(basename "${WORK_DIR:-wolfbbs}")"
    "$(basename "${PREFIX:-wolfbbs}")"
  )

  if [[ -n "${compose_file:-}" ]]; then
    candidates+=("$(basename "$(dirname "$compose_file")")")
  fi

  for raw in "${candidates[@]}"; do
    normalized="$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]' | tr -cd 'a-z0-9_-')"
    if [[ -z "$normalized" ]]; then
      continue
    fi
    case "$seen" in
      *" ${normalized} "*)
        ;;
      *)
        printf '%s\n' "$normalized"
        seen="${seen}${normalized} "
        ;;
    esac
  done
}

docker_cleanup_without_compose() {
  local remove_volumes="${1:-false}"
  local project=""
  local container_ids=""
  local network_ids=""
  local volume_ids=""
  local legacy_container=""

  if ! command -v docker >/dev/null 2>&1; then
    log "Docker CLI not found; skipping compose-less cleanup."
    return 0
  fi
  if ! eval "$DOCKER_BIN info" >/dev/null 2>&1; then
    log "Docker daemon is unreachable; skipping compose-less cleanup."
    return 0
  fi

  while IFS= read -r project; do
    if [[ -z "$project" ]]; then
      continue
    fi
    container_ids="$(eval "$DOCKER_BIN ps -aq --filter label=com.docker.compose.project=${project}" 2>/dev/null || true)"
    if [[ -n "$container_ids" ]]; then
      run "$DOCKER_BIN rm -f ${container_ids}" || true
    fi

    network_ids="$(eval "$DOCKER_BIN network ls -q --filter label=com.docker.compose.project=${project}" 2>/dev/null || true)"
    if [[ -n "$network_ids" ]]; then
      run "$DOCKER_BIN network rm ${network_ids}" || true
    fi

    if [[ "$remove_volumes" == "true" ]]; then
      volume_ids="$(eval "$DOCKER_BIN volume ls -q --filter label=com.docker.compose.project=${project}" 2>/dev/null || true)"
      if [[ -n "$volume_ids" ]]; then
        run "$DOCKER_BIN volume rm -f ${volume_ids}" || true
      fi
    fi
  done < <(derive_compose_project_candidates)

  for legacy_container in wolfbbs-bbs-1 wolfbbs-web-1 wolfbbs-irc-1 wolfbbs-mailin-1 wolfbbs-postgres-1; do
    run "$DOCKER_BIN rm -f '$legacy_container' >/dev/null 2>&1 || true"
  done
  run "$DOCKER_BIN network rm 'wolfbbs_default' >/dev/null 2>&1 || true"
  if [[ "$remove_volumes" == "true" ]]; then
    run "$DOCKER_BIN volume rm -f 'wolfbbs_pgdata' >/dev/null 2>&1 || true"
  fi
}

remove_install_prefix() {
  local resolved=""
  if [[ ! -d "$PREFIX" ]]; then
    return 0
  fi

  if ! resolved="$(cd "$PREFIX" 2>/dev/null && pwd)"; then
    resolved="$PREFIX"
  fi

  case "$resolved" in
    ""|"/"|"/Users"|"/home"|"/opt"|"/usr"|"/var"|"/tmp"|"$HOME")
      echo "Safety stop: refusing to remove protected path: ${resolved}"
      return 1
      ;;
  esac

  if [[ -d "${resolved}/.git" && "$FORCE" != "true" ]]; then
    echo "Refusing to remove git checkout at ${resolved} without --force."
    echo "Run with --clean-uninstall --force to delete this checkout."
    return 1
  fi

  rm -rf "$resolved"
  echo "Removed ${resolved}."
}

port_listener_details() {
  local port="$1"
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | sed -n '2,5p' || true
    return
  fi
  if command -v ss >/dev/null 2>&1; then
    ss -ltnp 2>/dev/null | awk -v match=":${port}" '
      index($4, match) > 0 { print; found = 1 }
      END { if (found != 1) exit 1 }
    ' || true
    return
  fi
  echo "listener ownership unavailable (install lsof or ss)"
}

port_audit() {
  local runtime_ssh_port="$SSH_PORT"
  local runtime_web_port="$WEB_PORT"
  local runtime_irc_port="$IRC_PORT"
  local runtime_mailin_port="$MAILIN_PORT"
  local open_count=0
  local closed_count=0
  local unknown_count=0
  local pair=""
  local label=""
  local port=""
  local state=""
  local details=""
  local env_ssh=""
  local env_web=""
  local env_irc=""
  local env_mailin=""

  resolve_runtime_context_if_available
  if [[ -n "${ENV_FILE:-}" && -f "$ENV_FILE" ]]; then
    env_ssh="$(read_env_value "WOLFBBS_SSH_PORT" "$ENV_FILE")"
    env_web="$(read_env_value "WOLFBBS_WEB_PORT" "$ENV_FILE")"
    env_irc="$(read_env_value "WOLFBBS_IRC_PORT" "$ENV_FILE")"
    env_mailin="$(read_env_value "WOLFBBS_MAILIN_PORT" "$ENV_FILE")"
    if [[ -n "$env_ssh" ]]; then
      runtime_ssh_port="$env_ssh"
    fi
    if [[ -n "$env_web" ]]; then
      runtime_web_port="$env_web"
    fi
    if [[ -n "$env_irc" ]]; then
      runtime_irc_port="$env_irc"
    fi
    if [[ -n "$env_mailin" ]]; then
      runtime_mailin_port="$env_mailin"
    fi
  fi

  echo "WolfBBS port audit"
  echo "  prefix=${PREFIX}"
  echo "  compose=${compose_file:-not detected}"
  echo "  env=${ENV_FILE:-not found}"
  echo
  for pair in \
    "SSH BBS:${runtime_ssh_port}" \
    "Web UI:${runtime_web_port}" \
    "IRC:${runtime_irc_port}" \
    "Mail Ingest:${runtime_mailin_port}"; do
    label="${pair%%:*}"
    port="${pair##*:}"
    if command -v nc >/dev/null 2>&1; then
      if nc -z 127.0.0.1 "$port" >/dev/null 2>&1; then
        state="LISTENING"
        open_count=$((open_count + 1))
      else
        state="CLOSED"
        closed_count=$((closed_count + 1))
      fi
    else
      state="UNKNOWN (nc unavailable)"
      unknown_count=$((unknown_count + 1))
    fi
    echo "  - ${label} (${port}): ${state}"
    details="$(port_listener_details "$port" || true)"
    if [[ -n "$details" ]]; then
      while IFS= read -r detail_line; do
        if [[ -n "$detail_line" ]]; then
          echo "      ${detail_line}"
        fi
      done <<<"$details"
    fi
  done
  echo
  echo "Port audit summary: open=${open_count} closed=${closed_count} unknown=${unknown_count}"
  if (( closed_count > 0 )); then
    echo "Recommended next steps:"
    echo "  - If services should be running: bash install.sh --restart"
    echo "  - If restart fails: bash install.sh --repair"
    echo "  - For deeper context: bash install.sh --debug-bundle"
  fi
  return 0
}

redact_env_line() {
  local line="$1"
  local key=""
  if [[ -z "$line" || "$line" == \#* || "$line" != *=* ]]; then
    printf '%s\n' "$line"
    return
  fi
  key="${line%%=*}"
  if [[ "$key" =~ (PASSWORD|SECRET|TOKEN|DATABASE_URL|SESSION|COOKIE|KEY) ]]; then
    printf '%s=%s\n' "$key" "<redacted>"
    return
  fi
  printf '%s\n' "$line"
}

debug_bundle_report() {
  local ts=""
  local out_file=""
  local cmd=""
  local compose_base=""
  local runtime_ssh_port="$SSH_PORT"
  local runtime_web_port="$WEB_PORT"
  local runtime_irc_port="$IRC_PORT"
  local runtime_mailin_port="$MAILIN_PORT"
  local doctor_output=""
  local env_line=""
  local env_web=""
  local env_ssh=""
  local env_irc=""
  local env_mailin=""

  resolve_runtime_context_if_available
  if [[ -n "${ENV_FILE:-}" && -f "$ENV_FILE" ]]; then
    env_ssh="$(read_env_value "WOLFBBS_SSH_PORT" "$ENV_FILE")"
    env_web="$(read_env_value "WOLFBBS_WEB_PORT" "$ENV_FILE")"
    env_irc="$(read_env_value "WOLFBBS_IRC_PORT" "$ENV_FILE")"
    env_mailin="$(read_env_value "WOLFBBS_MAILIN_PORT" "$ENV_FILE")"
    if [[ -n "$env_ssh" ]]; then
      runtime_ssh_port="$env_ssh"
    fi
    if [[ -n "$env_web" ]]; then
      runtime_web_port="$env_web"
    fi
    if [[ -n "$env_irc" ]]; then
      runtime_irc_port="$env_irc"
    fi
    if [[ -n "$env_mailin" ]]; then
      runtime_mailin_port="$env_mailin"
    fi
  fi

  ts="$(date -u +'%Y%m%dT%H%M%SZ')"
  out_file="${PREFIX}/WOLFBBS_DIAGNOSTICS_${ts}.txt"
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would write diagnostics bundle to ${out_file}"
    return 0
  fi
  if ! mkdir -p "$PREFIX" >/dev/null 2>&1; then
    out_file="${SCRIPT_PATH}/WOLFBBS_DIAGNOSTICS_${ts}.txt"
  fi

  {
    echo "WolfBBS Diagnostics Bundle"
    echo "Generated: $(date -u +'%Y-%m-%dT%H:%M:%SZ')"
    echo "OS: ${OS}"
    echo "Distro: ${DISTRO}"
    echo "Arch: ${ARCH}"
    echo "Package manager: ${PKG_MGR:-none}"
    echo "Prefix: ${PREFIX}"
    echo "Work dir: ${WORK_DIR}"
    echo "Compose file: ${compose_file:-not detected}"
    echo "Env file: ${ENV_FILE:-not found}"
    echo "Log file: ${LOG_FILE}"

    echo
    echo "== Tool Versions =="
    echo "bash: $(bash --version 2>/dev/null | head -n 1 || echo unavailable)"
    if command -v curl >/dev/null 2>&1; then
      echo "curl: $(curl --version 2>/dev/null | head -n 1 || echo available)"
    else
      echo "curl: missing"
    fi
    if command -v docker >/dev/null 2>&1; then
      echo "docker: $(docker --version 2>/dev/null || echo available)"
      echo "docker daemon: $(docker info --format '{{.ServerVersion}}' 2>/dev/null || echo unreachable)"
    else
      echo "docker: missing"
    fi
    cmd="$(compose_cmd)"
    if [[ -n "$cmd" ]]; then
      echo "compose: ${cmd}"
    else
      echo "compose: missing"
    fi
    if command -v git >/dev/null 2>&1; then
      echo "git: $(git --version 2>/dev/null || echo available)"
    else
      echo "git: missing"
    fi

    echo
    echo "== Host Capacity =="
    df -h "$PREFIX" 2>/dev/null || df -h . 2>/dev/null || true
    if command -v docker >/dev/null 2>&1; then
      docker system df 2>/dev/null || true
    fi

    echo
    echo "== Runtime Probes =="
    if command -v curl >/dev/null 2>&1; then
      if curl -fsS "http://127.0.0.1:${runtime_web_port}/healthz" >/dev/null 2>&1; then
        echo "PASS web healthz http://127.0.0.1:${runtime_web_port}/healthz"
      else
        echo "WARN web healthz unreachable http://127.0.0.1:${runtime_web_port}/healthz"
      fi
      if curl -fsS "http://127.0.0.1:${runtime_web_port}/readyz" >/dev/null 2>&1; then
        echo "PASS web readyz http://127.0.0.1:${runtime_web_port}/readyz"
      else
        echo "WARN web readyz unreachable http://127.0.0.1:${runtime_web_port}/readyz"
      fi
    else
      echo "WARN curl unavailable; HTTP probes skipped"
    fi
    if command -v nc >/dev/null 2>&1; then
      if nc -z 127.0.0.1 "$runtime_ssh_port" >/dev/null 2>&1; then
        echo "PASS ssh port ${runtime_ssh_port} reachable"
      else
        echo "WARN ssh port ${runtime_ssh_port} unreachable"
      fi
      if nc -z 127.0.0.1 "$runtime_irc_port" >/dev/null 2>&1; then
        echo "PASS irc port ${runtime_irc_port} reachable"
      else
        echo "WARN irc port ${runtime_irc_port} unreachable"
      fi
      if nc -z 127.0.0.1 "$runtime_mailin_port" >/dev/null 2>&1; then
        echo "PASS mail ingest port ${runtime_mailin_port} reachable"
      else
        echo "WARN mail ingest port ${runtime_mailin_port} unreachable"
      fi
    else
      echo "WARN nc unavailable; TCP probes skipped"
    fi

    echo
    echo "== Port Audit =="
    port_audit 2>&1

    echo
    echo "== Redacted Env Snapshot =="
    if [[ -n "${ENV_FILE:-}" && -f "$ENV_FILE" ]]; then
      while IFS= read -r env_line || [[ -n "$env_line" ]]; do
        redact_env_line "$env_line"
      done <"$ENV_FILE"
    else
      echo "No env file detected."
    fi

    echo
    echo "== Doctor Report =="
    if doctor_output="$(doctor_report 2>&1)"; then
      printf '%s\n' "$doctor_output"
    else
      printf '%s\n' "$doctor_output"
    fi

    if [[ -n "$cmd" && -n "${compose_file:-}" && -f "${compose_file:-}" ]]; then
      compose_base="$cmd -f \"$compose_file\""
      if [[ -n "${ENV_FILE:-}" && -f "$ENV_FILE" ]]; then
        compose_base="${compose_base} --env-file \"$ENV_FILE\""
      fi
      echo
      echo "== Compose Status =="
      eval "$compose_base ps" 2>&1 || true
      echo
      echo "== Compose Logs (tail 200) =="
      eval "$compose_base logs --tail=200" 2>&1 || true
    else
      echo
      echo "== Compose Status =="
      echo "Compose context not detected."
    fi
  } >"$out_file"

  echo "Debug bundle written: ${out_file}"
  echo "Share this file when opening an install/runtime support issue."
}

parse_args() {
  while (($# > 0)); do
    case "$1" in
      --prefix)
        require_value "$1" "${2:-}"
        PREFIX="$2"
        shift 2
        ;;
      --with-docker)
        WITH_DOCKER=true
        shift
        ;;
      --dry-run)
        DRY_RUN=true
        shift
        ;;
      --yes|--non-interactive)
        NON_INTERACTIVE=true
        shift
        ;;
      --install-brew)
        INSTALL_BREW=true
        shift
        ;;
      --force)
        FORCE=true
        shift
        ;;
      --bbs-name)
        require_value "$1" "${2:-}"
        BBS_NAME="$2"
        USED_INSTALLER_CONFIG_FLAGS=true
        shift 2
        ;;
      --hostname)
        require_value "$1" "${2:-}"
        BBS_HOSTNAME="$2"
        USED_INSTALLER_CONFIG_FLAGS=true
        shift 2
        ;;
      --setup-profile)
        require_value "$1" "${2:-}"
        if ! is_valid_setup_profile "$2"; then
          echo "Invalid setup profile: $2 (expected basic|critical|expert)"
          exit 1
        fi
        SETUP_PROFILE="$(normalize_setup_profile "$2")"
        USED_INSTALLER_CONFIG_FLAGS=true
        shift 2
        ;;
      --ssh-port)
        require_value "$1" "${2:-}"
        SSH_PORT="$2"
        shift 2
        ;;
      --web-port)
        require_value "$1" "${2:-}"
        WEB_PORT="$2"
        shift 2
        ;;
      --irc-port)
        require_value "$1" "${2:-}"
        IRC_PORT="$2"
        shift 2
        ;;
      --irc-tls-port)
        require_value "$1" "${2:-}"
        IRC_TLS_PORT="$2"
        shift 2
        ;;
      --mailin-port)
        require_value "$1" "${2:-}"
        MAILIN_PORT="$2"
        shift 2
        ;;
      --uninstall)
        UNINSTALL=true
        shift
        ;;
      --clean-uninstall)
        UNINSTALL=true
        PURGE=true
        CLEAN_UNINSTALL=true
        shift
        ;;
      --purge)
        PURGE=true
        shift
        ;;
      --upgrade)
        UPGRADE=true
        shift
        ;;
      --rapid-upgrade)
        RAPID_UPGRADE=true
        shift
        ;;
      --status)
        STATUS=true
        shift
        ;;
      --doctor)
        DOCTOR=true
        shift
        ;;
      --debug-bundle)
        DEBUG_BUNDLE=true
        shift
        ;;
      --port-audit)
        PORT_AUDIT=true
        shift
        ;;
      --start)
        START=true
        shift
        ;;
      --stop)
        STOP=true
        shift
        ;;
      --restart)
        RESTART=true
        shift
        ;;
      --logs)
        LOGS=true
        shift
        ;;
      --repair)
        REPAIR=true
        shift
        ;;
      --deps-only)
        DEPS_ONLY=true
        shift
        ;;
      --repo|--repo-url)
        require_value "$1" "${2:-}"
        REPO_URL="$2"
        shift 2
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

validate_action_flags() {
  local action_count=0
  local all_actions=(
    "$UNINSTALL"
    "$UPGRADE"
    "$RAPID_UPGRADE"
    "$STATUS"
    "$DOCTOR"
    "$DEBUG_BUNDLE"
    "$PORT_AUDIT"
    "$START"
    "$STOP"
    "$RESTART"
    "$LOGS"
    "$REPAIR"
    "$DEPS_ONLY"
  )
  local flag=""
  for flag in "${all_actions[@]}"; do
    if [[ "$flag" == "true" ]]; then
      action_count=$((action_count + 1))
    fi
  done
  if (( action_count > 1 )); then
    echo "Only one action mode can be used at a time:"
    echo "  --doctor | --debug-bundle | --port-audit | --status | --start | --stop | --restart | --logs | --repair | --upgrade | --rapid-upgrade | --uninstall | --clean-uninstall | --deps-only"
    exit 1
  fi
}

main() {
  local args_count=$#
  parse_args "$@"
  resolve_repo_url
  detect_platform
  detect_arch
  set_default_prefix
  detect_package_manager
  check_macos_prereqs
  init_log_file
  print_splash
  show_interactive_action_menu "$args_count"
  show_interactive_install_plan "$args_count"
  validate_action_flags

  if [[ "$USED_INSTALLER_CONFIG_FLAGS" == "true" ]]; then
    echo "Note: install-time identity/profile flags are supported for automation."
    echo "Recommended path is to configure WolfBBS in the UI at /admin/setup and /admin/config."
    echo
  fi

  if [[ "$DOCTOR" == "true" ]]; then
    doctor_report
    exit $?
  fi
  if [[ "$PORT_AUDIT" == "true" ]]; then
    port_audit
    exit $?
  fi
  if [[ "$DEBUG_BUNDLE" == "true" ]]; then
    debug_bundle_report
    exit $?
  fi
  if [[ "$OS" == "unknown" || ( "$OS" == "linux" && "$PKG_MGR" == "" ) || ( "$OS" == "linux" && "$DISTRO" == "unknown" ) ]]; then
    echo "Unsupported operating system. Supported: Linux (Debian/Ubuntu, Fedora/RHEL/CentOS, Arch) and macOS."
    echo "Required commands for manual install: curl, tar, openssl, sed, awk, grep, docker, docker compose."
    exit 1
  fi

  log "Detected platform: os=${OS} distro=${DISTRO} like=${ID_LIKE:-n/a} arch=${ARCH} pkg=${PKG_MGR:-none}"

  ensure_rootless_permissions

  ensure_base_prereqs

  if [[ "$DEPS_ONLY" == "true" ]]; then
    ensure_docker
    echo "Dependencies are installed and docker runtime is ready."
    exit 0
  fi

  if [[ "$DRY_RUN" == "false" ]]; then
    ensure_rootless_permissions
    if [[ "$STATUS" != "true" && "$START" != "true" && "$STOP" != "true" && "$RESTART" != "true" && "$LOGS" != "true" && "$UNINSTALL" != "true" && "$UPGRADE" != "true" && "$RAPID_UPGRADE" != "true" && "$REPAIR" != "true" && "$DEBUG_BUNDLE" != "true" && "$PORT_AUDIT" != "true" ]]; then
      check_space "$PREFIX"
    else
      log "Skipping disk-space check for non-install action mode."
    fi
  else
    log "DRY-RUN: skip disk-space check"
  fi

  compose_file="$(find_compose_file || true)"
  if [[ -z "$compose_file" ]]; then
    adopt_installed_compose_if_present || true
  fi
  if [[ -n "$compose_file" ]]; then
    WORK_DIR="$(dirname "$compose_file")"
  fi

  if [[ "$UNINSTALL" != "true" && "$UPGRADE" != "true" && "$RAPID_UPGRADE" != "true" && "$STATUS" != "true" && "$START" != "true" && "$STOP" != "true" && "$RESTART" != "true" && "$LOGS" != "true" && "$REPAIR" != "true" && "$DEPS_ONLY" != "true" ]]; then
    if [[ -z "$compose_file" ]]; then
      ensure_compose_file
    fi
  fi

  if [[ -n "$compose_file" ]]; then
    WORK_DIR="$(dirname "$compose_file")"
  fi

  if [[ "$STATUS" == "true" ]]; then
    if ! ensure_action_compose_context; then
      exit 1
    fi
    ENV_FILE="$(resolve_env_file || true)"
    if [[ -z "$ENV_FILE" ]]; then
      echo "No env file found for status check."
      exit 1
    fi
    ensure_runtime_env_defaults "$ENV_FILE"
    status_view
    exit 0
  fi

  if [[ "$START" == "true" || "$STOP" == "true" || "$RESTART" == "true" || "$LOGS" == "true" || "$REPAIR" == "true" ]]; then
    if ! ensure_action_compose_context; then
      exit 1
    fi
    ensure_docker
    ENV_FILE="$(resolve_env_file || true)"
    if [[ -n "$ENV_FILE" ]]; then
      ensure_runtime_env_defaults "$ENV_FILE"
    fi
    if [[ "$START" == "true" || "$RESTART" == "true" || "$REPAIR" == "true" ]]; then
      if [[ -z "$ENV_FILE" ]]; then
        write_env_file
        ENV_FILE="${PREFIX}/.env"
      fi
    elif [[ -z "$ENV_FILE" ]]; then
      if [[ "$DRY_RUN" == "true" ]]; then
        ENV_FILE="${PREFIX}/.env"
        log "DRY-RUN: no env file found; would use ${ENV_FILE} (run --repair first)"
      else
        echo "No env file found in ${PREFIX} or ${WORK_DIR}."
        echo "Run: bash install.sh --repair"
        exit 1
      fi
    fi
    if [[ "$REPAIR" == "true" ]]; then
      write_env_file
      ENV_FILE="${PREFIX}/.env"
    fi
    if [[ "$START" == "true" ]]; then
      docker_compose_start
      verify_install
      status_view
      exit 0
    fi
    if [[ "$STOP" == "true" ]]; then
      docker_compose_stop
      status_view
      exit 0
    fi
    if [[ "$RESTART" == "true" ]]; then
      docker_compose_restart
      verify_install
      status_view
      exit 0
    fi
    if [[ "$LOGS" == "true" ]]; then
      docker_compose_logs
      exit 0
    fi
    if [[ "$REPAIR" == "true" ]]; then
      write_env_file
      seed_admin_check
      docker_compose_up
      verify_install
      echo "Repair complete."
      status_view
      exit 0
    fi
  fi

  if [[ "$UNINSTALL" == "true" ]]; then
    local can_use_compose=false
    resolve_runtime_context_if_available
    ENV_FILE="$(resolve_env_file || true)"
    if [[ -n "$ENV_FILE" ]]; then
      ensure_runtime_env_defaults "$ENV_FILE"
    fi
    if compose_context_available && [[ -n "$(compose_cmd)" ]]; then
      can_use_compose=true
    fi
    if [[ "$DRY_RUN" == "false" ]]; then
      if ! confirm "Stop WolfBBS services from ${PREFIX}?"; then
        echo "Aborted."
        exit 0
      fi
      if [[ "$can_use_compose" == "true" ]]; then
        docker_compose_down || true
      else
        log "Compose context unavailable; using compose-less Docker cleanup fallback."
        docker_cleanup_without_compose false
      fi
      if [[ "$PURGE" == "true" ]] || confirm "Remove volumes and all installed data? (run with --purge to auto-confirm)"; then
        if [[ "$can_use_compose" == "true" ]]; then
          docker_compose_down_purge || true
        else
          docker_cleanup_without_compose true
        fi
      fi
      if [[ "$CLEAN_UNINSTALL" == "true" ]]; then
        if ! remove_install_prefix; then
          exit 1
        fi
      else
        echo "Preserved install directory: ${PREFIX}"
        echo "Use --clean-uninstall to remove the install directory too."
      fi
    else
      log "DRY-RUN: would stop/remove services in ${PREFIX}"
    fi
    exit 0
  fi

  if [[ "$UPGRADE" == "true" ]]; then
    if [[ ! -d "$PREFIX" ]]; then
      echo "No existing install in ${PREFIX}"
      exit 1
    fi
    if ! ensure_action_compose_context; then
      exit 1
    fi
    ensure_docker
    ENV_FILE="$(resolve_env_file || true)"
    if [[ -z "$ENV_FILE" ]]; then
      write_env_file
      ENV_FILE="${PREFIX}/.env"
    fi
    ensure_runtime_env_defaults "$ENV_FILE"
    docker_compose_pull_restart
    verify_install
    echo "Upgrade complete."
    exit 0
  fi

  if [[ "$RAPID_UPGRADE" == "true" ]]; then
    if [[ ! -d "$PREFIX" ]]; then
      echo "No existing install in ${PREFIX}"
      exit 1
    fi
    if ! ensure_action_compose_context; then
      exit 1
    fi
    ensure_docker
    ENV_FILE="$(resolve_env_file || true)"
    if [[ -z "$ENV_FILE" ]]; then
      write_env_file
      ENV_FILE="${PREFIX}/.env"
    fi
    ensure_runtime_env_defaults "$ENV_FILE"
    docker_compose_rapid_upgrade
    verify_install
    echo "Rapid upgrade complete."
    exit 0
  fi

  if [[ "$WITH_DOCKER" != "true" ]]; then
    echo "Native install mode is not supported yet."
    echo "Use --with-docker (default) for now."
    exit 1
  fi

  ensure_docker
  if [[ "$DRY_RUN" == "false" ]]; then
    require_cmd docker
    require_cmd nc
  fi
  ensure_compose_file
  require_ports_free "$SSH_PORT" "$WEB_PORT" "$IRC_PORT" "$MAILIN_PORT"
  init_install_dir

  if [[ "$DRY_RUN" == "true" ]]; then
    if [[ -z "${compose_file:-}" ]]; then
      compose_file="$(find_compose_file || true)"
    fi
    if [[ -z "$compose_file" ]]; then
      compose_file="$(managed_checkout_dir)/docker-compose.yml"
      WORK_DIR="$(managed_checkout_dir)"
      log "DRY-RUN: would use compose file ${compose_file}"
    fi
  else
    compose_file="$(find_compose_file || true)"
    if [[ -z "$compose_file" ]]; then
      compose_file="$(find_installed_compose_file || true)"
      if [[ -z "$compose_file" ]]; then
        echo "No compose file found after setup."
        exit 1
      fi
    fi
  fi

  write_env_file
  seed_admin_check
  docker_compose_up
  verify_install

  print_install_summary
}

main "$@"
