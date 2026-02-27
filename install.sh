#!/usr/bin/env bash
set -euo pipefail

DEFAULT_PREFIX_LINUX="/opt/wolfbbs"
DEFAULT_PREFIX_MACOS="${HOME}/.local/share/wolfbbs"
DEFAULT_SSH_PORT=2222
DEFAULT_WEB_PORT=8080
DEFAULT_IRC_PORT=6667
DEFAULT_IRC_TLS_PORT=6697
DEFAULT_MAILIN_PORT=8091
DEFAULT_REPO_URL="https://github.com/seanheiney/New-project.git"

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
UPGRADE=false
STATUS=false
DOCTOR=false
START=false
STOP=false
RESTART=false
LOGS=false
REPAIR=false
DEPS_ONLY=false
REPO_URL="${WOLFBBS_REPO_URL:-${WOLFBBS_GH:-}}"
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
bootstrap_handle=""

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

usage() {
  cat <<'USAGE'
WolfBBS installer
Supports Linux (apt/dnf/yum/pacman) and macOS (Docker Desktop or Colima).

Usage:
  bash install.sh [options]

Options:
  --prefix <dir>            install directory (default: Linux=/opt/wolfbbs, macOS=$HOME/.local/share/wolfbbs)
  --with-docker             use docker mode (default)
  --dry-run                 print actions without applying
  --yes, --non-interactive  run non-interactively
  --install-brew            on macOS, install Homebrew when missing (requires explicit flag)
  --force                   overwrite existing generated config
  --ssh-port <port>         SSH BBS port (default: 2222)
  --web-port <port>         web port (default: 8080)
  --irc-port <port>         IRC port (default: 6667)
  --irc-tls-port <port>     IRC TLS port suggestion (default: 6697)
  --mailin-port <port>      inbound mail webhook port (default: 8091)
  --uninstall               stop/remove services
  --upgrade                 pull/restart services in existing install
  --status                  show service status and endpoints
  --doctor                  run non-mutating preflight + install health diagnostics
  --start                   start existing WolfBBS services
  --stop                    stop existing WolfBBS services
  --restart                 restart existing WolfBBS services
  --logs                    show recent service logs (tail)
  --repair                  self-heal install: ensure deps/env, rebuild + verify stack
  --deps-only               install/check prerequisites and docker runtime, then exit
  --purge                   remove docker volumes/instance on uninstall
  --repo <owner/repo|url>   GitHub slug or git URL to clone if installer is run standalone
  --repo-url <url>          alias of --repo
  -h, --help                show this help

Environment shortcuts:
  WOLFBBS_GH=<owner/repo>         e.g. seanheiney/New-project
  WOLFBBS_REPO_URL=<git-url>      e.g. https://github.com/seanheiney/New-project.git
  WOLFBBS_REPO_URL defaults to:   https://github.com/seanheiney/New-project.git
USAGE
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
  printf "No local docker-compose file found. Enter repository (owner/repo or git URL): "
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

resolve_repo_url() {
  if [[ -n "$REPO_URL" ]]; then
    REPO_URL="$(normalize_repo_input "$REPO_URL")"
    return
  fi

  # If installer is executed from a git checkout, prefer that remote.
  if [[ -d "${WORK_DIR}/.git" ]] && command -v git >/dev/null 2>&1; then
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
  if [[ -f "${WORK_DIR}/docker-compose.yml" ]]; then
    echo "${WORK_DIR}/docker-compose.yml"
    return 0
  fi
  if [[ -f "${WORK_DIR}/compose.yml" ]]; then
    echo "${WORK_DIR}/compose.yml"
    return 0
  fi
  return 1
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
  local required=(curl git sed awk grep openssl)
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
  if [[ -n "$compose_file" ]]; then
    return
  fi

  if [[ -z "$REPO_URL" ]]; then
    resolve_repo_url
    prompt_repo_url
  fi

  if [[ -n "$REPO_URL" ]]; then
    if [[ -d "$PREFIX" && -n "$(ls -A "$PREFIX" 2>/dev/null)" && "$FORCE" != "true" ]]; then
      if [[ -d "$PREFIX/.git" ]]; then
        echo "Target directory is a git checkout and will be updated: $PREFIX"
      elif [[ "$FORCE" == "true" ]]; then
        run "rm -rf '$PREFIX'"
        run "mkdir -p '$PREFIX'"
      else
        echo "Target directory exists and is not empty: $PREFIX"
        echo "Use --force to replace it, or choose a different --prefix."
        exit 1
      fi
    fi
    log "No local compose file found. Cloning repository from ${REPO_URL}."
    init_install_dir
    if [[ "$DRY_RUN" == "true" ]]; then
      WORK_DIR="$PREFIX"
      compose_file="${PREFIX}/docker-compose.yml"
      log "DRY-RUN: would clone repository to ${PREFIX} and use ${compose_file}"
      return
    fi
    if [[ -d "$PREFIX/.git" ]]; then
      run_retry 3 3 "git -C '$PREFIX' pull --ff-only"
    else
      run_retry 3 3 "git clone '$REPO_URL' '$PREFIX'"
    fi
    WORK_DIR="$PREFIX"
    compose_file="$(find_compose_file || true)"
    if [[ -n "$compose_file" ]]; then
      PREFIX="$(dirname "$compose_file")"
      return
    fi
    echo "compose file still not found after clone."
    exit 1
  fi

  echo "Could not find docker-compose.yml or compose.yml."
  echo "Run from repository root, or pass --repo/--repo-url."
  echo "Examples:"
  echo "  bash install.sh --with-docker --repo seanheiney/New-project --yes"
  echo "  WOLFBBS_GH=seanheiney/New-project bash install.sh --with-docker --yes"
  exit 1
}

write_env_file() {
  ENV_FILE="${PREFIX}/.env"
  local db_pass db_user db_name db_seed bootstrap_admin_handle bootstrap_admin_password
  db_user="wolfbbs"
  db_name="wolfbbs"
  db_pass="$(random_secret)"
  db_seed="$(random_secret)"
  bootstrap_admin_handle="sysop"
  bootstrap_admin_password="$(random_secret)"

  if [[ -f "$ENV_FILE" && "$FORCE" != "true" ]]; then
    log "Using existing env file: $ENV_FILE"
    return
  fi
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would write ${ENV_FILE}"
    return
  fi
  if [[ -f "$ENV_FILE" && "$FORCE" == "true" ]]; then
    if ! confirm "Overwrite existing env file at ${ENV_FILE}?"; then
      log "Keeping existing env file."
      return
    fi
  fi
  cat > "$ENV_FILE" <<EOF
WOLFBBS_DATABASE_URL=postgres://$db_user:$db_pass@postgres:5432/$db_name?sslmode=disable
POSTGRES_USER=$db_user
POSTGRES_PASSWORD=$db_pass
POSTGRES_DB=$db_name
WOLFBBS_DB_CONNECT_RETRIES=15
WOLFBBS_DB_CONNECT_DELAY_MS=500
WOLFBBS_SESSION_SECRET=$db_seed
WOLFBBS_INBOUND_TOKEN=$(random_secret)
WOLFBBS_OFFLINE_DIR=/app/.wolfbbs/offline
WOLFBBS_SSH_PORT=${SSH_PORT}
WOLFBBS_WEB_PORT=${WEB_PORT}
WOLFBBS_IRC_PORT=${IRC_PORT}
WOLFBBS_IRC_TLS_PORT=${IRC_TLS_PORT}
WOLFBBS_MAILIN_PORT=${MAILIN_PORT}
WOLFBBS_READ_ONLY=false
WOLFBBS_BOOTSTRAP_ADMIN_HANDLE=${bootstrap_admin_handle}
WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD=${bootstrap_admin_password}
WOLFBBS_BOOTSTRAP_MODERATOR_HANDLE=
WOLFBBS_BOOTSTRAP_MODERATOR_PASSWORD=
WOLFBBS_BOOTSTRAP_USER_HANDLE=
WOLFBBS_BOOTSTRAP_USER_PASSWORD=
EOF
  chmod 600 "$ENV_FILE"
  log "Wrote ${ENV_FILE}"
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
  run_retry 3 5 "cd '$WORK_DIR' && $cmd -f \"$compose_file\" --env-file \"$ENV_FILE\" up -d --build"
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
  run_retry 3 5 "cd '$WORK_DIR' && $cmd -f \"$compose_file\" --env-file \"$ENV_FILE\" pull"
  run_retry 3 5 "cd '$WORK_DIR' && $cmd -f \"$compose_file\" --env-file \"$ENV_FILE\" up -d --build"
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
  run "cd '$WORK_DIR' && $cmd -f \"$compose_file\" --env-file \"$ENV_FILE\" down"
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
  run "cd '$WORK_DIR' && $cmd -f \"$compose_file\" --env-file \"$ENV_FILE\" down -v --remove-orphans"
}

docker_compose_status() {
  local cmd
  cmd="$(compose_cmd)"
  if [[ -z "$cmd" ]]; then
    echo "Docker Compose not found."
    return 1
  fi
  run "$cmd -f '$compose_file' --env-file '$ENV_FILE' ps"
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
  run "cd '$WORK_DIR' && $cmd -f \"$compose_file\" --env-file \"$ENV_FILE\" up -d"
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
  run "cd '$WORK_DIR' && $cmd -f \"$compose_file\" --env-file \"$ENV_FILE\" stop"
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
  run "cd '$WORK_DIR' && $cmd -f \"$compose_file\" --env-file \"$ENV_FILE\" restart"
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
  run "cd '$WORK_DIR' && $cmd -f \"$compose_file\" --env-file \"$ENV_FILE\" logs --tail=200"
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
  handle="$(grep '^WOLFBBS_BOOTSTRAP_ADMIN_HANDLE=' "$ENV_FILE" | head -n1 | cut -d= -f2-)"
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
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: skip runtime checks."
    return
  fi
  if ! command -v curl >/dev/null 2>&1; then
    echo "curl missing; cannot verify services."
    return 1
  fi
  if ! curl -fsS "http://127.0.0.1:${WEB_PORT}/healthz" >/dev/null; then
    echo "web service health check failed"
    return 1
  fi
  if ! curl -fsS "http://127.0.0.1:${WEB_PORT}/readyz" >/dev/null; then
    echo "web service readiness check failed"
    return 1
  fi
  wait_for_port 127.0.0.1 "$SSH_PORT" "SSH BBS"
  wait_for_port 127.0.0.1 "$IRC_PORT" "IRC"
  wait_for_port 127.0.0.1 "$MAILIN_PORT" "Mail Ingest"
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
  echo "SSH: ssh ${HOSTNAME:-localhost} -p ${WOLFBBS_SSH_PORT:-$SSH_PORT}"
  echo "Web: http://localhost:${WOLFBBS_WEB_PORT:-$WEB_PORT}/admin"
  echo "Chat: http://localhost:${WOLFBBS_WEB_PORT:-$WEB_PORT}/chat"
  echo "IRC: localhost:${WOLFBBS_IRC_PORT:-$IRC_PORT} (TLS: localhost:${WOLFBBS_IRC_TLS_PORT:-$IRC_TLS_PORT})"
  echo "Mail Ingest: http://localhost:${WOLFBBS_MAILIN_PORT:-$MAILIN_PORT}/ingest"
  if [[ -n "${WOLFBBS_BOOTSTRAP_ADMIN_HANDLE:-}" ]]; then
    echo "Bootstrap sysop handle: ${WOLFBBS_BOOTSTRAP_ADMIN_HANDLE} (password stored in ${ENV_FILE})"
  fi
  local cmd
  cmd="$(compose_cmd)"
  if [[ -n "$cmd" ]]; then
    echo "Compose status:"
    eval "$cmd -f '$compose_file' --env-file '$ENV_FILE' ps" || true
  fi
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

  echo "WolfBBS doctor report"
  echo "  os=${OS} distro=${DISTRO} arch=${ARCH} pkg=${PKG_MGR:-none}"

  if [[ "$OS" == "unknown" ]]; then
    doctor_fail "unsupported operating system"
    failures=$((failures + 1))
  else
    doctor_ok "supported OS detected"
  fi

  local required=(curl git openssl sed awk grep)
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
  else
    doctor_ok "required base commands are present"
  fi

  if command -v docker >/dev/null 2>&1; then
    doctor_ok "docker CLI found"
  else
    doctor_fail "docker CLI missing"
    failures=$((failures + 1))
  fi

  compose_cmd_value="$(compose_cmd)"
  if [[ -n "$compose_cmd_value" ]]; then
    doctor_ok "docker compose command detected: ${compose_cmd_value}"
  else
    doctor_fail "docker compose command not found"
    failures=$((failures + 1))
  fi

  if command -v docker >/dev/null 2>&1; then
    if docker info >/dev/null 2>&1; then
      doctor_ok "docker daemon reachable"
    else
      doctor_warn "docker daemon not reachable for current user"
    fi
  fi

  local_compose="$(find_compose_file || true)"
  if [[ -n "$local_compose" ]]; then
    doctor_ok "compose file in working dir: ${local_compose}"
  else
    doctor_warn "no compose file in working dir (installer can clone via --repo)"
  fi

  install_compose=""
  if [[ -f "${PREFIX}/docker-compose.yml" ]]; then
    install_compose="${PREFIX}/docker-compose.yml"
  elif [[ -f "${PREFIX}/compose.yml" ]]; then
    install_compose="${PREFIX}/compose.yml"
  fi
  if [[ -n "$install_compose" ]]; then
    doctor_ok "compose file in install prefix: ${install_compose}"
  else
    doctor_warn "no compose file in install prefix ${PREFIX}"
  fi

  install_env="${PREFIX}/.env"
  if [[ -f "$install_env" ]]; then
    doctor_ok "env file found: ${install_env}"
    local mode
    mode="$(stat -f '%Lp' "$install_env" 2>/dev/null || stat -c '%a' "$install_env" 2>/dev/null || true)"
    if [[ "$mode" == "600" ]]; then
      doctor_ok ".env permissions are 600"
    elif [[ -n "$mode" ]]; then
      doctor_warn ".env permissions are ${mode} (recommended 600)"
    fi
  else
    doctor_warn "no env file in install prefix (expected before first install)"
  fi

  if command -v nc >/dev/null 2>&1; then
    if nc -z 127.0.0.1 "$SSH_PORT" >/dev/null 2>&1; then
      doctor_ok "ssh port ${SSH_PORT} is reachable"
    else
      doctor_warn "ssh port ${SSH_PORT} is not reachable"
    fi
    if nc -z 127.0.0.1 "$IRC_PORT" >/dev/null 2>&1; then
      doctor_ok "irc port ${IRC_PORT} is reachable"
    else
      doctor_warn "irc port ${IRC_PORT} is not reachable"
    fi
  else
    doctor_warn "netcat (nc) not found; skipping port checks"
  fi

  if command -v curl >/dev/null 2>&1; then
    if curl -fsS "http://127.0.0.1:${WEB_PORT}/healthz" >/dev/null 2>&1; then
      doctor_ok "web health endpoint is reachable on port ${WEB_PORT}"
    else
      doctor_warn "web health endpoint not reachable on port ${WEB_PORT}"
    fi
  fi

  if (( failures > 0 )); then
    echo "Doctor found ${failures} blocking issue(s)."
    return 1
  fi
  echo "Doctor completed with no blocking issues."
  return 0
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
      --purge)
        PURGE=true
        shift
        ;;
      --upgrade)
        UPGRADE=true
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
    "$STATUS"
    "$DOCTOR"
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
    echo "  --doctor | --status | --start | --stop | --restart | --logs | --repair | --upgrade | --uninstall | --deps-only"
    exit 1
  fi
}

main() {
  parse_args "$@"
  validate_action_flags
  resolve_repo_url
  detect_platform
  detect_arch
  set_default_prefix
  detect_package_manager
  check_macos_prereqs
  init_log_file

  if [[ "$DOCTOR" == "true" ]]; then
    doctor_report
    exit $?
  fi
  if [[ "$OS" == "unknown" || ( "$OS" == "linux" && "$PKG_MGR" == "" ) || ( "$OS" == "linux" && "$DISTRO" == "unknown" ) ]]; then
    echo "Unsupported operating system. Supported: Linux (Debian/Ubuntu, Fedora/RHEL/CentOS, Arch) and macOS."
    echo "Required commands for manual install: curl, git, openssl, sed, awk, grep, docker, docker compose."
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
    if [[ "$STATUS" != "true" && "$START" != "true" && "$STOP" != "true" && "$RESTART" != "true" && "$LOGS" != "true" && "$UNINSTALL" != "true" ]]; then
      check_space "$PREFIX"
    else
      log "Skipping disk-space check for non-install action mode."
    fi
  else
    log "DRY-RUN: skip disk-space check"
  fi

  compose_file="$(find_compose_file || true)"
  if [[ -z "$compose_file" && "$STATUS" != "true" ]]; then
    ensure_compose_file
  elif [[ -z "$compose_file" && "$STATUS" == "true" ]]; then
    if [[ -d "$PREFIX" ]]; then
      WORK_DIR="$PREFIX"
      compose_file="$(find_compose_file || true)"
    fi
  fi

  if [[ -n "$compose_file" ]]; then
    # if found in a separate path, keep prefix aligned
    if [[ "$(dirname "$compose_file")" != "$WORK_DIR" ]]; then
      PREFIX="$(dirname "$compose_file")"
    fi
    WORK_DIR="$(dirname "$compose_file")"
  fi

  if [[ "$STATUS" == "true" ]]; then
    ENV_FILE="$(resolve_env_file || true)"
    if [[ -z "$ENV_FILE" ]]; then
      echo "No env file found for status check."
      exit 1
    fi
    compose_file="$(find_compose_file || true)"
    status_view
    exit 0
  fi

  if [[ "$START" == "true" || "$STOP" == "true" || "$RESTART" == "true" || "$LOGS" == "true" || "$REPAIR" == "true" ]]; then
    ensure_docker
    ENV_FILE="$(resolve_env_file || true)"
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
    if [[ "$DRY_RUN" == "false" ]]; then
      if ! confirm "Stop WolfBBS services from ${PREFIX}?"; then
        echo "Aborted."
        exit 0
      fi
      docker_compose_down
      if [[ "$PURGE" == "true" ]] || confirm "Remove volumes and all installed data? (run with --purge to auto-confirm)"; then
        docker_compose_down_purge
      fi
      if confirm "Remove install directory ${PREFIX}?"; then
        rm -rf "$PREFIX"
        echo "Removed ${PREFIX}."
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
    ensure_docker
    require_ports_free "$SSH_PORT" "$WEB_PORT" "$IRC_PORT" "$MAILIN_PORT"
    write_env_file
    ENV_FILE="${PREFIX}/.env"
    docker_compose_pull_restart
    verify_install
    echo "Upgrade complete."
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
      compose_file="${PREFIX}/docker-compose.yml"
      WORK_DIR="$PREFIX"
      log "DRY-RUN: would use compose file ${compose_file}"
    fi
  else
    compose_file="$(find_compose_file || true)"
    if [[ -z "$compose_file" ]]; then
      if [[ ! -f "$PREFIX/docker-compose.yml" ]]; then
        echo "No compose file found after setup."
        exit 1
      fi
      compose_file="$PREFIX/docker-compose.yml"
    fi
  fi

  write_env_file
  seed_admin_check
  docker_compose_up
  verify_install

  echo "WolfBBS installation complete."
  echo "SSH: ssh localhost -p ${SSH_PORT}"
  echo "Web: http://localhost:${WEB_PORT}/admin"
  echo "Web Chat: http://localhost:${WEB_PORT}/chat"
  echo "IRC: localhost:${IRC_PORT} (TLS: ${IRC_TLS_PORT})"
  echo "Mail Ingest: http://localhost:${MAILIN_PORT}/ingest"
  if [[ -f "$ENV_FILE" ]]; then
    bootstrap_handle="$(grep '^WOLFBBS_BOOTSTRAP_ADMIN_HANDLE=' "$ENV_FILE" | head -n1 | cut -d= -f2-)"
    if [[ -n "$bootstrap_handle" ]]; then
      echo "Bootstrap sysop handle: ${bootstrap_handle}"
      echo "Bootstrap sysop password is stored in ${ENV_FILE}"
    fi
  fi
}

main "$@"
