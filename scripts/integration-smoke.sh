#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker not found; skipping compose smoke"
  exit 0
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose not found; skipping compose smoke"
  exit 0
fi

docker compose up -d --build
cleanup() {
  docker compose down -v --remove-orphans || true
}
trap cleanup EXIT

echo "waiting for web health..."
for _ in {1..40}; do
  if curl -fsS "http://127.0.0.1:8080/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
curl -fsS "http://127.0.0.1:8080/healthz" >/dev/null

echo "checking socket listeners..."
nc -z 127.0.0.1 2222
nc -z 127.0.0.1 6667
nc -z 127.0.0.1 8091

echo "compose smoke passed"
