#!/usr/bin/env bash
set -euo pipefail

DEFAULT_PREFIX="/opt/wolfbbs"
DEFAULT_SSH_PORT=2222
DEFAULT_WEB_PORT=8080
DEFAULT_IRC_PORT=6667
DEFAULT_IRC_TLS_PORT=6697
DEFAULT_MAILIN_PORT=8091

PREFIX="$DEFAULT_PREFIX"
WITH_DOCKER=true
DRY_RUN=false
NON_INTERACTIVE=false
FORCE=false
SSH_PORT="$DEFAULT_SSH_PORT"
WEB_PORT="$DEFAULT_WEB_PORT"
IRC_PORT="$DEFAULT_IRC_PORT"
IRC_TLS_PORT="$DEFAULT_IRC_TLS_PORT"
MAILIN_PORT="$DEFAULT_MAILIN_PORT"
UNINSTALL=false
UPGRADE=false
STATUS=false
REPO_URL="${WOLFBBS_REPO_URL:-}"

SCRIPT_PATH="$(cd "$(dirname "$0")" && pwd)"
LOG_FILE="${SCRIPT_PATH}/install.log"
WORK_DIR="${SCRIPT_PATH}"
PURGE=false
ENV_FILE=""

usage() {
  cat <<'USAGE'
WolfBBS installer

Usage:
  bash install.sh [options]

Options:
  --prefix <dir>            install directory (default: /opt/wolfbbs)
  --with-docker             use docker mode (default)
  --dry-run                 print actions without applying
  --yes, --non-interactive  run non-interactively
  --force                   overwrite existing generated config
  --ssh-port <port>         SSH BBS port (default: 2222)
  --web-port <port>         web port (default: 8080)
  --irc-port <port>         IRC port (default: 6667)
  --irc-tls-port <port>     IRC TLS port suggestion (default: 6697)
  --mailin-port <port>      inbound mail webhook port (default: 8091)
  --uninstall               stop/remove services
  --upgrade                 pull/restart services in existing install
  --status                  show service status and endpoints
  --purge                   remove docker volumes/instance on uninstall
  --repo-url <url>          git URL to clone if installer is run standalone
  -h, --help                show this help
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
  printf "No local docker-compose file found. Enter repository URL to clone: "
  read -r input
  if [[ -n "$input" ]]; then
    REPO_URL="$input"
  fi
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
      if lsof -iTCP -sTCP:LISTEN -P -n | grep -qE "[:.]$port[[:space:]]"; then
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
  local free_gb
  free_gb=$(df -Pm "$dir" | awk 'NR==2 {print $4}')
  if [[ -z "${free_gb}" ]]; then
    return
  fi
  if (( free_gb < 2048 )); then
    echo "Low disk in $dir: ${free_gb}MB free. At least 2GB is recommended."
    if [[ "$NON_INTERACTIVE" == "true" ]]; then
      exit 1
    fi
    if ! confirm "Continue anyway?"; then
      exit 1
    fi
  fi
}

compose_cmd() {
  if command -v docker >/dev/null 2>&1; then
    if docker compose version >/dev/null 2>&1; then
      echo "docker compose"
      return
    fi
  fi
  if command -v docker-compose >/dev/null 2>&1; then
    echo "docker-compose"
    return
  fi
  echo ""
}

ensure_docker_linux() {
  if command -v docker >/dev/null 2>&1; then
    return
  fi
  echo "Docker is not installed."
  if [[ "$NON_INTERACTIVE" == "true" ]]; then
    echo "Install Docker manually or pass a mode not implemented in this script."
    exit 1
  fi
  if is_linux; then
    case "$PKG_MGR" in
      apt)
        if confirm "Install Docker Engine and compose plugin now using APT?"; then
          run "sudo apt-get update"
          run "sudo apt-get install -y ca-certificates curl gnupg lsb-release"
          run "sudo mkdir -p /etc/apt/keyrings"
          run "curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg"
          run "echo \"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \\$(lsb_release -cs) stable\" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null"
          run "sudo apt-get update"
          run "sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin"
          return
        fi
        ;;
      dnf|yum)
        if confirm "Install Docker using ${PKG_MGR}?"; then
          if [[ "$PKG_MGR" == "dnf" ]]; then
            run "sudo dnf -y install dnf-plugins-core"
            run "sudo dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo"
          else
            run "sudo yum -y install yum-utils"
            run "sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo"
          fi
          run "sudo ${PKG_MGR} -y install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin"
          return
        fi
        ;;
      pacman)
        if confirm "Install Docker now using pacman?"; then
          run "sudo pacman -Sy --noconfirm docker docker-compose"
          return
        fi
        ;;
    esac
  elif is_macos; then
    if confirm "Install Docker Desktop via Homebrew now?"; then
      if command -v brew >/dev/null 2>&1; then
        run "brew install --cask docker"
        echo "Start Docker Desktop before continuing."
        echo "Press Enter when Docker is running."
        if [[ "$NON_INTERACTIVE" != "true" ]]; then
          read -r
        fi
        return
      fi
    fi
  fi
  echo "Please install Docker manually and rerun the installer."
  echo "Linux: https://docs.docker.com/engine/install"
  echo "macOS: https://docs.docker.com/desktop/"
  exit 1
}

ensure_docker() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: skip Docker install checks"
    return 0
  fi
  if command -v docker >/dev/null 2>&1; then
    return
  fi
  ensure_docker_linux
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
      return
    fi
    if [[ -d "$PREFIX/.git" ]]; then
      run "git -C '$PREFIX' pull --ff-only"
    else
      run "git clone '$REPO_URL' '$PREFIX'"
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
  echo "Run from repository root, or pass --repo-url."
  exit 1
}

write_env_file() {
  ENV_FILE="${PREFIX}/.env"
  local db_pass db_user db_name db_seed
  db_user="wolfbbs"
  db_name="wolfbbs"
  db_pass="$(random_secret)"
  db_seed="$(random_secret)"

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
  run "cd '$WORK_DIR' && $cmd -f '$(printf '%q' "$compose_file")' --env-file '$ENV_FILE' up -d --build"
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
  run "cd '$WORK_DIR' && $cmd -f '$(printf '%q' "$compose_file")' --env-file '$ENV_FILE' pull"
  run "cd '$WORK_DIR' && $cmd -f '$(printf '%q' "$compose_file")' --env-file '$ENV_FILE' up -d --build"
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
  run "cd '$WORK_DIR' && $cmd -f '$(printf '%q' "$compose_file")' --env-file '$ENV_FILE' down"
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
  run "cd '$WORK_DIR' && $cmd -f '$(printf '%q' "$compose_file")' --env-file '$ENV_FILE' down -v --remove-orphans"
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

seed_admin_check() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: skipping admin user seeding check"
    return
  fi
  if ! grep -q "admin" "$compose_file" 2>/dev/null; then
    log "No explicit admin seed override in compose. Web app may seed defaults."
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
  . "$ENV_FILE"
  echo "SSH: ssh ${HOSTNAME:-localhost} -p ${WOLFBBS_SSH_PORT:-$SSH_PORT}"
  echo "Web: http://localhost:${WOLFBBS_WEB_PORT:-$WEB_PORT}/admin"
  echo "Chat: http://localhost:${WOLFBBS_WEB_PORT:-$WEB_PORT}/chat"
  echo "IRC: localhost:${WOLFBBS_IRC_PORT:-$IRC_PORT} (TLS: localhost:${WOLFBBS_IRC_TLS_PORT:-$IRC_TLS_PORT})"
  echo "Mail Ingest: http://localhost:${WOLFBBS_MAILIN_PORT:-$MAILIN_PORT}/ingest"
  local cmd
  cmd="$(compose_cmd)"
  if [[ -n "$cmd" ]]; then
    echo "Compose status:"
    eval "$cmd -f '$compose_file' --env-file '$ENV_FILE' ps" || true
  fi
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
      --repo-url)
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

main() {
  parse_args "$@"
  detect_platform
  detect_package_manager
  if [[ "$OS" == "unknown" || ( "$OS" == "linux" && "$PKG_MGR" == "" ) || ( "$OS" == "linux" && "$DISTRO" == "unknown" ) ]]; then
    echo "Unsupported operating system. Supported: Linux (Debian/Ubuntu, Fedora/RHEL/CentOS, Arch) and macOS."
    exit 1
  fi

  if [[ -f "${PREFIX}/.env" ]]; then
    LOG_FILE="${PREFIX}/install.log"
  else
    LOG_FILE="${SCRIPT_PATH}/install.log"
  fi
  touch "$LOG_FILE"

  ensure_rootless_permissions

  require_cmd sed
  require_cmd awk
  require_cmd grep
  require_cmd openssl

  if [[ -z "$compose_file" && "$STATUS" != "true" ]]; then
    require_cmd git
  fi

  if [[ "$DRY_RUN" == "false" ]]; then
    ensure_rootless_permissions
    check_space "$PREFIX"
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

  compose_file="$(find_compose_file || true)"
  if [[ -z "$compose_file" ]]; then
    if [[ ! -f "$PREFIX/docker-compose.yml" ]]; then
      echo "No compose file found after setup."
      exit 1
    fi
    compose_file="$PREFIX/docker-compose.yml"
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
}

main "$@"
