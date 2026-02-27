const { test, expect } = require("@playwright/test");
const AxeBuilder = require("@axe-core/playwright").default;

async function login(page, handle, password, path = "/login") {
  await page.goto(path);
  await page.fill('input[name="handle"]', handle);
  await page.fill('input[name="password"]', password);
  await page.click('button[type="submit"]');
}

async function expectNoCriticalA11y(page) {
  const report = await new AxeBuilder({ page }).analyze();
  const critical = (report.violations || []).filter((v) => {
    return v.impact === "critical" || v.impact === "serious";
  });
  expect(critical, JSON.stringify(critical, null, 2)).toEqual([]);
}

test("user web journey supports keyboard navigation and status/config visibility", async ({
  page,
}) => {
  await page.goto("/login");
  await expect(page.locator("h1")).toContainText("WolfBBS Web Login");
  await expectNoCriticalA11y(page);

  await login(page, "caller", "password123");
  await expect(page).toHaveURL(/\/boards$/);
  await expect(page.locator("h1")).toContainText("Message Boards");

  await page.locator('a[href="/mail"]').focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/mail$/);
  await expect(page.locator("h1")).toContainText("Private Mail");

  await page.goto("/boards?board=1");
  await page.fill('input[name="subject"]', "Playwright Post");
  await page.fill('textarea[name="body"]', "Testing from browser flow.");
  await page.getByRole("button", { name: "Post" }).click();
  await expect(page.locator("body")).toContainText("Playwright Post");

  await page.goto("/boards?board=1");
  const link = page.locator('a:has-text("Playwright Post")').first();
  await expect(link).toBeVisible();
  await link.click();
  await expect(page.locator("h2")).toContainText("Reader");
  await page.fill('textarea[name="body"]', "Reply body from playwright.");
  await page.getByRole("button", { name: "Post Reply" }).click();
  await expect(page.locator("body")).toContainText("Playwright Post");

  await page.goto("/status");
  await expect(page.locator("h1")).toContainText("Status Center");
  await expect(page.locator("body")).toContainText("Quick jump");
  await expect(page.locator("body")).toContainText("Guest tour");

  await page.goto("/config");
  await expect(page.locator("h1")).toContainText("Config Center");
  await expect(page.locator("body")).toContainText("Runtime Feature Flags");
  await expectNoCriticalA11y(page);
});

test("admin journey enforces RBAC and exposes sysop pages", async ({ browser }) => {
  const adminCtx = await browser.newContext();
  const adminPage = await adminCtx.newPage();
  await login(adminPage, "sysop", "password123", "/admin/login");
  await expect(adminPage).toHaveURL(/\/admin$/);
  await expect(adminPage.locator("h1")).toContainText("Sysop Control Panel");

  await adminPage.locator('a[href="/admin/users"]').focus();
  await adminPage.keyboard.press("Enter");
  await expect(adminPage).toHaveURL(/\/admin\/users$/);
  await expect(adminPage.locator("h1")).toContainText("Sysop Users");

  await adminPage.goto("/admin/boards");
  await expect(adminPage.locator("h1")).toContainText("Sysop Boards");
  await adminPage.goto("/admin/config");
  await expect(adminPage.locator("h1")).toContainText("Runtime Config");
  await adminPage.goto("/admin/system");
  await expect(adminPage.locator("h1")).toContainText("System / WFC Dashboard");
  await expectNoCriticalA11y(adminPage);

  const userCtx = await browser.newContext();
  const userPage = await userCtx.newPage();
  await login(userPage, "caller", "password123");
  const resp = await userPage.goto("/admin");
  expect(resp).not.toBeNull();
  expect(resp.status()).toBe(403);
  await userCtx.close();
  await adminCtx.close();
});

test("chat syncs between two web sessions in realtime", async ({ browser }) => {
  const aCtx = await browser.newContext();
  const bCtx = await browser.newContext();
  const aPage = await aCtx.newPage();
  const bPage = await bCtx.newPage();

  await login(aPage, "sysop", "password123");
  await login(bPage, "caller", "password123");
  await aPage.goto("/chat");
  await bPage.goto("/chat");

  await aPage.fill("#message", "hello from playwright chat");
  await aPage.getByRole("button", { name: "Send" }).click();

  await expect
    .poll(async () => {
      return bPage.locator("#chat").innerText();
    })
    .toContain("hello from playwright chat");
  await aCtx.close();
  await bCtx.close();
});
