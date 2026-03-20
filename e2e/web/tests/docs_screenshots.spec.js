const fs = require("fs");
const path = require("path");
const { test, expect } = require("@playwright/test");

const ADMIN_HANDLE = process.env.WOLFBBS_E2E_ADMIN_HANDLE || "sysop";
const ADMIN_PASSWORD = process.env.WOLFBBS_E2E_ADMIN_PASSWORD || "password123";
const USER_HANDLE = process.env.WOLFBBS_E2E_USER_HANDLE || "caller";
const USER_PASSWORD = process.env.WOLFBBS_E2E_USER_PASSWORD || "password123";
const CAPTURE_ENABLED = process.env.WOLFBBS_CAPTURE_SCREENSHOTS === "1";
const SCREENSHOT_DIR = path.resolve(__dirname, "../../../docs/assets/screenshots");

async function login(page, handle, password) {
  await page.goto("/login");
  await expect(page.locator("h1")).toContainText(/Web Login/i);
  await page.fill('input[name="handle"]', handle);
  await page.fill('input[name="password"]', password);
  await page.click('button[type="submit"]');
}

async function capture(page, route, filename, headingPattern) {
  await page.goto(route);
  if (headingPattern) {
    await expect(page.locator("h1")).toContainText(headingPattern);
  } else {
    await expect(page.locator("body")).toBeVisible();
  }
  await page.waitForTimeout(300);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.waitForTimeout(100);
  await page.screenshot({
    path: path.join(SCREENSHOT_DIR, filename),
    fullPage: false,
  });
}

test.describe("docs screenshot capture", () => {
  test.skip(!CAPTURE_ENABLED, "Set WOLFBBS_CAPTURE_SCREENSHOTS=1 to capture docs screenshots.");

  test("capture core product surfaces for README/docs", async ({ browser }) => {
    fs.mkdirSync(SCREENSHOT_DIR, { recursive: true });

    const context = await browser.newContext({
      viewport: { width: 1600, height: 1000 },
      colorScheme: "dark",
    });
    const page = await context.newPage();

    await capture(page, "/connect", "connect.png", /Connect/i);

    await login(page, USER_HANDLE, USER_PASSWORD);
    await expect(page).toHaveURL(/\/(boards|today|start)/);
    await capture(page, "/boards", "boards.png", /Message Boards/i);
    await capture(page, "/chat", "chat.png", /Chat/i);
    await capture(page, "/doors", "doors.png", /Door/i);
    await capture(page, "/today", "today.png", /Today Brief/i);
    await context.close();

    const adminContext = await browser.newContext({
      viewport: { width: 1600, height: 1000 },
      colorScheme: "dark",
    });
    const adminPage = await adminContext.newPage();
    await login(adminPage, ADMIN_HANDLE, ADMIN_PASSWORD);
    await expect(adminPage).toHaveURL(/\/(boards|today|start)/);
    await capture(adminPage, "/admin/setup", "admin-setup.png", /Setup/i);
    await capture(adminPage, "/admin/config", "admin-config.png", /Config/i);
    await adminContext.close();

    const mobileContext = await browser.newContext({
      viewport: { width: 430, height: 932 },
      colorScheme: "dark",
      userAgent:
        "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
    });
    const mobilePage = await mobileContext.newPage();
    await capture(mobilePage, "/connect", "connect-mobile.png", /Connect/i);
    await mobileContext.close();
  });
});
