#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

MODE="guided"
RUN_SMOKE=true
REPORT_PATH="${REPORT_PATH:-docs/manual-acceptance-latest.md}"
WEB_TIMEOUT_SECONDS="${MANUAL_WEB_TIMEOUT_SECONDS:-900}"

usage() {
  cat <<'USAGE'
WolfBBS manual acceptance runner

Usage:
  scripts/manual-acceptance.sh [options]

Options:
  --guided         Interactive pass/fail/skip prompts (default)
  --non-interactive
                   Print checklist only and mark all MANUAL checks skipped
  --auto           Run automated evidence checks and map results to MANUAL IDs
  --no-smoke       Skip scripts/verify.sh --smoke precheck
  --web-timeout <seconds>
                   Timeout for automated web e2e run in --auto mode (default: 900)
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
    --auto)
      MODE="auto"
      ;;
    --no-smoke)
      RUN_SMOKE=false
      ;;
    --web-timeout)
      if [[ $# -lt 2 ]]; then
        echo "missing value for --web-timeout" >&2
        exit 2
      fi
      WEB_TIMEOUT_SECONDS="$2"
      shift
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

if ! [[ "$WEB_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || [[ "$WEB_TIMEOUT_SECONDS" -le 0 ]]; then
  echo "--web-timeout must be a positive integer; got: $WEB_TIMEOUT_SECONDS" >&2
  exit 2
fi

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

AUTO_RESULTS_FILE="$(mktemp)"
cleanup() {
  rm -f "$AUTO_RESULTS_FILE"
}
trap cleanup EXIT

set_auto_result() {
  local id="$1"
  local result="$2"
  local notes="$3"
  printf '%s|%s|%s\n' "$id" "$result" "$notes" >>"$AUTO_RESULTS_FILE"
}

get_auto_result() {
  local id="$1"
  local line=""
  line="$(grep "^${id}|" "$AUTO_RESULTS_FILE" | tail -n1 || true)"
  if [[ -z "$line" ]]; then
    return 1
  fi
  printf '%s' "$line"
  return 0
}

run_auto_checks() {
  local tui_ok=false
  local web_ok=false

  echo "[auto] terminal e2e: scripts/test_tui_pexpect.py"
  if python3 scripts/test_tui_pexpect.py; then
    tui_ok=true
    set_auto_result "BBS-003" "PASS" "Automated terminal e2e validated welcome/login/main menu flow"
    set_auto_result "BBS-004" "PASS" "Automated terminal e2e validated account creation + login"
  else
    set_auto_result "BBS-003" "FAIL" "Terminal e2e failed; see scripts/test_tui_pexpect.py output"
    set_auto_result "BBS-004" "FAIL" "Terminal e2e failed; see scripts/test_tui_pexpect.py output"
  fi

  echo "[auto] web e2e: scripts/run-e2e.sh --no-go --no-tui --web-timeout ${WEB_TIMEOUT_SECONDS}"
  if scripts/run-e2e.sh --no-go --no-tui --web-timeout "$WEB_TIMEOUT_SECONDS"; then
    web_ok=true
    set_auto_result "BBS-005" "PASS" "Web e2e validated board list/read/post/reply"
    set_auto_result "BBS-009" "PASS" "Web e2e validated ANSI preference toggle in settings"
    set_auto_result "WA-003" "PASS" "Web e2e exercised admin user create/search/disable/ban/unban/role changes"
    set_auto_result "WA-004" "PASS" "Web e2e exercised admin board create/edit/delete + ACS fields"
    set_auto_result "WA-006" "PASS" "Web e2e exercised gateway settings save in /admin/gateways"
    set_auto_result "WA-007" "PASS" "Web e2e exercised chat channel admin + moderation events"
    set_auto_result "WA-008" "PASS" "Web e2e verified admin audit log entries are visible"
    set_auto_result "WA-009" "PASS" "Web e2e exercised sysop 2FA enable/disable flow"
    set_auto_result "CH-003" "PASS" "Web e2e verified realtime chat between two sessions"
    set_auto_result "CH-004" "PASS" "Web e2e verified chat history persists after reload"
    set_auto_result "IRC-007" "PASS" "Web e2e verified IRC -> web chat bridge message visibility"
    set_auto_result "GW-EMAIL-003" "PASS" "Web e2e exercised admin outbound mail disable/enable controls"
    set_auto_result "GW-WEB-001" "PASS" "Web e2e exercised gateway fetch and readable rendering"
    set_auto_result "GW-WEB-004" "PASS" "Web e2e exercised save-for-offline in gateway fetch flow"

    if go test ./cmd/wolfbbs-web -run TestAdminBoardsModerationActions -count=1 >/tmp/wolfbbs-wa005.log 2>&1; then
      set_auto_result "WA-005" "PASS" "Automated moderation test validated delete/lock/move/report queue actions"
    else
      set_auto_result "WA-005" "FAIL" "Automated moderation test failed; see /tmp/wolfbbs-wa005.log"
    fi
  else
    set_auto_result "BBS-005" "FAIL" "Web e2e failed; board flow validation failed"
    set_auto_result "BBS-009" "FAIL" "Web e2e failed; settings/ANSI validation failed"
    set_auto_result "WA-003" "FAIL" "Web e2e failed; admin users validation failed"
    set_auto_result "WA-004" "FAIL" "Web e2e failed; admin boards validation failed"
    set_auto_result "WA-005" "FAIL" "Web e2e failed; moderation validation failed"
    set_auto_result "WA-006" "FAIL" "Web e2e failed; admin gateways validation failed"
    set_auto_result "WA-007" "FAIL" "Web e2e failed; admin chat validation failed"
    set_auto_result "WA-008" "FAIL" "Web e2e failed; admin audit validation failed"
    set_auto_result "WA-009" "FAIL" "Web e2e failed; admin 2FA validation failed"
    set_auto_result "CH-003" "FAIL" "Web e2e failed; realtime chat validation failed"
    set_auto_result "CH-004" "FAIL" "Web e2e failed; chat history validation failed"
    set_auto_result "IRC-007" "FAIL" "Web e2e failed; IRC bridge validation failed"
    set_auto_result "GW-EMAIL-003" "FAIL" "Web e2e failed; outbound mail policy validation failed"
    set_auto_result "GW-WEB-001" "FAIL" "Web e2e failed; web gateway fetch validation failed"
    set_auto_result "GW-WEB-004" "FAIL" "Web e2e failed; offline save validation failed"
  fi

  if go test ./internal/sshserver -run TestSSHMailReplyDeleteFlow -count=1 >/tmp/wolfbbs-bbs006.log 2>&1; then
    set_auto_result "BBS-006" "PASS" "Terminal integration test validated inbox/outbox/send/read/reply/delete flow"
  else
    set_auto_result "BBS-006" "FAIL" "Terminal mail integration test failed; see /tmp/wolfbbs-bbs006.log"
  fi

  if go test ./internal/sshserver -run TestSSHNewscanDigestShowsRecentTraffic -count=1 >/tmp/wolfbbs-bbs007.log 2>&1; then
    set_auto_result "BBS-007" "PASS" "Terminal integration test validated newscan digest since previous login"
  else
    set_auto_result "BBS-007" "FAIL" "Terminal newscan integration test failed; see /tmp/wolfbbs-bbs007.log"
  fi

  if go test ./internal/sshserver -run TestPagerWriteShowsMorePrompt -count=1 >/tmp/wolfbbs-bbs008.log 2>&1; then
    set_auto_result "BBS-008" "PASS" "Pager unit test validated classic More prompt rendering"
  else
    set_auto_result "BBS-008" "FAIL" "Pager unit test failed; see /tmp/wolfbbs-bbs008.log"
  fi

  if [[ "$web_ok" == "true" ]] && go test ./cmd/wolfbbs-irc -run TestIRCGatewayModerationEnforced -count=1 >/tmp/wolfbbs-ch005.log 2>&1; then
    set_auto_result "CH-005" "PASS" "Moderation integration validated mute enforcement in shared IRC/chat backend"
  else
    set_auto_result "CH-005" "FAIL" "Cross-client moderation integration failed; see /tmp/wolfbbs-ch005.log"
  fi
}

if [[ "$MODE" == "auto" ]]; then
  run_auto_checks
fi

mkdir -p "$(dirname "$REPORT_PATH")"
repo_label="$(basename "$ROOT_DIR")"
{
  echo "# WolfBBS Manual Acceptance Report"
  echo
  echo "- Date: $(date -u +"%Y-%m-%d %H:%M:%S UTC")"
  echo "- Mode: $MODE"
  echo "- Repo: $repo_label"
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
  elif [[ "$MODE" == "auto" ]]; then
    auto_line="$(get_auto_result "$id" || true)"
    if [[ -n "${auto_line:-}" ]]; then
      result="$(printf '%s' "$auto_line" | cut -d'|' -f2)"
      notes="$(printf '%s' "$auto_line" | cut -d'|' -f3-)"
      case "$result" in
        PASS) pass_count=$((pass_count + 1)) ;;
        FAIL) fail_count=$((fail_count + 1)) ;;
        *) result="SKIPPED"; skip_count=$((skip_count + 1)) ;;
      esac
    else
      result="SKIPPED"
      notes="No automated evidence mapping yet"
      skip_count=$((skip_count + 1))
    fi
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
