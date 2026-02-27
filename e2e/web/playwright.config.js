const path = require("path");
const { defineConfig } = require("@playwright/test");

const repoRoot = process.env.WOLFBBS_E2E_REPO_ROOT || path.resolve(__dirname, "..", "..");
const baseURL = process.env.WOLFBBS_E2E_BASE_URL || "http://127.0.0.1:18080";
const sqlitePath = process.env.WOLFBBS_E2E_SQLITE_PATH || "/tmp/wolfbbs-playwright-e2e.db";

const webServer = process.env.WOLFBBS_E2E_BASE_URL
  ? undefined
  : {
      command:
        `WOLFBBS_DATABASE_URL=sqlite://${sqlitePath} ` +
        "WOLFBBS_BOOTSTRAP_ADMIN_HANDLE=sysop " +
        "WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD=password123 " +
        "WOLFBBS_BOOTSTRAP_USER_HANDLE=caller " +
        "WOLFBBS_BOOTSTRAP_USER_PASSWORD=password123 " +
        "WOLFBBS_GUEST_TOUR_ENABLE=true " +
        "WOLFBBS_DISCOVER_ENABLE=true " +
        "WOLFBBS_QUICK_JUMP_ENABLE=true " +
        "WOLFBBS_CLASSIC_SEARCH_ENABLE=true " +
        "go run ./cmd/wolfbbs-web -listen 127.0.0.1:18080",
      cwd: repoRoot,
      url: `${baseURL}/healthz`,
      reuseExistingServer: true,
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
