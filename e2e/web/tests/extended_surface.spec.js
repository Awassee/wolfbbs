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

function datetimeLocalMinutes(offsetMinutes = 0) {
  const value = new Date(Date.now() + offsetMinutes * 60 * 1000);
  const pad = (n) => String(n).padStart(2, "0");
  return `${value.getFullYear()}-${pad(value.getMonth() + 1)}-${pad(value.getDate())}T${pad(value.getHours())}:${pad(value.getMinutes())}`;
}

async function gotoHealthy(page, path, headingPattern) {
  const response = await page.goto(path);
  expect(response).not.toBeNull();
  expect(response.status()).toBeLessThan(500);
  if (headingPattern) {
    await expect(page.locator("h1")).toContainText(headingPattern);
  }
  await expect(page.locator("body")).not.toContainText("Internal Server Error");
  await expect(page.locator("body")).not.toContainText("panic:");
}

test("extended caller surfaces and exports stay healthy", async ({ page }) => {
  test.setTimeout(180_000);
  await login(page, USER_HANDLE, USER_PASSWORD);
  await expect(page).toHaveURL(/\/(boards|today)$/);

  const callerRoutes = [
    { path: "/showcase", heading: /Product Showcase/i },
    { path: "/topx", heading: /TopX Leaderboards/i },
    { path: "/streaks", heading: /Daily Streaks/i },
    { path: "/next", heading: /Next Best Actions/i },
    { path: "/spotlights", heading: /Returning Caller Spotlights/i },
    { path: "/challenges", heading: /Seasonal Challenges/i },
    { path: "/missions", heading: /Seasonal Missions/i },
    { path: "/resume", heading: /Smart Re-entry/i },
    { path: "/doors/comeback", heading: /Door Comeback Prompts/i },
    { path: "/mentorship", heading: /Mentorship Pairing/i },
    { path: "/milestones", heading: /Milestone Celebrations/i },
    { path: "/time-lane", heading: /Time-of-Day Landing States/i },
    { path: "/events/recaps", heading: /Event Recaps/i },
    { path: "/digest/preferences", heading: /Digest Preferences By Weekday/i },
  ];
  for (const route of callerRoutes) {
    await gotoHealthy(page, route.path, route.heading);
  }

  await page.fill('input[name="monday"]', "14");
  await page.fill('input[name="friday"]', "8");
  await page.getByRole("button", { name: "Save Weekday Preferences" }).click();
  await expect(page.locator("body")).toContainText("Weekday digest preferences saved.");
  await expect(page.locator('input[name="monday"]')).not.toHaveValue("");
  await expect(page.locator('input[name="friday"]')).not.toHaveValue("");

  const exportRes = await page.request.get("/attention/export");
  expect(exportRes.status()).toBe(200);
  expect((exportRes.headers()["content-type"] || "").toLowerCase()).toContain("application/json");
  const payload = await exportRes.json();
  expect(payload.handle).toBe(USER_HANDLE);
  expect(payload).toHaveProperty("digest_preferences");
  expect(payload).toHaveProperty("attention");

  await gotoHealthy(page, "/today", /Today Brief/i);
  const prefControls = page.locator(".wolfbbs-pref-controls");
  await expect(prefControls).toBeVisible();
  const openControlMenu = async (label) => {
    const menu = prefControls.locator(".wolfbbs-pref-menu").filter({ has: page.locator("summary", { hasText: label }) }).first();
    await expect(menu).toBeVisible();
    await menu.locator("summary").click();
    await expect(menu).toHaveAttribute("open", "");
    return menu;
  };
  const body = page.locator("body");
  const moreMenu = await openControlMenu("More");
  await expect(moreMenu.locator(".wolfbbs-status-grid")).toBeVisible();
  await expect(moreMenu.locator(".wolfbbs-focus-pill")).toBeVisible();
  const layoutButton = moreMenu.getByRole("button", { name: /Layout:/i });
  const layoutBefore = await body.getAttribute("data-layout-mode");
  await layoutButton.click();
  const layoutAfter = await body.getAttribute("data-layout-mode");
  expect(layoutAfter).not.toBe(layoutBefore);
  await expect(moreMenu.getByRole("button", { name: /Accent:/i })).toBeVisible();
  await expect(moreMenu.getByRole("button", { name: /Motion:/i })).toBeVisible();
  await expect(moreMenu.getByRole("button", { name: /Profile:/i })).toBeVisible();
  await expect(prefControls.getByRole("button", { name: "Back" })).toBeVisible();
  await expect(prefControls.getByRole("button", { name: "Search" })).toBeVisible();
  const compactControls = moreMenu.getByRole("button", { name: /Controls:/i });
  await compactControls.click();
  await expect(page.locator("body")).toHaveClass(/wolfbbs-controls-compact/);
  await compactControls.click();
  await expect(page.locator("#wolfbbsNotesButton")).toBeHidden();
  await expect(page.locator("#wolfbbsUXDiagButton")).toBeHidden();
  await expect(page.locator("#wolfbbsFeedbackButton")).toBeHidden();
  await expect(page.locator("#wolfbbsBugButton")).toBeHidden();
  const helpMenu = await openControlMenu("Help");
  const shortcutsButton = helpMenu.getByRole("button", { name: "Shortcuts" });
  await expect(shortcutsButton).toBeVisible();
  await shortcutsButton.click();
  await expect(page.locator("#wolfbbsShortcutOverlay")).toHaveClass(/active/);
  await page.keyboard.press("Escape");
  await expect(page.locator("#wolfbbsShortcutOverlay")).not.toHaveClass(/active/);
  const sectionNav = page.locator(".wolfbbs-section-nav").first();
  if (await sectionNav.count()) {
    await expect(sectionNav).toBeVisible();
    await expect(sectionNav).toContainText("Jump to");
  }
  await expect(page.locator(".wolfbbs-section-nav-tools")).toHaveCount(0);
  await expect(page.locator(".wolfbbs-section-pin-button")).toHaveCount(0);
  await expect(page.locator(".wolfbbs-section-done-toggle")).toHaveCount(0);

  const helpMenuAgain = await openControlMenu("Help");
  await helpMenuAgain.getByRole("button", { name: "Bug report" }).click();
  await expect(page.locator("#wolfbbsBugOverlay")).toHaveClass(/active/);
  await expect(page.locator("#wolfbbsBugPayload")).toBeVisible();
  await page.locator("#wolfbbsBugClose").click();
  await expect(page.locator("#wolfbbsBugOverlay")).not.toHaveClass(/active/);

  await gotoHealthy(page, "/boards", /Message Boards/i);
  await expect(page.locator("body")).not.toContainText("Submit form");

  await gotoHealthy(page, "/directory", /Caller Directory/i);
  const stickyToggle = page.getByRole("button", { name: /Sticky head:/i }).first();
  await expect(stickyToggle).toBeVisible();
  await stickyToggle.click();
  await expect(page.locator(".wolfbbs-table-wrap").first()).toHaveClass(/wolfbbs-sticky-head/);
  const rowInspector = page.locator(".wolfbbs-row-inspector").first();
  await expect(rowInspector).toBeVisible();
  await page.locator("table tr").nth(1).click();
  await expect(rowInspector).toContainText(/Row inspector/i);

  await gotoHealthy(page, "/feedback", /Feedback to Sysop/i);
  const progressMeter = page.locator(".wolfbbs-form-progress");
  if (await progressMeter.count()) {
    await expect(progressMeter.first()).toBeVisible();
  }
  await expect(page.locator("body")).not.toContainText("Form slot #");
});

test("extended admin routes and setup actions stay functional", async ({ browser }) => {
  test.setTimeout(240_000);
  const runID = Date.now().toString(36);
  const uiHandle = `ui${runID.slice(-6)}`;
  const uiPassword = "ui123456";
  const pluginID = `qa_plugin_${runID.slice(-6)}`;
  const themeID = `qa-theme-${runID.slice(-6)}`;
  const challengeName = `QA Challenge ${runID}`;
  const goalTitle = `QA Goal ${runID}`;
  const missionID = `qa-mission-${runID.slice(-6)}`;
  const missionTitle = `QA Mission ${runID}`;

  const adminCtx = await browser.newContext();
  const adminPage = await adminCtx.newPage();
  await login(adminPage, ADMIN_HANDLE, ADMIN_PASSWORD, "/admin/login");
  await expect(adminPage).toHaveURL(/\/admin$/);

  await gotoHealthy(adminPage, "/admin/setup", /Setup & Install/i);
  await adminPage.getByRole("button", { name: "Seed Default Boards" }).click();
  await expect(adminPage.locator("body")).toContainText(/Seeded \d+ default board\(s\)\.|Default boards already present\./);

  await gotoHealthy(adminPage, "/admin/users", /Sysop Users/i);
  const createUserForm = adminPage.locator('form:has(input[name="action"][value="create"])').first();
  await createUserForm.locator('input[name="handle"]').fill(uiHandle);
  await createUserForm.locator('input[name="password"]').fill(uiPassword);
  await createUserForm.locator('select[name="role"]').selectOption("user");
  await createUserForm.getByRole("button", { name: "Create" }).click();
  await expect(adminPage.locator("body")).toContainText(`Created user ${uiHandle}.`);
  await expect(adminPage.locator("body")).toContainText("One-time credential receipts");
  await expect(adminPage.locator("body")).toContainText(uiPassword);

  await gotoHealthy(adminPage, "/admin/mentorship", /Mentorship Pairing Admin/i);
  await adminPage.fill('input[name="mentee"]', uiHandle);
  await adminPage.fill('input[name="mentor"]', ADMIN_HANDLE);
  await adminPage.fill('input[name="note"]', "QA mentorship lane");
  await adminPage.getByRole("button", { name: "Save Pair" }).click();
  await expect(adminPage.locator("body")).toContainText("Mentorship pair saved.");

  await gotoHealthy(adminPage, "/admin/plugins", /Plugin Manifest Contracts/i);
  await adminPage.fill('input[name="id"]', pluginID);
  await adminPage.fill('input[name="entrypoint"]', `/app/plugins/${pluginID}/entrypoint.sh`);
  await adminPage.fill('input[name="capabilities"]', "board.read,chat.read");
  await adminPage.selectOption('select[name="sandbox_profile"]', "strict");
  await adminPage.fill('input[name="retention_days"]', "30");
  await adminPage.getByRole("button", { name: "Validate + Save" }).click();
  await expect(adminPage.locator("body")).toContainText("Plugin manifest saved.");
  await expect(adminPage.locator("body")).toContainText(pluginID);

  const starterRes = await adminPage.request.get(`/admin/plugins/starter?id=${encodeURIComponent(pluginID)}`);
  expect(starterRes.status()).toBe(200);
  expect((starterRes.headers()["content-type"] || "").toLowerCase()).toContain("application/zip");

  await gotoHealthy(adminPage, "/admin/themes", /Theme Marketplace \/ Import/i);
  const themeBundle = {
    manifest: {
      id: themeID,
      name: "QA Theme",
      version: "1.0.0",
      author: "playwright",
      description: "qa bundle",
    },
    themes: [
      {
        name: `${themeID}-main`,
        status_fg: "fg-white",
        status_bg: "bg-blue",
        body_fg: "fg-white",
        accent_fg: "fg-cyan",
        warn_fg: "fg-yellow",
        error_fg: "fg-red",
        muted_fg: "fg-green",
      },
    ],
  };
  await adminPage.fill('textarea[name="bundle_json"]', JSON.stringify(themeBundle));
  await adminPage.getByRole("button", { name: "Import + Validate" }).click();
  await expect(adminPage.locator("body")).toContainText("Theme bundle imported and validated.");
  await expect(adminPage.locator("body")).toContainText(themeID);
  await adminPage.locator("tr", { hasText: themeID }).first().getByRole("button", { name: "Apply" }).click();
  await expect(adminPage.locator("body")).toContainText("Theme bundle applied.");

  await gotoHealthy(adminPage, "/admin/webhooks", /External Webhook Bridge/i);
  await adminPage.fill('input[name="endpoint"]', "https://example.invalid/wolfbbs-hook");
  await adminPage.fill('input[name="events"]', "board.created,message.posted");
  await adminPage.fill('input[name="retry_attempts"]', "2");
  await adminPage.fill('input[name="backoff_ms"]', "200");
  await adminPage.fill('input[name="timeout_seconds"]', "2");
  await adminPage.getByRole("button", { name: "Save Bridge" }).click();
  await expect(adminPage.locator("body")).toContainText("Webhook bridge settings saved.");
  await adminPage.getByRole("button", { name: "Send Test Event" }).click();
  await expect(adminPage.locator("body")).toContainText("Webhook test event dispatched.");

  await gotoHealthy(adminPage, "/admin/challenges", /Challenges & Clubhouse Goals/i);
  await adminPage.fill('input[name="name"]', challengeName);
  await adminPage.fill('input[name="theme"]', "QA Run");
  await adminPage.fill('input[name="starts_at"]', datetimeLocalMinutes(-10));
  await adminPage.fill('input[name="ends_at"]', datetimeLocalMinutes(24 * 60));
  await adminPage.fill('input[name="board_weight"]', "3");
  await adminPage.fill('input[name="chat_weight"]', "2");
  await adminPage.fill('input[name="door_weight"]', "2");
  await adminPage.fill('form[action="/admin/challenges"] input[name="description"]', "QA challenge description");
  await adminPage.getByRole("button", { name: "Save Challenge" }).click();
  await expect(adminPage.locator("body")).toContainText("Seasonal challenge saved.");
  await expect(adminPage.locator("body")).toContainText(challengeName);

  await adminPage.fill('input[name="title"]', goalTitle);
  await adminPage.fill('input[name="target"]', "25");
  await adminPage.fill('input[name="progress"]', "5");
  await adminPage.locator('form[action="/admin/challenges"] input[name="description"]').nth(1).fill("QA goal description");
  await adminPage.getByRole("button", { name: "Save Goal" }).click();
  await expect(adminPage.locator("body")).toContainText("Clubhouse goal saved.");
  await expect(adminPage.locator("body")).toContainText(goalTitle);

  await gotoHealthy(adminPage, "/admin/missions", /Seasonal Missions Admin/i);
  await adminPage.fill('input[name="id"]', missionID);
  await adminPage.fill('input[name="season"]', "QA Season");
  await adminPage.fill('input[name="title"]', missionTitle);
  await adminPage.fill('input[name="description"]', "QA mission description");
  await adminPage.fill('input[name="starts_at"]', datetimeLocalMinutes(-10));
  await adminPage.fill('input[name="ends_at"]', datetimeLocalMinutes(24 * 60));
  await adminPage.fill('input[name="target_board_posts"]', "1");
  await adminPage.fill('input[name="target_chat_posts"]', "1");
  await adminPage.fill('input[name="target_door_runs"]', "1");
  await adminPage.check('input[name="active"]');
  await adminPage.getByRole("button", { name: "Save Mission" }).click();
  await expect(adminPage.locator("body")).toContainText("Mission saved.");
  await expect(adminPage.locator("body")).toContainText(missionTitle);

  await gotoHealthy(adminPage, "/admin/analytics", /Embedded Product Analytics/i);
  await gotoHealthy(adminPage, "/admin/mod-center", /Moderation Center/i);
  const cannedID = `qa_warn_${runID.slice(-6)}`;
  const cannedForm = adminPage.locator('form:has(input[name="action"][value="save_canned"])').first();
  await cannedForm.locator('input[name="id"]').fill(cannedID);
  await cannedForm.locator('input[name="category"]').fill("qa");
  await cannedForm.locator('input[name="title"]').fill(`QA Reminder ${runID}`);
  await cannedForm.locator('textarea[name="body"]').fill("Please keep QA chatter concise.");
  await cannedForm.getByRole("button", { name: "Save Template" }).click();
  await expect(adminPage.locator("body")).toContainText("Canned response saved.");

  const caseForm = adminPage.locator('form:has(input[name="action"][value="save_case"])').first();
  await caseForm.locator('input[name="title"]').fill(`QA Case ${runID}`);
  await caseForm.locator('select[name="status"]').selectOption("open");
  await caseForm.locator('select[name="priority"]').selectOption("high");
  await caseForm.locator('input[name="owner"]').fill(ADMIN_HANDLE);
  await caseForm.locator('input[name="target_handle"]').fill(uiHandle);
  await caseForm.locator('input[name="summary"]').fill("Case workflow QA coverage.");
  await caseForm.getByRole("button", { name: "Save Case Thread" }).click();
  await expect(adminPage.locator("body")).toContainText("Case thread saved.");

  await gotoHealthy(adminPage, "/admin/upgrade-safety", /Upgrade Safety Dashboard/i);
  await gotoHealthy(adminPage, "/admin/backups", /Backup Browser/i);
  await gotoHealthy(adminPage, "/admin/release", /Release Dashboard/i);
  await adminPage.locator('form[action="/admin/release"]:has(input[name="action"][value="toggle_check"]) button').first().click();
  await expect(adminPage.locator("body")).toContainText("Release checklist updated.");

  const userCtx = await browser.newContext();
  const userPage = await userCtx.newPage();
  await login(userPage, uiHandle, uiPassword);
  await expect(userPage).toHaveURL(/\/(boards|today)$/);
  await gotoHealthy(userPage, "/mentorship", /Mentorship Pairing/i);
  await userPage.fill('textarea[name="message"]', "QA mentorship ping");
  await userPage.getByRole("button", { name: "Send Check-In" }).click();
  await expect(userPage.locator("body")).toContainText("Mentor check-in sent.");
  await gotoHealthy(userPage, "/resume", /Smart Re-entry/i);
  await gotoHealthy(userPage, "/doors/comeback", /Door Comeback Prompts/i);
  await gotoHealthy(userPage, "/milestones", /Milestone Celebrations/i);
  await gotoHealthy(userPage, "/time-lane", /Time-of-Day Landing States/i);
  await gotoHealthy(userPage, "/challenges", /Seasonal Challenges/i);
  await gotoHealthy(userPage, "/missions", /Seasonal Missions/i);
  await userCtx.close();
  await adminCtx.close();
});
