#!/usr/bin/env bash
set -euo pipefail

INSTALLER_URL="${WOLFBBS_BOOTSTRAP_INSTALLER_URL:-https://raw.githubusercontent.com/seanheiney/wolfbbs-public/main/install.sh}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/wolfbbs-bootstrap.XXXXXX")"
INSTALLER_PATH="${TMP_DIR}/install.sh"

cleanup() {
  rm -rf "$TMP_DIR"
}

trap cleanup EXIT

if ! command -v curl >/dev/null 2>&1; then
  echo "WolfBBS bootstrap requires curl."
  exit 1
fi

if ! command -v bash >/dev/null 2>&1; then
  echo "WolfBBS bootstrap requires bash."
  exit 1
fi

echo "WolfBBS bootstrap: downloading installer..."
curl -fsSL "$INSTALLER_URL" -o "$INSTALLER_PATH"
chmod +x "$INSTALLER_PATH"

exec bash "$INSTALLER_PATH" "$@"
