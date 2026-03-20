#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

pass() {
  printf 'PASS %s\n' "$1"
}

fail() {
  printf 'FAIL %s\n' "$1" >&2
  exit 1
}

[[ -f LICENSE ]] || fail 'root LICENSE file missing'
grep -Fq 'MIT License' LICENSE || fail 'LICENSE does not contain MIT License text'
pass 'root MIT license present'

[[ -f CONTRIBUTING.md ]] || fail 'CONTRIBUTING.md missing'
grep -Eiq 'MIT License|released under the MIT' CONTRIBUTING.md || fail 'CONTRIBUTING.md does not describe MIT contribution terms'
pass 'contributing terms reference MIT'

[[ -f docs/OPEN_SOURCE.md ]] || fail 'docs/OPEN_SOURCE.md missing'
grep -Eiq 'MIT' docs/OPEN_SOURCE.md || fail 'docs/OPEN_SOURCE.md does not mention MIT'
pass 'open source guide present'

grep -Eiq 'License: MIT|## License' README.md || fail 'README does not expose license information'
pass 'README exposes license information'

grep -Fq 'org.opencontainers.image.licenses="MIT"' Dockerfile || fail 'Dockerfile missing OCI license label'
pass 'container metadata includes MIT label'

grep -Fq 'LICENSE' scripts/package-dist.sh || fail 'package-dist.sh does not bundle LICENSE'
grep -Fq 'CONTRIBUTING.md' scripts/package-dist.sh || fail 'package-dist.sh does not bundle CONTRIBUTING.md'
grep -Fq 'docs/OPEN_SOURCE.md' scripts/package-dist.sh || fail 'package-dist.sh does not bundle docs/OPEN_SOURCE.md'
pass 'distribution packaging includes OSS notices'

pass 'license audit complete'
