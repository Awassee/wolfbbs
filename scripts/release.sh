#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

VERSION=""
NOTES_FILE=""
PUBLISH=false
GITHUB_RELEASE=false
ALLOW_DIRTY=false
NO_VERIFY=false
NO_SYNC_REFS=false
REMOTE="origin"
PLATFORMS=()
RUN_SMOKE=false
RUN_FUNCTIONAL=false
RUN_MANUAL=false
RUN_SECURITY=false
WEB_TIMEOUT_SECONDS="${WOLFBBS_RELEASE_WEB_TIMEOUT_SECONDS:-900}"

usage() {
  cat <<'USAGE'
WolfBBS release automation

Usage:
  scripts/release.sh --version vX.Y.Z [options]

Options:
  --version <tag>         release tag/version to ship (required)
  --notes <path>          release notes file (default: docs/releases/<version>.md)
  --platform <os/arch>    package target; may be repeated (default: public 4-platform matrix)
  --publish               commit synced docs/notes, create tag, and push branch + tag
  --github-release        create/update GitHub release with gh after packaging
  --remote <name>         git remote to push/release against (default: origin)
  --smoke                 run scripts/verify.sh --smoke before packaging
  --functional            run scripts/run-e2e.sh before packaging
  --manual-auto           run manual acceptance auto mode before packaging
  --security              run scripts/security-audit.sh before packaging
  --full-qa               run smoke + functional + manual acceptance + security audit before packaging
  --web-timeout <sec>     web e2e timeout for --functional (default: 900)
  --allow-dirty           skip clean-worktree guard
  --no-verify             skip shellcheck + verify fast
  --no-sync-refs          do not replace current-release doc refs
  -h, --help              show this help

Examples:
  scripts/release.sh --version v1.1.23
  scripts/release.sh --version v1.1.23 --publish --github-release --remote awassee
  scripts/release.sh --version v1.1.23 --full-qa --publish --github-release --remote awassee
USAGE
}

require_value() {
  local flag="$1"
  local value="${2:-}"
  if [[ -z "$value" ]]; then
    echo "Missing value for ${flag}" >&2
    exit 2
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      require_value "$1" "${2:-}"
      VERSION="$2"
      shift 2
      ;;
    --notes)
      require_value "$1" "${2:-}"
      NOTES_FILE="$2"
      shift 2
      ;;
    --platform)
      require_value "$1" "${2:-}"
      PLATFORMS+=("$2")
      shift 2
      ;;
    --publish)
      PUBLISH=true
      shift
      ;;
    --github-release)
      GITHUB_RELEASE=true
      shift
      ;;
    --remote)
      require_value "$1" "${2:-}"
      REMOTE="$2"
      shift 2
      ;;
    --smoke)
      RUN_SMOKE=true
      shift
      ;;
    --functional)
      RUN_FUNCTIONAL=true
      shift
      ;;
    --manual-auto)
      RUN_MANUAL=true
      shift
      ;;
    --security)
      RUN_SECURITY=true
      shift
      ;;
    --full-qa)
      RUN_SMOKE=true
      RUN_FUNCTIONAL=true
      RUN_MANUAL=true
      RUN_SECURITY=true
      shift
      ;;
    --web-timeout)
      require_value "$1" "${2:-}"
      WEB_TIMEOUT_SECONDS="$2"
      shift 2
      ;;
    --allow-dirty)
      ALLOW_DIRTY=true
      shift
      ;;
    --no-verify)
      NO_VERIFY=true
      shift
      ;;
    --no-sync-refs)
      NO_SYNC_REFS=true
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
done

if [[ -z "$VERSION" ]]; then
  echo "--version is required" >&2
  usage >&2
  exit 2
fi

if [[ -z "$NOTES_FILE" ]]; then
  NOTES_FILE="docs/releases/${VERSION}.md"
fi

if ! [[ "$WEB_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || [[ "$WEB_TIMEOUT_SECONDS" -le 0 ]]; then
  echo "--web-timeout must be a positive integer" >&2
  exit 2
fi

current_release_tag() {
  git describe --tags --abbrev=0 2>/dev/null || true
}

ensure_clean_worktree() {
  if [[ "$ALLOW_DIRTY" == "true" ]]; then
    return
  fi
  if [[ -n "$(git status --short)" ]]; then
    echo "Working tree is dirty. Commit or stash changes first, or use --allow-dirty." >&2
    exit 1
  fi
}

sync_current_release_refs() {
  local previous_tag="$1"
  if [[ "$NO_SYNC_REFS" == "true" || -z "$previous_tag" || "$previous_tag" == "$VERSION" ]]; then
    return
  fi
  local files=(
    "README.md"
    "docs/DATASHEET.md"
    "docs/FIRST_30_MINUTES.md"
    "docs/INSTALL.md"
    "docs/README.md"
    "docs/SHOWCASE.md"
    "docs/START_HERE.md"
  )
  local file=""
  for file in "${files[@]}"; do
    [[ -f "$file" ]] || continue
    perl -0pi -e "s/\\Q${previous_tag}\\E/${VERSION}/g" "$file"
  done
  if [[ -f "docs/releases/README.md" ]]; then
    perl -0pi -e 's/^Current public release: `[^`]+`$/Current public release: `'"${VERSION}"'`/m' "docs/releases/README.md"
    perl -0pi -e "s/^- \\[\\Q${previous_tag}\\E\\]\\(\\Q${previous_tag}\\E\\.md\\): current public distribution release\\.\$/- [${VERSION}](${VERSION}.md): current public distribution release./m" "docs/releases/README.md"
    perl -0pi -e "if (!/\\Q- [${previous_tag}](${previous_tag}.md)\\E/m) { s/(Recent historical notes:\\n\\n)/\$1- [${previous_tag}](${previous_tag}.md)\\n/s }" "docs/releases/README.md"
  fi
}

ensure_notes_file() {
  if [[ -f "$NOTES_FILE" ]]; then
    return
  fi
  mkdir -p "$(dirname "$NOTES_FILE")"
  cat >"$NOTES_FILE" <<EOF
# ${VERSION}

- summarize release scope
- note release-blocking fixes
- list validation commands and outcomes
- list artifact names or links
EOF
}

remote_repo_slug() {
  local remote="$1"
  local url=""
  url="$(git remote get-url "$remote" 2>/dev/null || true)"
  if [[ -z "$url" ]]; then
    echo "Unable to resolve git remote URL for ${remote}" >&2
    exit 1
  fi
  url="${url%.git}"
  url="${url#git@github.com:}"
  url="${url#https://github.com/}"
  url="${url#http://github.com/}"
  if [[ "$url" != */* ]]; then
    echo "Unable to derive GitHub repo slug from remote ${remote}: ${url}" >&2
    exit 1
  fi
  printf '%s\n' "$url"
}

release_target_ref() {
  local branch="${WOLFBBS_RELEASE_TARGET_BRANCH:-}"
  if [[ -z "$branch" ]]; then
    branch="$(git symbolic-ref --quiet --short HEAD 2>/dev/null || true)"
  fi
  if [[ -z "$branch" ]]; then
    branch="main"
  fi
  printf 'refs/heads/%s\n' "$branch"
}

run_verify() {
  if [[ "$NO_VERIFY" == "true" ]]; then
    return
  fi
  shellcheck install.sh bootstrap.sh scripts/verify.sh scripts/test-installer-regressions.sh scripts/release.sh scripts/security-audit.sh
  scripts/verify.sh --fast
}

run_extended_validation() {
  if [[ "$RUN_SMOKE" == "true" ]]; then
    scripts/verify.sh --smoke
  fi
  if [[ "$RUN_FUNCTIONAL" == "true" ]]; then
    scripts/run-e2e.sh --web-functional-only --web-timeout "$WEB_TIMEOUT_SECONDS"
  fi
  if [[ "$RUN_MANUAL" == "true" ]]; then
    scripts/manual-acceptance.sh --auto --no-smoke --report docs/manual-acceptance-latest.md
  fi
  if [[ "$RUN_SECURITY" == "true" ]]; then
    scripts/security-audit.sh
  fi
}

package_release() {
  local selected_platforms=()
  if [[ ${#PLATFORMS[@]} -eq 0 ]]; then
    selected_platforms=(
      "linux/amd64"
      "linux/arm64"
      "darwin/amd64"
      "darwin/arm64"
    )
  else
    selected_platforms=("${PLATFORMS[@]}")
  fi
  local cmd=(scripts/package-dist.sh --clean --version "$VERSION")
  local platform=""
  for platform in "${selected_platforms[@]-}"; do
    [[ -n "$platform" ]] || continue
    cmd+=(--platform "$platform")
  done
  "${cmd[@]}"
}

commit_if_needed() {
  if [[ -z "$(git status --short)" ]]; then
    return
  fi
  git add README.md docs/DATASHEET.md docs/FIRST_30_MINUTES.md docs/INSTALL.md docs/README.md docs/SHOWCASE.md docs/START_HERE.md docs/manual-acceptance-latest.md docs/releases/README.md scripts/release.sh "$NOTES_FILE"
  git add docs/assets/screenshots
  git add -A -f dist
  if [[ -n "$(git diff --cached --name-only)" ]]; then
    git commit -m "Ship ${VERSION}"
  fi
}

publish_git() {
  local target_ref=""
  if git rev-parse -q --verify "refs/tags/${VERSION}" >/dev/null 2>&1; then
    echo "Tag ${VERSION} already exists locally." >&2
    exit 1
  fi
  commit_if_needed
  git tag "$VERSION"
  target_ref="$(release_target_ref)"
  git -c http.version=HTTP/1.1 -c http.postBuffer=524288000 push "$REMOTE" "HEAD:${target_ref}"
  git -c http.version=HTTP/1.1 -c http.postBuffer=524288000 push "$REMOTE" "$VERSION"
}

publish_github_release() {
  if [[ "$GITHUB_RELEASE" != "true" ]]; then
    return
  fi
  if ! command -v gh >/dev/null 2>&1; then
    echo "gh is required for --github-release" >&2
    exit 1
  fi
  if ! git rev-parse -q --verify "refs/tags/${VERSION}" >/dev/null 2>&1; then
    echo "Tag ${VERSION} must exist before creating a GitHub release. Use --publish or tag it manually." >&2
    exit 1
  fi
  local assets=("dist/${VERSION}"/*.tar.gz)
  local checksum="dist/${VERSION}/checksums.txt"
  local manifest="dist/${VERSION}/release-manifest.txt"
  local repo_slug=""
  repo_slug="$(remote_repo_slug "$REMOTE")"
  gh release create "$VERSION" -R "$repo_slug" --notes-file "$NOTES_FILE" "${assets[@]}" "$checksum" "$manifest"
}

previous_tag="$(current_release_tag)"
ensure_clean_worktree
sync_current_release_refs "$previous_tag"
ensure_notes_file
run_verify
run_extended_validation
package_release

if [[ "$PUBLISH" == "true" ]]; then
  publish_git
fi
publish_github_release

echo "Release staging complete for ${VERSION}"
echo "Notes: ${NOTES_FILE}"
echo "Artifacts: dist/${VERSION}"
