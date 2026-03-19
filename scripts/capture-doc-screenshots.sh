#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
E2E_DIR="$ROOT_DIR/e2e/web"
OUT_DIR="$ROOT_DIR/docs/assets/screenshots"

mkdir -p "$OUT_DIR"
rm -f "$OUT_DIR"/*.png

cd "$E2E_DIR"

if [[ ! -d node_modules ]]; then
  npm ci
fi

npx playwright install chromium >/dev/null
if [[ -z "${WOLFBBS_E2E_LOCAL_WEB_PORT:-}" ]]; then
  WOLFBBS_E2E_LOCAL_WEB_PORT="$(node -e '
const net = require("net");
const server = net.createServer();
server.listen(0, "127.0.0.1", () => {
  process.stdout.write(String(server.address().port));
  server.close();
});
')"
fi

WOLFBBS_E2E_LOCAL_WEB_PORT="$WOLFBBS_E2E_LOCAL_WEB_PORT" \
WOLFBBS_E2E_SQLITE_PATH="${WOLFBBS_E2E_SQLITE_PATH:-/tmp/wolfbbs-doc-screenshots.db}" \
WOLFBBS_CAPTURE_SCREENSHOTS=1 \
npx playwright test tests/docs_screenshots.spec.js

if python3 -c "import PIL" >/dev/null 2>&1; then
  "$ROOT_DIR/scripts/generate-readme-gif.py"
else
  echo "WARN: Pillow not installed, skipping getting-started.gif generation."
fi

echo "Screenshots written to: $OUT_DIR"
