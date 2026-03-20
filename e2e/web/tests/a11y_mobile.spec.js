const { test, expect } = require("@playwright/test");
const AxeBuilder = require("@axe-core/playwright").default;

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

async function expectNoSeriousViolations(page, label) {
  const report = await new AxeBuilder({ page })
    .disableRules(["landmark-one-main", "region"])
    .analyze();
  const blocking = report.violations.filter((violation) =>
    ["serious", "critical"].includes(String(violation.impact || "").toLowerCase()),
  );
  expect(blocking, `${label} serious/critical accessibility violations`).toEqual([]);
}

test.describe("accessibility and mobile regression", () => {
  test("keyboard users can launch the omnibar and navigate", async ({ browser }) => {
    const context = await browser.newContext({
      viewport: { width: 1440, height: 960 },
      colorScheme: "dark",
    });
    const page = await context.newPage();

    await login(page, USER_HANDLE, USER_PASSWORD);
    await expect(page).toHaveURL(/\/(boards|today|start)/);

    await page.keyboard.press("Control+K");
    await expect(page.locator("#wolfbbsPalette")).toBeVisible();
    await page.locator("#wolfbbsPaletteInput").fill("open boards");
    await page.locator("#wolfbbsPaletteInput").press("Enter");
    await expect(page).toHaveURL(/\/boards$/);

    await context.close();
  });

  test("core caller and sysop screens avoid serious accessibility regressions", async ({ browser }) => {
    const callerContext = await browser.newContext({
      viewport: { width: 1440, height: 960 },
      colorScheme: "dark",
    });
    const callerPage = await callerContext.newPage();
    await login(callerPage, USER_HANDLE, USER_PASSWORD);
    await callerPage.goto("/start");
    await expect(callerPage.locator("h1")).toContainText(/Start|Boards|Today/i);
    await expectNoSeriousViolations(callerPage, "/start");
    await callerPage.goto("/boards");
    await expect(callerPage.locator("h1")).toContainText(/Message Boards/i);
    await expectNoSeriousViolations(callerPage, "/boards");
    await callerContext.close();

    const adminContext = await browser.newContext({
      viewport: { width: 1440, height: 960 },
      colorScheme: "dark",
    });
    const adminPage = await adminContext.newPage();
    await login(adminPage, ADMIN_HANDLE, ADMIN_PASSWORD, "/admin/login");
    await adminPage.goto("/admin/setup");
    await expect(adminPage.locator("h1")).toContainText(/Setup/i);
    await expectNoSeriousViolations(adminPage, "/admin/setup");
    await adminContext.close();
  });

  test("mobile quick tools stay reachable on dense admin routes", async ({ browser }) => {
    const context = await browser.newContext({
      viewport: { width: 430, height: 932 },
      colorScheme: "dark",
      isMobile: true,
      hasTouch: true,
    });
    const page = await context.newPage();

    await login(page, ADMIN_HANDLE, ADMIN_PASSWORD, "/admin/login");
    await page.goto("/admin/config");
    await expect(page.locator("#wolfbbsMobileToolsButton")).toBeVisible();
    await page.locator("#wolfbbsMobileToolsButton").click();
    await expect(page.locator("#wolfbbsMobileToolsPanel")).toBeVisible();
    await expect(page.locator("#wolfbbsMobileToolsPanel")).toContainText(/Quick Tools/i);
    await expect(page.locator("#wolfbbsMobileToolsList")).toContainText(/Open omnibar/i);
    await page.locator("#wolfbbsMobileToolsClose").click();
    await expect(page.locator("#wolfbbsMobileToolsOverlay")).not.toHaveClass(/active/);

    await context.close();
  });
});
