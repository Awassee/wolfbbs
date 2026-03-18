#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL_SRC="${ROOT_DIR}/install.sh"
TMP_WORK=""

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
if [[ "${1:-}" == "--version" ]]; then
  echo "curl 8.fake"
fi
exit 0
EOF
  chmod +x "${bin_dir}/curl"

  cat > "${bin_dir}/nc" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
exit 0
EOF
  chmod +x "${bin_dir}/nc"
}

run_installer_case() {
  local installer_dir="$1"
  local fake_bin="$2"
  local docker_log="$3"
  local out_file="$4"
  shift 4
  : > "$docker_log"
  (
    cd "$installer_dir"
    PATH="${fake_bin}:${PATH}" \
    HOME="${installer_dir}/home" \
    WOLFBBS_FAKE_DOCKER_LOG="$docker_log" \
    bash ./install.sh "$@"
  ) >"$out_file" 2>&1
}

main() {
  TMP_WORK="$(mktemp -d "${TMPDIR:-/tmp}/wolfbbs-installer-regressions.XXXXXX")"
  trap 'rm -rf "$TMP_WORK"' EXIT

  local installer_dir="${TMP_WORK}/installer"
  local fake_bin="${TMP_WORK}/fake-bin"
  local docker_log="${TMP_WORK}/docker.log"
  local out_file="${TMP_WORK}/run.out"
  mkdir -p "$installer_dir"
  cp "$INSTALL_SRC" "${installer_dir}/install.sh"
  chmod +x "${installer_dir}/install.sh"
  build_fake_runtime "$fake_bin"

  local prefix_uninstall="${TMP_WORK}/WolfBBSCase/InstallA"
  mkdir -p "${prefix_uninstall}/app"
  cat > "${prefix_uninstall}/app/docker-compose.yml" <<'EOF'
services:
  web:
    image: wolfbbs-web:latest
EOF

  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$out_file" \
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

  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$out_file" \
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

  run_installer_case "$installer_dir" "$fake_bin" "$docker_log" "$out_file" \
    --yes --uninstall --purge --prefix "$prefix_rapid"
  assert_contains "$out_file" "down -v --remove-orphans" \
    "uninstall --purge should include volume removal"
  assert_not_contains "$out_file" "--env-file \"\"" \
    "uninstall --purge should not emit blank env-file argument"

  echo "PASS: installer regression harness"
}

main "$@"
