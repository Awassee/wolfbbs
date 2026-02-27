#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

MODE="guided"
RUN_SMOKE=true
REPORT_PATH="${REPORT_PATH:-docs/manual-acceptance-latest.md}"

usage() {
  cat <<'USAGE'
WolfBBS manual acceptance runner

Usage:
  scripts/manual-acceptance.sh [options]

Options:
  --guided         Interactive pass/fail/skip prompts (default)
  --non-interactive
                   Print checklist only and mark all MANUAL checks skipped
  --no-smoke       Skip scripts/verify.sh --smoke precheck
  --report <path>  Write markdown report to path
  -h, --help       Show help
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --guided)
      MODE="guided"
      ;;
    --non-interactive)
      MODE="non_interactive"
      ;;
    --no-smoke)
      RUN_SMOKE=false
      ;;
    --report)
      if [[ $# -lt 2 ]]; then
        echo "missing value for --report" >&2
        exit 2
      fi
      REPORT_PATH="$2"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

if [[ "$RUN_SMOKE" == "true" ]]; then
  echo "[precheck] scripts/verify.sh --smoke"
  scripts/verify.sh --smoke --keep-stack
fi

declare -a IDS=(
  "BBS-003" "BBS-004" "BBS-005" "BBS-006" "BBS-007" "BBS-008" "BBS-009"
  "WA-003" "WA-004" "WA-005" "WA-006" "WA-007" "WA-008" "WA-009"
  "CH-003" "CH-004" "CH-005"
  "IRC-007"
  "GW-EMAIL-003" "GW-WEB-001" "GW-WEB-004"
)

desc_for() {
  case "$1" in
    BBS-003) echo "ANSI flow: Welcome -> Login -> Main Menu -> sections" ;;
    BBS-004) echo "User registration + login" ;;
    BBS-005) echo "Boards list/read/post/reply" ;;
    BBS-006) echo "Private mail inbox/outbox/send/read/reply/delete" ;;
    BBS-007) echo "New scan since last login" ;;
    BBS-008) echo "Paging (More) behavior" ;;
    BBS-009) echo "Per-user ANSI on/off toggle" ;;
    WA-003) echo "Admin users: list/search/disable/ban/reset/audit summary" ;;
    WA-004) echo "Admin boards: create/edit/delete/perms" ;;
    WA-005) echo "Moderation: delete/edit reason, lock, move, queue/reports" ;;
    WA-006) echo "Gateway settings pages exist and work" ;;
    WA-007) echo "Chat admin: channels + kick/ban/mute + logs" ;;
    WA-008) echo "Admin actions recorded in visible audit log" ;;
    WA-009) echo "Optional admin TOTP/2FA" ;;
    CH-003) echo "Realtime web chat between two sessions" ;;
    CH-004) echo "Chat history persistence + pagination" ;;
    CH-005) echo "Moderation consistency across web/irc/bbs" ;;
    IRC-007) echo "Cross-client bridge: IRC<->Web (and BBS when available)" ;;
    GW-EMAIL-003) echo "Admin can disable outbound email for user" ;;
    GW-WEB-001) echo "Text web gateway fetch + ANSI render works" ;;
    GW-WEB-004) echo "Save for offline reading works" ;;
    *) echo "Unknown manual check" ;;
  esac
}

step_for() {
  case "$1" in
    BBS-003) echo "ssh localhost -p 2222" ;;
    BBS-004) echo "Create account at login prompt, then sign in" ;;
    BBS-005) echo "Use boards menu: list -> reader -> post -> reply" ;;
    BBS-006) echo "Send mail between two local users and verify inbox/outbox" ;;
    BBS-007) echo "Post as user A, login as user B, run newscan" ;;
    BBS-008) echo "Open long thread and verify More prompt" ;;
    BBS-009) echo "Toggle ANSI in settings and verify ascii-safe render" ;;
    WA-003) echo "Open /admin/users and execute user actions" ;;
    WA-004) echo "Open /admin/boards and execute board actions" ;;
    WA-005) echo "Moderate content via /admin/boards + /admin/chat" ;;
    WA-006) echo "Open /admin/gateways and save settings" ;;
    WA-007) echo "Open /admin/chat and perform moderation actions" ;;
    WA-008) echo "Open /admin/audit after admin actions" ;;
    WA-009) echo "Enable/verify 2FA for sysop in settings" ;;
    CH-003) echo "Open /chat in two browsers, send/receive in #lobby" ;;
    CH-004) echo "Reload /chat and verify history still present" ;;
    CH-005) echo "Mute/kick/ban in admin then test from IRC and web clients" ;;
    IRC-007) echo "Use scripts/test_irc.py and verify message appears in web chat" ;;
    GW-EMAIL-003) echo "Disable outbound for target user in /admin/mail and retry send" ;;
    GW-WEB-001) echo "Run text web gateway from ANSI, verify readable output" ;;
    GW-WEB-004) echo "Use save-for-offline and verify content is persisted" ;;
    *) echo "See acceptance spec" ;;
  esac
}

mkdir -p "$(dirname "$REPORT_PATH")"
{
  echo "# WolfBBS Manual Acceptance Report"
  echo
  echo "- Date: $(date -u +"%Y-%m-%d %H:%M:%S UTC")"
  echo "- Mode: $MODE"
  echo "- Repo: $ROOT_DIR"
  echo
  echo "| ID | Result | Description | Notes |"
  echo "|---|---|---|---|"
} >"$REPORT_PATH"

pass_count=0
fail_count=0
skip_count=0

for id in "${IDS[@]}"; do
  desc="$(desc_for "$id")"
  step="$(step_for "$id")"
  result="SKIPPED"
  notes="Not executed"

  if [[ "$MODE" == "guided" ]]; then
    echo
    echo "[$id] $desc"
    echo "  Step: $step"
    printf "  Result? [p]ass [f]ail [s]kip: "
    read -r answer
    case "${answer:-s}" in
      p|P|pass|PASS)
        result="PASS"
        notes="Completed manually"
        pass_count=$((pass_count + 1))
        ;;
      f|F|fail|FAIL)
        result="FAIL"
        notes="Manual validation failed"
        fail_count=$((fail_count + 1))
        ;;
      *)
        result="SKIPPED"
        notes="Skipped by operator"
        skip_count=$((skip_count + 1))
        ;;
    esac
  else
    skip_count=$((skip_count + 1))
  fi

  echo "| $id | $result | $desc | $notes |" >>"$REPORT_PATH"
done

{
  echo
  echo "## Summary"
  echo
  echo "- PASS: $pass_count"
  echo "- FAIL: $fail_count"
  echo "- SKIPPED: $skip_count"
} >>"$REPORT_PATH"

echo
echo "Manual report written to $REPORT_PATH"
echo "PASS=$pass_count FAIL=$fail_count SKIPPED=$skip_count"

if [[ "$fail_count" -gt 0 ]]; then
  exit 1
fi
