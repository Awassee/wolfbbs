#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL_SRC="${ROOT_DIR}/install.sh"
TMP_WORK=""
TMP_PARENT="${ROOT_DIR}/.tmp"

fail() {
  echo "FAIL: $*"
  exit 1
}

assert_contains() {
  local file="$1"
  local pattern="$2"
  local context="$3"
  if ! grep -Fq -- "$pattern" "$file"; then
    fail "${context} (missing: ${pattern})"
  fi
}

assert_not_contains() {
  local file="$1"
  local pattern="$2"
  local context="$3"
  if grep -Fq -- "$pattern" "$file"; then
    fail "${context} (unexpected: ${pattern})"
  fi
}

build_fake_runtime() {
  local bin_dir="$1"
  mkdir -p "$bin_dir"

  cat > "${bin_dir}/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${WOLFBBS_FAKE_DOCKER_LOG:?missing fake docker log path}"
printf '%s\n' "$*" >> "$log_file"
if [[ "${1:-}" == "info" ]]; then
  exit 0
fi
if [[ "${1:-}" == "compose" ]]; then
  if [[ "${2:-}" == "version" ]]; then
    echo "Docker Compose version v2.fake"
  fi
  exit 0
fi
if [[ "${1:-}" == "--version" || "${1:-}" == "version" ]]; then
  echo "Docker version v0.fake"
  exit 0
fi
exit 0
EOF
  chmod +x "${bin_dir}/docker"

  cat > "${bin_dir}/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ -n "${WOLFBBS_FAKE_CURL_LOG:-}" ]]; then
  printf '%s\n' "$*" >> "${WOLFBBS_FAKE_CURL_LOG}"
fi
if [[ "${1:-}" == "--version" ]]; then
  echo "curl 8.fake"
fi
exit 0
EOF
  chmod +x "${bin_dir}/curl"

  cat > "${bin_dir}/git" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "--version" ]]; then
  echo "git: command not found" >&2
fi
exit 127
EOF
  chmod +x "${bin_dir}/git"

  cat > "${bin_dir}/nc" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ -n "${WOLFBBS_FAKE_NC_LOG:-}" ]]; then
  printf '%s\n' "$*" >> "${WOLFBBS_FAKE_NC_LOG}"
fi
exit 0
EOF
  chmod +x "${bin_dir}/nc"
}

run_installer_case() {
  local installer_dir="$1"
  local fake_bin="$2"
  local docker_log="$3"
  local curl_log="$4"
  local nc_log="$5"
  local out_file="$6"
  shift 6
  : > "$docker_log"
  : > "$curl_log"
  : > "$nc_log"
  (
    cd "$installer_dir"
    PATH="${fake_bin}:${PATH}" \
    HOME="${installer_dir}/home" \
    WOLFBBS_SKIP_SPACE_CHECK=1 \
    WOLFBBS_FAKE_DOCKER_LOG="$docker_log" \
    WOLFBBS_FAKE_CURL_LOG="$curl_log" \
    WOLFBBS_FAKE_NC_LOG="$nc_log" \
    bash ./install.sh "$@"
  ) >"$out_file" 2>&1
}

main() {
  mkdir -p "$TMP_PARENT"
  TMP_WORK="$(mktemp -d "${TMP_PARENT}/wolfbbs-installer-regressions.XXXXXX")"
  trap 'rm -rf "$TMP_WORK"' EXIT

  local installer_dir="${TMP_WORK}/installer"
  local fake_bin="${TMP_WORK}/fake-bin"
  local docker_log="${TMP_WORK}/docker.log"
  local curl_log="${TMP_WORK}/curl.log"
  local nc_log="${TMP_WORK}/nc.log"
  local out_file="${TMP_WORK}/run.out"
  mkdir -p "$installer_dir"
  cp "$INSTALL_SRC" "${installer_dir}/install.sh"
  chmod +x "${installer_dir}/install.sh"
  cat > "${installer_dir}/docker-compose.yml" <<'EOF'
services:
  web:
    image: wolfbbs-web:latest
EOF
  build_fake_runtime "$fake_bin"

  local prefix_socket="${TMP_WORK}/WolfBBSCase/SocketInstall"
  (
    cd "$installer_dir"
    PATH="${fake_bin}:${PATH}" \
    HOME="${installer_dir}/home" \
    DOCKER_HOST="unix://${installer_dir}/home/.colima/docker.sock" \
    WOLFBBS_SKIP_SPACE_CHECK=1 \
    WOLFBBS_FAKE_DOCKER_LOG="$docker_log" \
    bash ./install.sh --yes --prefix "$prefix_socket"
  ) >"$out_file" 2>&1
  assert_contains "${prefix_socket}/.env" "WOLFBBS_DOCKER_SOCKET='/var/run/docker.sock'" \
    "installer should normalize macOS docker socket mounts to /var/run/docker.sock"
  assert_not_contains "${prefix_socket}/.env" ".colima/docker.sock" \
    "installer should not persist host-side colima socket paths into runtime env"
  rm -f "${installer_dir}/docker-compose.yml"

  local prefix_uninstall="${TMP_WORK}/WolfBBSCase/InstallA"
  mkdir -p "${prefix_uninstall}/app"
  cat > "${prefix_uninstall}/app/docker-compose.yml" <<'EOF'
services:
  web:
    image: wolfbbs-web:latest
EOF

  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$curl_log" "$nc_log" "$out_file" \
    --yes --uninstall --prefix "$prefix_uninstall"
  assert_contains "$out_file" "RUN: cd '${prefix_uninstall}/app' && docker compose -f \"${prefix_uninstall}/app/docker-compose.yml\" down --remove-orphans" \
    "uninstall should target installed compose path"
  assert_not_contains "$out_file" "--env-file \"\"" \
    "uninstall should not emit blank env-file argument"
  assert_not_contains "$docker_log" "--env-file" \
    "uninstall docker command should not include env-file when .env is absent"
  local prefix_uninstall_lower
  prefix_uninstall_lower="$(printf '%s' "${prefix_uninstall}/app" | tr '[:upper:]' '[:lower:]')"
  if [[ "$prefix_uninstall_lower" != "${prefix_uninstall}/app" ]]; then
    assert_not_contains "$out_file" "$prefix_uninstall_lower" \
      "installer should preserve path casing in compose working directory"
  fi

  local prefix_fresh="${TMP_WORK}/WolfBBSCase/FreshInstall"
  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$curl_log" "$nc_log" "$out_file" \
    --dry-run --yes --with-docker --repo Awassee/wolfbbs --prefix "$prefix_fresh"
  assert_contains "$out_file" "DRY-RUN: would download repository archive to" \
    "fresh install should support archive download path when git is unavailable"
  assert_contains "$out_file" "${prefix_fresh}/app/docker-compose.yml" \
    "fresh install dry-run should resolve managed checkout compose path"

  local prefix_rapid="${TMP_WORK}/WolfBBSCase/InstallB"
  mkdir -p "${prefix_rapid}/app"
  cat > "${prefix_rapid}/app/docker-compose.yml" <<'EOF'
services:
  web:
    image: wolfbbs-web:latest
EOF
  cat > "${prefix_rapid}/.env" <<'EOF'
WOLFBBS_WEB_PORT=8080
WOLFBBS_SSH_PORT=2222
WOLFBBS_IRC_PORT=6667
WOLFBBS_MAILIN_PORT=8091
EOF

  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$curl_log" "$nc_log" "$out_file" \
    --yes --rapid-upgrade --prefix "$prefix_rapid"
  assert_contains "$out_file" "--env-file \"${prefix_rapid}/.env\" up -d --build --remove-orphans" \
    "rapid-upgrade should include populated env-file and remove-orphans"
  assert_contains "$docker_log" "compose -f ${prefix_rapid}/app/docker-compose.yml --env-file ${prefix_rapid}/.env up -d --build --remove-orphans" \
    "rapid-upgrade docker command should include env-file and rebuild flags"
  local prefix_rapid_lower
  prefix_rapid_lower="$(printf '%s' "${prefix_rapid}/app" | tr '[:upper:]' '[:lower:]')"
  if [[ "$prefix_rapid_lower" != "${prefix_rapid}/app" ]]; then
    assert_not_contains "$out_file" "$prefix_rapid_lower" \
      "rapid-upgrade should preserve path casing in compose working directory"
  fi

  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$curl_log" "$nc_log" "$out_file" \
    --yes --upgrade --prefix "$prefix_rapid"
  assert_contains "$out_file" "pull" \
    "upgrade should pull published images before restart"
  assert_contains "$out_file" "--env-file \"${prefix_rapid}/.env\" up -d --build --remove-orphans" \
    "upgrade should rebuild/restart with env-file context"
  assert_contains "$docker_log" "compose -f ${prefix_rapid}/app/docker-compose.yml --env-file ${prefix_rapid}/.env pull" \
    "upgrade docker command should include pull"

  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$curl_log" "$nc_log" "$out_file" \
    --yes --start --prefix "$prefix_rapid"
  assert_contains "$docker_log" "compose -f ${prefix_rapid}/app/docker-compose.yml --env-file ${prefix_rapid}/.env up -d" \
    "start should bring services up with env-file context"

  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$curl_log" "$nc_log" "$out_file" \
    --yes --stop --prefix "$prefix_rapid"
  assert_contains "$docker_log" "compose -f ${prefix_rapid}/app/docker-compose.yml --env-file ${prefix_rapid}/.env stop" \
    "stop should stop compose services"

  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$curl_log" "$nc_log" "$out_file" \
    --yes --restart --prefix "$prefix_rapid"
  assert_contains "$docker_log" "compose -f ${prefix_rapid}/app/docker-compose.yml --env-file ${prefix_rapid}/.env restart" \
    "restart should restart compose services"

  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$curl_log" "$nc_log" "$out_file" \
    --yes --uninstall --purge --prefix "$prefix_rapid"
  assert_contains "$out_file" "down -v --remove-orphans" \
    "uninstall --purge should include volume removal"
  assert_not_contains "$out_file" "--env-file \"\"" \
    "uninstall --purge should not emit blank env-file argument"

  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$curl_log" "$nc_log" "$out_file" \
    --yes --status --prefix "$prefix_rapid"
  assert_contains "$out_file" "WolfBBS install status: ${prefix_rapid}" \
    "status should render install summary"
  assert_contains "$docker_log" "compose -f ${prefix_rapid}/app/docker-compose.yml --env-file ${prefix_rapid}/.env ps" \
    "status should inspect compose ps with resolved env-file"

  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$curl_log" "$nc_log" "$out_file" \
    --yes --repair --prefix "$prefix_rapid"
  assert_contains "$out_file" "Repair complete." \
    "repair should report success"
  assert_contains "$docker_log" "compose -f ${prefix_rapid}/app/docker-compose.yml --env-file ${prefix_rapid}/.env up -d --build" \
    "repair should rebuild and start services with env-file"

  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$curl_log" "$nc_log" "$out_file" \
    --yes --doctor --prefix "$prefix_rapid"
  assert_contains "$out_file" "WolfBBS doctor report" \
    "doctor should render health diagnostics"
  assert_contains "$out_file" "PASS doctor: docker daemon reachable" \
    "doctor should probe docker daemon health"

  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$curl_log" "$nc_log" "$out_file" \
    --yes --port-audit --prefix "$prefix_rapid"
  assert_contains "$out_file" "WolfBBS port audit" \
    "port audit should render listener summary"
  assert_contains "$out_file" "Port audit summary:" \
    "port audit should include totals"

  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$curl_log" "$nc_log" "$out_file" \
    --yes --debug-bundle --prefix "$prefix_rapid"
  assert_contains "$out_file" "Debug bundle written:" \
    "debug bundle should emit output path"
  if ! find "$prefix_rapid" -maxdepth 1 -type f -name 'WOLFBBS_DIAGNOSTICS_*.txt' | grep -q .; then
    fail "debug bundle should create a diagnostics report"
  fi

  local prefix_verify="${TMP_WORK}/WolfBBSCase/InstallVerify"
  mkdir -p "${prefix_verify}/app"
  cat > "${prefix_verify}/app/docker-compose.yml" <<'EOF'
services:
  web:
    image: wolfbbs-web:latest
EOF
  cat > "${prefix_verify}/.env" <<'EOF'
WOLFBBS_WEB_PORT=18080
WOLFBBS_SSH_PORT=12222
WOLFBBS_IRC_PORT=16667
WOLFBBS_MAILIN_PORT=18091
EOF
  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$curl_log" "$nc_log" "$out_file" \
    --yes --upgrade --prefix "$prefix_verify"
  assert_contains "$curl_log" "-fsS http://127.0.0.1:18080/healthz" \
    "upgrade verification should probe the env-file web health port"
  assert_contains "$curl_log" "-fsS http://127.0.0.1:18080/readyz" \
    "upgrade verification should probe the env-file web ready port"
  assert_contains "$nc_log" "-z 127.0.0.1 12222" \
    "upgrade verification should probe the env-file ssh port"
  assert_contains "$nc_log" "-z 127.0.0.1 16667" \
    "upgrade verification should probe the env-file irc port"
  assert_contains "$nc_log" "-z 127.0.0.1 18091" \
    "upgrade verification should probe the env-file mail ingest port"

  local prefix_clean="${TMP_WORK}/WolfBBSCase/InstallC"
  mkdir -p "${prefix_clean}/app"
  cat > "${prefix_clean}/app/docker-compose.yml" <<'EOF'
services:
  web:
    image: wolfbbs-web:latest
EOF
  cat > "${prefix_clean}/.env" <<'EOF'
WOLFBBS_WEB_PORT=8080
WOLFBBS_SSH_PORT=2222
WOLFBBS_IRC_PORT=6667
WOLFBBS_MAILIN_PORT=8091
EOF
  local prefix_clean_resolved
  prefix_clean_resolved="$(cd "${prefix_clean}" && pwd)"
  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$curl_log" "$nc_log" "$out_file" \
    --yes --uninstall --clean-uninstall --prefix "$prefix_clean"
  assert_contains "$out_file" "Removed ${prefix_clean_resolved}." \
    "clean uninstall should remove install directory"
  if [[ -d "$prefix_clean" ]]; then
    fail "clean uninstall should delete prefix directory"
  fi

  echo "PASS: installer regression harness"
}

main "$@"
