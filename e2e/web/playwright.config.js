const path = require("path");
const { defineConfig } = require("@playwright/test");

const repoRoot = process.env.WOLFBBS_E2E_REPO_ROOT || path.resolve(__dirname, "..", "..");
const localWebPort = process.env.WOLFBBS_E2E_LOCAL_WEB_PORT || "18080";
const localBaseURL = `http://127.0.0.1:${localWebPort}`;
const externalBaseURL = process.env.WOLFBBS_E2E_BASE_URL;
const baseURL = externalBaseURL || localBaseURL;
const sqlitePath = process.env.WOLFBBS_E2E_SQLITE_PATH || "/tmp/wolfbbs-playwright-e2e.db";
const configuredDatabaseURL =
  process.env.WOLFBBS_E2E_DATABASE_URL ||
  (externalBaseURL
    ? process.env.WOLFBBS_DATABASE_URL || process.env.DATABASE_URL
    : undefined) ||
  `sqlite://${sqlitePath}`;

function normalizeDatabaseURL(value) {
  if (!value || value.startsWith("sqlite://") || value.startsWith("file:") || value === ":memory:") {
    return value;
  }
  try {
    const parsed = new URL(value);
    if (parsed.hostname === "postgres") {
      parsed.hostname = process.env.WOLFBBS_E2E_DATABASE_HOST || "127.0.0.1";
    }
    return parsed.toString();
  } catch (_) {
    return value;
  }
}

const databaseURL = normalizeDatabaseURL(configuredDatabaseURL);
const offlineDir = process.env.WOLFBBS_E2E_OFFLINE_DIR || path.join(repoRoot, ".wolfbbs", "offline");
const sqliteMode =
  databaseURL === ":memory:" ||
  databaseURL.startsWith("sqlite://") ||
  databaseURL.startsWith("file:");
const bootstrapAdminHandle = process.env.WOLFBBS_BOOTSTRAP_ADMIN_HANDLE || "sysop";
const bootstrapAdminPassword = process.env.WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD || "password123";
const bootstrapUserHandle = process.env.WOLFBBS_BOOTSTRAP_USER_HANDLE || "caller";
const bootstrapUserPassword = process.env.WOLFBBS_BOOTSTRAP_USER_PASSWORD || "password123";
const ircPort = process.env.WOLFBBS_E2E_IRC_PORT || (sqliteMode ? "1" : "16667");

process.env.WOLFBBS_E2E_IRC_PORT = ircPort;
process.env.WOLFBBS_E2E_ADMIN_HANDLE = process.env.WOLFBBS_E2E_ADMIN_HANDLE || bootstrapAdminHandle;
process.env.WOLFBBS_E2E_ADMIN_PASSWORD = process.env.WOLFBBS_E2E_ADMIN_PASSWORD || bootstrapAdminPassword;
process.env.WOLFBBS_E2E_USER_HANDLE = process.env.WOLFBBS_E2E_USER_HANDLE || bootstrapUserHandle;
process.env.WOLFBBS_E2E_USER_PASSWORD = process.env.WOLFBBS_E2E_USER_PASSWORD || bootstrapUserPassword;

function shellQuote(value) {
  return `'${String(value).replace(/'/g, `'\\''`)}'`;
}

const sharedEnv = [
  `WOLFBBS_DATABASE_URL=${shellQuote(databaseURL)}`,
  `WOLFBBS_OFFLINE_DIR=${shellQuote(offlineDir)}`,
  `WOLFBBS_BOOTSTRAP_ADMIN_HANDLE=${shellQuote(bootstrapAdminHandle)}`,
  `WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD=${shellQuote(bootstrapAdminPassword)}`,
  `WOLFBBS_BOOTSTRAP_USER_HANDLE=${shellQuote(bootstrapUserHandle)}`,
  `WOLFBBS_BOOTSTRAP_USER_PASSWORD=${shellQuote(bootstrapUserPassword)}`,
  "WOLFBBS_DEV_SHOW_RESET_TOKEN=true",
  "WOLFBBS_GUEST_TOUR_ENABLE=true",
  "WOLFBBS_DISCOVER_ENABLE=true",
  "WOLFBBS_QUICK_JUMP_ENABLE=true",
  "WOLFBBS_CLASSIC_SEARCH_ENABLE=true",
  "WOLFBBS_ACTIVITYPUB_ENABLE=true",
  `WOLFBBS_ACTIVITYPUB_BASE_URL=${shellQuote(baseURL)}`,
];
const envPrefix = `env ${sharedEnv.join(" ")}`;

const webServer = externalBaseURL
  ? undefined
  : {
      command: sqliteMode
        ? `${envPrefix} go run ./cmd/wolfbbs-web -listen 127.0.0.1:${localWebPort}`
        : `${envPrefix} sh -c ${shellQuote(
            `trap 'kill 0' EXIT; ` +
              `go run ./cmd/wolfbbs-irc -listen 127.0.0.1:${ircPort} >/tmp/wolfbbs-e2e-irc.log 2>&1 & ` +
              `go run ./cmd/wolfbbs-web -listen 127.0.0.1:${localWebPort}`,
          )}`,
      cwd: repoRoot,
      url: `${baseURL}/healthz`,
      reuseExistingServer: false,
      timeout: 120_000,
    };

module.exports = defineConfig({
  testDir: path.join(__dirname, "tests"),
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 60_000,
  expect: {
    timeout: 10_000,
  },
  reporter: [["list"]],
  use: {
    baseURL,
    headless: true,
  },
  webServer,
});
