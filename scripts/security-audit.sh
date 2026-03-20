#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

SECURITY_GO_TOOLCHAIN="${WOLFBBS_SECURITY_GOTOOLCHAIN:-go1.26.1}"

echo "[security 1/4] go module integrity"
GOTOOLCHAIN="$SECURITY_GO_TOOLCHAIN" go mod verify

echo "[security 2/4] vulnerability audit"
if command -v govulncheck >/dev/null 2>&1; then
  GOTOOLCHAIN="$SECURITY_GO_TOOLCHAIN" govulncheck ./...
else
  GOTOOLCHAIN="$SECURITY_GO_TOOLCHAIN" go run golang.org/x/vuln/cmd/govulncheck@latest ./...
fi

echo "[security 3/4] npm production dependency audit"
(cd e2e/web && npm audit --omit=dev)

echo "[security 4/4] secrets + privacy scan"
rg -n --hidden \
  --glob '!.git/**' \
  --glob '!dist/**' \
  --glob '!e2e/web/node_modules/**' \
  --glob '!e2e/web/test-results/**' \
  --glob '!scripts/security-audit.sh' \
  -e '/Users/seanheiney' \
  -e 'seanheiney@' \
  -e 'ghp_[A-Za-z0-9]{20,}' \
  -e 'github_pat_[A-Za-z0-9_]+' \
  -e 'sk-[A-Za-z0-9]{20,}' \
  -e 'AKIA[0-9A-Z]{16}' \
  -e 'BEGIN [A-Z ]+ PRIVATE KEY' \
  -e 'BEGIN OPENSSH PRIVATE KEY' \
  . >/tmp/wolfbbs-security-scan.txt || true

if [[ -s /tmp/wolfbbs-security-scan.txt ]]; then
  cat /tmp/wolfbbs-security-scan.txt
  rm -f /tmp/wolfbbs-security-scan.txt
  echo "Security scan found potential secrets or private identifiers." >&2
  exit 1
fi
rm -f /tmp/wolfbbs-security-scan.txt

echo "PASS security-audit"
