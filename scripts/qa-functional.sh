#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

RUN_WEB_E2E=false
RUN_MANUAL=false
WEB_TIMEOUT_SECONDS="${WOLFBBS_WEB_E2E_TIMEOUT_SECONDS:-900}"

usage() {
  cat <<'USAGE'
WolfBBS functional QA runner

Usage:
  scripts/qa-functional.sh [options]

Options:
  --with-web-e2e         run Playwright functional web checks
  --with-manual-auto     run manual-acceptance auto mapping after functional checks
  --web-timeout <sec>    timeout for web e2e run (default: 900)
  -h, --help             show help
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --with-web-e2e)
      RUN_WEB_E2E=true
      ;;
    --with-manual-auto)
      RUN_MANUAL=true
      ;;
    --web-timeout)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for --web-timeout" >&2
        exit 2
      fi
      WEB_TIMEOUT_SECONDS="$2"
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

if ! [[ "$WEB_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || [[ "$WEB_TIMEOUT_SECONDS" -le 0 ]]; then
  echo "--web-timeout must be a positive integer" >&2
  exit 2
fi

echo "[qa 1/4] admin + settings functional tests"
go test ./cmd/wolfbbs-web -count=1 -run '^(TestAdminSetupConfigAndErrorScreens|TestAdminUsersCreateValidationAndDuplicateErrors|TestAdminUsersLifecycleActionsAndAuthEffects|TestAdminConfigSaveRuntimeServicesAndReload|TestAdminBoardsModerationActions|TestAdminMailDisableOutboundBlocksExternalSend|TestGatewayFilebaseQueueAndTicket|TestChatChannelLockEnforcedForNonModerators|TestSettingsRequiresCSRFAndUpdatesPreferences|TestStatusAndConfigCenters)$'

echo "[qa 2/4] terminal functional tests"
go test ./internal/sshserver -count=1 -run '^(TestSSHLoginFlow|TestSSHBoardPostFlow|TestSSHMailReplyDeleteFlow|TestSSHNewscanDigestShowsRecentTraffic|TestPagerWriteShowsMorePrompt)$'

echo "[qa 3/4] irc functional tests"
go test ./cmd/wolfbbs-irc -count=1 -run '^(TestIRCGatewayFlow|TestIRCGatewayModerationEnforced)$'

if [[ "$RUN_WEB_E2E" == "true" ]]; then
  echo "[qa 4/4] web e2e functional checks"
  scripts/run-e2e.sh --no-go --no-tui --web-timeout "$WEB_TIMEOUT_SECONDS"
else
  echo "[qa 4/4] web e2e skipped (use --with-web-e2e)"
fi

if [[ "$RUN_MANUAL" == "true" ]]; then
  echo "[qa] manual acceptance auto report"
  scripts/manual-acceptance.sh --auto --no-smoke --report docs/manual-acceptance-latest.md
fi

echo "PASS qa-functional"
