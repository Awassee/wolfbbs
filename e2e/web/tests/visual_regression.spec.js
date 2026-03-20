const { test, expect } = require("@playwright/test");

const ADMIN_HANDLE = process.env.WOLFBBS_E2E_ADMIN_HANDLE || "sysop";
const ADMIN_PASSWORD = process.env.WOLFBBS_E2E_ADMIN_PASSWORD || "password123";
const USER_HANDLE = process.env.WOLFBBS_E2E_USER_HANDLE || "caller";
const USER_PASSWORD = process.env.WOLFBBS_E2E_USER_PASSWORD || "password123";

async function login(page, handle, password, path = "/login") {
  await page.goto(path);
  await page.fill('input[name="handle"]', handle);
  await page.fill('input[name="password"]', password);
  await page.click('button[type="submit"]');
}

async function stabilizeForScreenshot(page) {
  await page.waitForLoadState("networkidle");
  await page.waitForTimeout(300);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.addStyleTag({
    content: `
      *, *::before, *::after {
        animation: none !important;
        transition: none !important;
        caret-color: transparent !important;
      }
      #wolfbbsScrollProgress,
      #wolfbbsBackToTop,
      #wolfbbsCommandButton,
      #wolfbbsUX20PrimaryAction,
      #wolfbbsMobileToolsButton,
      #wolfbbsActionDock,
      #wolfbbsNotesButton,
      #wolfbbsUXDiagButton,
      #wolfbbsFeedbackButton,
      #wolfbbsBugButton,
      .wolfbbs-page-hero-meta,
      .wolfbbs-session-chip,
      [data-kind="latency"],
      [data-kind="telemetry"],
      [data-kind="net-online"],
      [data-kind="net-offline"] {
        visibility: hidden !important;
      }
    `,
  });
  await page.waitForTimeout(120);
}

async function expectRouteScreenshot(page, route, heading, name, options = {}) {
  await page.goto(route);
  await expect(page.locator("h1")).toContainText(heading);
  await stabilizeForScreenshot(page);
  await expect(page).toHaveScreenshot(name, {
    fullPage: false,
    animations: "disabled",
    maxDiffPixels: options.maxDiffPixels || 600,
  });
}

test.describe("secondary route visual regression", () => {
  test("caller dense routes keep layout integrity", async ({ browser }) => {
    const context = await browser.newContext({
      viewport: { width: 1600, height: 1000 },
      colorScheme: "dark",
    });
    const page = await context.newPage();

    await login(page, USER_HANDLE, USER_PASSWORD);
    await expect(page).toHaveURL(/\/(boards|today|start)/);

    await expectRouteScreenshot(page, "/boards", /Message Boards/i, "caller-boards.png", {
      // Dense caller chrome and live board counters create small rendering jitter between runs.
      maxDiffPixels: 4000,
    });
    await expectRouteScreenshot(page, "/doors", /Door/i, "caller-doors.png", {
      // The doors grid has a small amount of browser/font jitter on CI macOS runners.
      maxDiffPixels: 1800,
    });

    await context.close();
  });

  test("admin dense routes keep layout integrity", async ({ browser }) => {
    const context = await browser.newContext({
      viewport: { width: 1600, height: 1000 },
      colorScheme: "dark",
    });
    const page = await context.newPage();

    await login(page, ADMIN_HANDLE, ADMIN_PASSWORD, "/admin/login");
    await expect(page).toHaveURL(/\/admin$/);

    await expectRouteScreenshot(page, "/admin/setup", /Setup/i, "admin-setup.png");
    await expectRouteScreenshot(page, "/admin/config", /Config/i, "admin-config.png");

    await context.close();
  });
});
