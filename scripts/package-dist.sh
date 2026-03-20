#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

OUT_DIR="$ROOT_DIR/dist"
VERSION="${WOLFBBS_VERSION:-}"
PLATFORMS=()
CLEAN_OUT=false

usage() {
  cat <<'USAGE'
WolfBBS distribution packager

Usage:
  scripts/package-dist.sh [options]

Options:
  --out <dir>             output directory (default: dist/)
  --version <value>       release version label
  --platform <os/arch>    build a platform bundle; may be repeated
  --clean                 wipe output directory before packaging
  -h, --help              show this help

Examples:
  scripts/package-dist.sh
  scripts/package-dist.sh --platform linux/amd64 --platform linux/arm64
  scripts/package-dist.sh --clean --version v1.0.0
  scripts/package-dist.sh --version v0.9.0-rc1 --out ./dist
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for --out" >&2
        exit 2
      fi
      OUT_DIR="$2"
      shift
      ;;
    --version)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for --version" >&2
        exit 2
      fi
      VERSION="$2"
      shift
      ;;
    --platform)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for --platform" >&2
        exit 2
      fi
      PLATFORMS+=("$2")
      shift
      ;;
    --clean)
      CLEAN_OUT=true
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

if [[ "$CLEAN_OUT" == "true" ]]; then
  rm -rf "$OUT_DIR"
fi

if [[ -z "$VERSION" ]]; then
  VERSION="$(git describe --tags --always --dirty 2>/dev/null || git rev-parse --short HEAD)"
fi

if [[ ${#PLATFORMS[@]} -eq 0 ]]; then
  goos="$(go env GOOS)"
  goarch="$(go env GOARCH)"
  PLATFORMS+=("${goos}/${goarch}")
fi

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file"
    return
  fi
  shasum -a 256 "$file"
}

build_bin() {
  local os="$1"
  local arch="$2"
  local output="$3"
  local pkg="$4"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="-s -w" -o "$output" "$pkg"
}

doc_list=(
  "docs/README.md"
  "docs/START_HERE.md"
  "docs/QUICKSTART.md"
  "docs/INSTALL.md"
  "docs/OPEN_SOURCE.md"
  "docs/OPERATIONS.md"
  "docs/PRODUCT_GUIDE.md"
  "docs/DATASHEET.md"
  "docs/SHOWCASE.md"
  "docs/ACCEPTANCE_SPEC.md"
  "docs/feature-reference.md"
  "docs/irc-compat.md"
  "docs/doors.md"
  "docs/chat.md"
  "docs/admin.md"
  "docs/LAUNCH_CHECKLIST.md"
  "docs/TROUBLESHOOTING.md"
  "docs/OPERATOR_PLAYBOOK.md"
  "docs/FIRST_30_MINUTES.md"
  "docs/RUNNING_A_COMMUNITY.md"
)
if [[ -f "docs/releases/$VERSION.md" ]]; then
  doc_list+=("docs/releases/$VERSION.md")
fi
if [[ -f "docs/releases/README.md" ]]; then
  doc_list+=("docs/releases/README.md")
fi

release_dir="$OUT_DIR/$VERSION"
rm -rf "$release_dir"
mkdir -p "$release_dir"

checksums_file="$release_dir/checksums.txt"
manifest_file="$release_dir/release-manifest.txt"
: >"$checksums_file"
: >"$manifest_file"

targets=(
  "wolfbbs ./cmd/wolfbbs"
  "wolfbbs-web ./cmd/wolfbbs-web"
  "wolfbbs-irc ./cmd/wolfbbs-irc"
  "wolfbbs-mailin ./cmd/wolfbbs-mailin"
  "wolfbbs-trivia ./cmd/doors-trivia"
  "oputil ./cmd/oputil"
)

for platform in "${PLATFORMS[@]}"; do
  if [[ "$platform" != */* ]]; then
    echo "Invalid --platform value: $platform (expected os/arch)" >&2
    exit 2
  fi
  os="${platform%/*}"
  arch="${platform#*/}"
  stage_dir="$(mktemp -d)"
  bundle_name="wolfbbs_${VERSION}_${os}_${arch}"
  bundle_root="$stage_dir/$bundle_name"
  mkdir -p "$bundle_root/bin" "$bundle_root/docs" "$bundle_root/scripts"

  echo "Packaging $bundle_name"
  for target in "${targets[@]}"; do
    bin_name="${target%% *}"
    pkg="${target#* }"
    build_bin "$os" "$arch" "$bundle_root/bin/$bin_name" "$pkg"
  done

  cp README.md LICENSE CONTRIBUTING.md docker-compose.yml .env.example install.sh bootstrap.sh "$bundle_root/"
  for doc in "${doc_list[@]}"; do
    rel="${doc#docs/}"
    mkdir -p "$bundle_root/docs/$(dirname "$rel")"
    cp "$doc" "$bundle_root/docs/$rel"
  done
  cp scripts/verify.sh scripts/build.sh "$bundle_root/scripts/"
  chmod +x "$bundle_root/install.sh" "$bundle_root/bootstrap.sh" "$bundle_root/scripts/verify.sh" "$bundle_root/scripts/build.sh"
  chmod +x "$bundle_root/bin/"*

  cat >"$bundle_root/RELEASE_NOTES.txt" <<EOF
WolfBBS distribution bundle
Version: $VERSION
Platform: $os/$arch

Contents:
- bin/: server, web, irc, mailin, trivia, and oputil binaries
- install.sh: installer and upgrade entrypoint
- bootstrap.sh: one-line downloader/bootstrap entrypoint
- LICENSE + CONTRIBUTING.md: open-source license and contribution terms
- docker-compose.yml + .env.example: default stack runtime
- docs/: documentation hub, install/start guides, open-source licensing guide, datasheet, showcase, launch checklist, operator playbook, acceptance, feature reference, and release notes
- scripts/: verify and build helpers

Quick start:
1. Review docs/START_HERE.md, docs/PRODUCT_GUIDE.md, and docs/OPERATOR_PLAYBOOK.md
2. Copy .env.example to a local .env if needed
3. Run ./install.sh or launch binaries from bin/
EOF

  tarball="$release_dir/${bundle_name}.tar.gz"
  tar -C "$stage_dir" -czf "$tarball" "$bundle_name"
  (
    cd "$release_dir"
    sha256_file "$(basename "$tarball")"
  ) >>"$checksums_file"
  {
    echo "$bundle_name"
    echo "  tarball: $(basename "$tarball")"
    echo "  binaries: ${#targets[@]}"
    echo "  docs: ${#doc_list[@]} files copied (README/START_HERE/INSTALL/OPEN_SOURCE/SHOWCASE/DATASHEET/ACCEPTANCE/etc)"
    echo "  root notices: LICENSE CONTRIBUTING.md"
  } >>"$manifest_file"
  rm -rf "$stage_dir"
done

echo "Release artifacts written to $release_dir"
echo "Checksums: $checksums_file"
