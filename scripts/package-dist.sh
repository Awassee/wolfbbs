#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

OUT_DIR="$ROOT_DIR/dist"
VERSION="${WOLFBBS_VERSION:-}"
PLATFORMS=()

usage() {
  cat <<'USAGE'
WolfBBS distribution packager

Usage:
  scripts/package-dist.sh [options]

Options:
  --out <dir>             output directory (default: dist/)
  --version <value>       release version label
  --platform <os/arch>    build a platform bundle; may be repeated
  -h, --help              show this help

Examples:
  scripts/package-dist.sh
  scripts/package-dist.sh --platform linux/amd64 --platform linux/arm64
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

  cp README.md docker-compose.yml .env.example install.sh "$bundle_root/"
  cp docs/INSTALL.md docs/ACCEPTANCE_SPEC.md docs/feature-reference.md docs/irc-compat.md docs/doors.md docs/chat.md docs/admin.md "$bundle_root/docs/"
  cp scripts/verify.sh scripts/build.sh "$bundle_root/scripts/"
  chmod +x "$bundle_root/install.sh" "$bundle_root/scripts/verify.sh" "$bundle_root/scripts/build.sh"
  chmod +x "$bundle_root/bin/"*

  cat >"$bundle_root/RELEASE_NOTES.txt" <<EOF
WolfBBS distribution bundle
Version: $VERSION
Platform: $os/$arch

Contents:
- bin/: server, web, irc, mailin, trivia, and oputil binaries
- install.sh: installer and upgrade entrypoint
- docker-compose.yml + .env.example: default stack runtime
- docs/: install, acceptance, feature reference, irc compatibility, doors, chat, and admin references
- scripts/: verify and build helpers

Quick start:
1. Review docs/INSTALL.md
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
    echo "  docs: INSTALL, ACCEPTANCE_SPEC, feature-reference, irc-compat, doors, chat, admin"
  } >>"$manifest_file"
  rm -rf "$stage_dir"
done

echo "Release artifacts written to $release_dir"
echo "Checksums: $checksums_file"
