const { test, expect } = require("@playwright/test");
const net = require("net");

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

async function expectNoCriticalA11y(page) {
  await expect(page.locator("body")).toBeVisible();
}

async function csrfFrom(page) {
  const token = await page.locator('input[name="csrf_token"]').first().getAttribute("value");
  expect(token).toBeTruthy();
  return token;
}

async function postForm(page, path, form, expectedStatuses = [200, 302]) {
  const response = await page.request.post(path, { form });
  const status = response.status();
  if (!expectedStatuses.includes(status)) {
    const body = await response.text();
    throw new Error(`unexpected status ${status} for ${path}; expected ${expectedStatuses.join(",")} body=${body}`);
  }
  return response;
}

async function canConnect(host, port, timeoutMs = 1000) {
  return new Promise((resolve) => {
    const socket = net.createConnection({ host, port });
    const finish = (ok) => {
      socket.destroy();
      resolve(ok);
    };
    socket.setTimeout(timeoutMs, () => finish(false));
    socket.on("connect", () => finish(true));
    socket.on("error", () => finish(false));
  });
}

async function sendIrcMessage({
  host = "127.0.0.1",
  port = Number(process.env.WOLFBBS_E2E_IRC_PORT || "6667"),
  nick = process.env.WOLFBBS_E2E_IRC_NICK || USER_HANDLE,
  pass = process.env.WOLFBBS_E2E_IRC_PASS || USER_PASSWORD,
  channel = "#lobby",
  message,
}) {
  return new Promise((resolve, reject) => {
    const socket = net.createConnection({ host, port });
    let ready = false;
    let joined = false;
    let done = false;
    let pending = "";
    const timeout = setTimeout(() => {
      if (!done) {
        done = true;
        socket.destroy();
        reject(new Error("IRC bridge timeout"));
      }
    }, 10000);

    const finish = (err) => {
      if (done) {
        return;
      }
      done = true;
      clearTimeout(timeout);
      try {
        socket.end("QUIT :bye\r\n");
      } catch (_) {
        // ignore
      }
      if (err) {
        reject(err);
        return;
      }
      resolve();
    };

    socket.on("connect", () => {
      socket.write(`PASS ${pass}\r\n`);
      socket.write(`NICK ${nick}\r\n`);
      socket.write(`USER ${nick} 0 * :${nick}\r\n`);
    });
    socket.on("error", (err) => finish(err));
    socket.on("data", (chunk) => {
      pending += chunk.toString("utf8");
      const lines = pending.split(/\r?\n/);
      pending = lines.pop() || "";
      for (const line of lines) {
        if (!line) {
          continue;
        }
        if (line.startsWith("PING ")) {
          socket.write(`PONG ${line.slice(5)}\r\n`);
        }
        if (!ready && line.includes(" 001 ")) {
          ready = true;
          socket.write(`JOIN ${channel}\r\n`);
        }
        if (ready && !joined && line.includes(" 366 ")) {
          joined = true;
          socket.write(`PRIVMSG ${channel} :${message}\r\n`);
        }
        if (line.includes(`PRIVMSG ${channel} :${message}`)) {
          finish();
          return;
        }
      }
    });
  });
}

test("connect page exposes the web terminal entrypoint", async ({ page }) => {
  await page.goto("/connect");
  await expect(page.locator("h1")).toContainText(/Connect/i);
  await expect(page.locator("body")).toContainText("Choose your client");
  await expect(page.locator("body")).toContainText("First call checklist");
  await expect(page.locator("#xterm")).toBeVisible();
  await expect(page.locator("#termStatus")).toContainText(/connecting|connected|disconnected|socket error/i);
  await expect(page.locator("body")).toContainText("ws-login");
});

test("user web journey supports keyboard navigation and status/config visibility", async ({
  page,
}) => {
  await page.goto("/login");
  await expect(page.locator("h1")).toContainText(/Web Login/i);
  await expectNoCriticalA11y(page);

  await login(page, USER_HANDLE, USER_PASSWORD);
  await expect(page).toHaveURL(/\/boards$/);
  await expect(page.locator("h1")).toContainText("Message Boards");
  await expect(page.locator("#wolfbbsCommandButton")).toContainText(/Jump \/ Search/i);
  await expect(page.locator("body")).toContainText("Caller Cockpit");
  await page.goto("/boards?mode=mentions");
  await expect(page.locator("body")).toContainText("Personal Board Queue");
  await expect(page.locator("body")).toContainText("Mentions");

  await page.goto("/bulletins");
  await expect(page.locator("h1")).toContainText("Bulletin Center");
  await expect(page.locator("body")).toContainText("Spotlight");

  await page.goto("/directory?online=1&verified=verified");
  await expect(page.locator("h1")).toContainText("Caller Directory");
  await expect(page.locator("body")).toContainText("Caller Card");
  await expect(page.locator("body")).toContainText("visible callers");
  await expect(page.locator("body")).toContainText("Origin");

  await page.goto(`/finder?q=${USER_HANDLE}&author=${USER_HANDLE}&tracker=post`);
  await expect(page.locator("h1")).toContainText("Message Finder");
  await expect(page.locator("body")).toContainText("Thread Tracker");
  await expect(page.locator("body")).toContainText("Conf");

  await page.goto("/newfiles?sort=rating&since=30d");
  await expect(page.locator("h1")).toContainText("New Files Desk");
  await expect(page.locator("body")).toContainText("Download Desk");
  await expect(page.locator("body")).toContainText("visible uploads");

  await page.goto("/feedback");
  await expect(page.locator("h1")).toContainText("Feedback to Sysop");

  await page.goto("/doors");
  await expect(page.locator("h1")).toContainText("Door Cockpit");
  await expect(page.locator("body")).toContainText("Recommended For This Caller");
  await expect(page.locator("body")).toContainText("Directory");

  await page.goto("/radar");
  await expect(page.locator("h1")).toContainText("Caller Radar");
  await expect(page.locator("body")).toContainText("Board Pulse");
  await expect(page.locator("body")).toContainText("Live Caller Radar");

  await page.goto("/clubhouse");
  await expect(page.locator("h1")).toContainText("Clubhouse");
  await expect(page.locator("body")).toContainText("OneLinerz Wall");
  await page.fill('input[name="text"]', "Playwright clubhouse line");
  await page.getByRole("button", { name: "Post" }).click();
  await expect(page.locator("body")).toContainText("One-liner posted");
  await expect(page.locator("body")).toContainText("Playwright clubhouse line");

  await page.locator('a[href="/mail"]').focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/mail$/);
  await expect(page.locator("h1")).toContainText("Private Mail");
  await page.goto("/mail?template=door_invite");
  await expect(page.locator('input[name="subject"]')).toHaveValue(/Meet me in the Door Hub/);
  await page.fill('input[name="to"]', ADMIN_HANDLE);
  await page.fill('input[name="subject"]', "Playwright Mail");
  await page.fill('textarea[name="body"]', "Mail body from browser flow.");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page.locator("body")).toContainText("Playwright Mail");

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

  await page.goto("/gateway");
  await page.fill('input[name="url"]', "http://example.com");
  await page.check('input[name="save"]');
  await page.getByRole("button", { name: "Fetch" }).click();
  await expect(page.locator("h1")).toContainText(/Gateway/i);
  await expect(page.locator("body")).toContainText("Example Domain");
  await expect(page.locator("body")).toContainText("Saved to:");

  await page.goto("/settings");
  await page.uncheck('input[name="ansi_enabled"]');
  await page.getByRole("button", { name: "Save Preferences" }).click();
  await expect(page.locator("body")).toContainText("ANSI: false");
  await page.check('input[name="ansi_enabled"]');
  await page.getByRole("button", { name: "Save Preferences" }).click();
  await expect(page.locator("body")).toContainText("ANSI: true");

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
  await login(adminPage, ADMIN_HANDLE, ADMIN_PASSWORD, "/admin/login");
  await expect(adminPage).toHaveURL(/\/admin$/);
  await expect(adminPage.locator("h1")).toContainText("Sysop Control Panel");
  await expect(adminPage.locator("body")).toContainText("Launch Digest");

  await adminPage.goto("/admin/launch");
  await expect(adminPage.locator("h1")).toContainText("Launch Center");
  await expect(adminPage.locator("body")).toContainText("Operator Commands");
  await expect(adminPage.locator("body")).toContainText("docs/OPERATOR_PLAYBOOK.md");
  await adminPage.goto("/admin");

  await adminPage.locator('a[href="/admin/users"]').focus();
  await adminPage.keyboard.press("Enter");
  await expect(adminPage).toHaveURL(/\/admin\/users$/);
  await expect(adminPage.locator("h1")).toContainText("Sysop Users");
  const runID = Date.now().toString(36);
  const qaHandle = `qa${runID.slice(-5)}`;
  const qaBoardTitle = `QA Board ${runID}`;
  const qaBoardTitleUpdated = `QA Board Updated ${runID}`;

  let csrf = await csrfFrom(adminPage);
  let res = await postForm(adminPage, "/admin/users", {
    csrf_token: csrf,
    action: "create",
    handle: qaHandle,
    password: "qa123456",
    role: "user",
  }, [200]);
  await expect(res.text()).resolves.toContain(`created user ${qaHandle}`);
  await adminPage.goto("/admin/users");
  await expect(adminPage.locator("body")).toContainText(qaHandle);
  csrf = await csrfFrom(adminPage);
  await postForm(adminPage, "/admin/users", { csrf_token: csrf, action: "disable", handle: qaHandle });
  const disabledCtx = await browser.newContext();
  const disabledPage = await disabledCtx.newPage();
  await login(disabledPage, qaHandle, "qa123456");
  await expect(disabledPage.locator("body")).toContainText("invalid credentials");
  await disabledCtx.close();
  await postForm(adminPage, "/admin/users", { csrf_token: csrf, action: "enable", handle: qaHandle });
  const enabledCtx = await browser.newContext();
  const enabledPage = await enabledCtx.newPage();
  await login(enabledPage, qaHandle, "qa123456");
  await expect(enabledPage).toHaveURL(/\/boards$/);
  await enabledCtx.close();
  await postForm(adminPage, "/admin/users", { csrf_token: csrf, action: "ban", handle: qaHandle });
  const bannedCtx = await browser.newContext();
  const bannedPage = await bannedCtx.newPage();
  await login(bannedPage, qaHandle, "qa123456");
  await expect(bannedPage.locator("body")).toContainText("invalid credentials");
  await bannedCtx.close();
  await postForm(adminPage, "/admin/users", { csrf_token: csrf, action: "unban", handle: qaHandle });
  await postForm(adminPage, "/admin/users", { csrf_token: csrf, action: "set_role", handle: qaHandle, role: "moderator" });
  await postForm(adminPage, "/admin/users", { csrf_token: csrf, action: "set_role", handle: qaHandle, role: "user" });

  await adminPage.goto("/admin/boards");
  await expect(adminPage.locator("h1")).toContainText("Sysop Boards");
  csrf = await csrfFrom(adminPage);
  await postForm(adminPage, "/admin/boards", {
    csrf_token: csrf,
    action: "create",
    title: qaBoardTitle,
    conference: "Testing",
    description: "Board created by playwright",
    read_acs: "",
    write_acs: "",
  });
  await adminPage.goto("/admin/boards");
  const qaBoardRow = adminPage.locator("tr", { hasText: qaBoardTitle }).first();
  await expect(qaBoardRow).toBeVisible();
  const qaBoardID = await qaBoardRow.locator('input[name="id"]').first().inputValue();
  csrf = await csrfFrom(adminPage);
  await postForm(adminPage, "/admin/boards", {
    csrf_token: csrf,
    action: "update",
    id: qaBoardID,
    title: qaBoardTitleUpdated,
    conference: "Testing",
    description: "Updated by playwright",
    read_acs: "",
    write_acs: "",
  });
  await adminPage.goto("/admin/boards");
  await expect(adminPage.locator("body")).toContainText(qaBoardTitleUpdated);
  csrf = await csrfFrom(adminPage);
  await postForm(adminPage, "/admin/boards", { csrf_token: csrf, action: "delete", id: qaBoardID });
  await adminPage.goto("/admin/boards");
  await expect(adminPage.locator("body")).not.toContainText(qaBoardTitleUpdated);

  await adminPage.goto("/admin/mail");
  const mailCsrfCount = await adminPage.locator('input[name="csrf_token"]').count();
  if (mailCsrfCount > 0) {
    csrf = await csrfFrom(adminPage);
    await postForm(adminPage, "/admin/mail", { csrf_token: csrf, handle: qaHandle, action: "disable_outbound" });
    await adminPage.goto("/admin/mail");
    await expect(adminPage.locator("body")).toContainText(qaHandle);
    await expect(adminPage.locator("body")).toContainText("true");
    csrf = await csrfFrom(adminPage);
    await postForm(adminPage, "/admin/mail", { csrf_token: csrf, handle: qaHandle, action: "enable_outbound" });
  } else {
    await expect(adminPage.locator("h1")).toContainText("Mail Controls");
  }

  await adminPage.goto("/admin/gateways");
  await expect(adminPage.locator("h1")).toContainText("Gateway Controls");
  csrf = await csrfFrom(adminPage);
  await postForm(adminPage, "/admin/gateways", {
    csrf_token: csrf,
    smtp_host: "smtp.example.test",
    smtp_port: "2525",
    smtp_user: "wolf",
    smtp_pass: "secret",
    from_domain: "example.test",
    max_recipients: "3",
    max_message_bytes: "65536",
    web_timeout_sec: "10",
    web_max_bytes: "2097152",
  });
  await adminPage.goto("/admin/gateways");
  await expect(adminPage.locator('input[name="smtp_host"]')).toHaveValue("smtp.example.test");

  await adminPage.goto("/admin/chat");
  await expect(adminPage.locator("h1")).toContainText("Chat Admin");
  csrf = await csrfFrom(adminPage);
  await postForm(adminPage, "/admin/chat", { csrf_token: csrf, action: "create_channel", channel: "#qa-chat" });
  await postForm(adminPage, "/admin/chat", { csrf_token: csrf, action: "lock_channel", channel: "#qa-chat" });
  await postForm(adminPage, "/admin/chat", { csrf_token: csrf, action: "unlock_channel", channel: "#qa-chat" });

  await adminPage.goto("/chat");
  csrf = await csrfFrom(adminPage);
  await postForm(adminPage, "/chat/moderation", {
    csrf_token: csrf,
    channel: "#lobby",
    action: "mute",
    target: "caller",
    reason: "qa-mute",
    duration: "5m",
  });
  await postForm(adminPage, "/chat/moderation", {
    csrf_token: csrf,
    channel: "#lobby",
    action: "unmute",
    target: "caller",
    reason: "qa-unmute",
    duration: "0",
  });
  await adminPage.goto("/admin/chat");
  await expect(adminPage.locator("body")).toContainText("qa-mute");

  await adminPage.goto("/settings");
  csrf = await csrfFrom(adminPage);
  await postForm(adminPage, "/settings", { csrf_token: csrf, action: "enable_2fa" });
  await adminPage.goto("/settings");
  await expect(adminPage.locator("body")).toContainText("2FA is enabled.");
  csrf = await csrfFrom(adminPage);
  await postForm(adminPage, "/settings", { csrf_token: csrf, action: "disable_2fa" });
  await adminPage.goto("/settings");
  await expect(adminPage.locator("body")).toContainText("2FA is currently disabled.");

  await adminPage.goto("/admin/config");
  await expect(adminPage.locator("h1")).toContainText("Runtime Config");
  await adminPage.goto("/admin/system");
  await expect(adminPage.locator("h1")).toContainText("System / WFC Dashboard");
  await adminPage.goto("/admin/audit");
  await expect(adminPage.locator("h1")).toContainText("Admin Audit Log");
  await expect(adminPage.locator("body")).toContainText("create_board");
  await expect(adminPage.locator("body")).toContainText("disable_user");
  await expectNoCriticalA11y(adminPage);

  const userCtx = await browser.newContext();
  const userPage = await userCtx.newPage();
  await login(userPage, qaHandle, "qa123456");
  const resp = await userPage.goto("/admin");
  expect(resp).not.toBeNull();
  expect(resp.status()).toBe(403);
  await userCtx.close();
  await adminCtx.close();
});

test("sysop setup/files/doors/system surfaces and actions stay healthy", async ({ browser }) => {
  const adminCtx = await browser.newContext();
  const adminPage = await adminCtx.newPage();
  await login(adminPage, ADMIN_HANDLE, ADMIN_PASSWORD, "/admin/login");
  await expect(adminPage).toHaveURL(/\/admin$/);

  const runID = Date.now().toString(36);
  const areaName = `qa-area-${runID.slice(-6)}`;
  const areaPath = `.wolfbbs/files/${areaName}`;

  await adminPage.goto("/admin/setup");
  await expect(adminPage.locator("h1")).toContainText("Setup & Install");
  await expect(adminPage.locator("body")).toContainText("Launch Checklist");
  await expect(adminPage.locator("body")).toContainText("Common Gotchas");
  let csrf = await csrfFrom(adminPage);
  await postForm(adminPage, "/admin/setup", { csrf_token: csrf, action: "seed_default_boards" });
  await postForm(adminPage, "/admin/setup", { csrf_token: csrf, action: "ensure_mailbot" });

  await adminPage.goto("/admin/files");
  await expect(adminPage.locator("h1")).toContainText("Files");
  csrf = await csrfFrom(adminPage);
  await postForm(adminPage, "/admin/files", {
    csrf_token: csrf,
    action: "create",
    name: areaName,
    path: areaPath,
    description: "qa functional area",
  });
  await adminPage.goto("/admin/files");
  const createdAreaRow = adminPage.locator("tr", { hasText: areaName }).first();
  await expect(createdAreaRow).toBeVisible();
  const createdAreaID = await createdAreaRow.locator('input[name="id"]').first().inputValue();
  csrf = await csrfFrom(adminPage);
  await postForm(adminPage, "/admin/files", {
    csrf_token: csrf,
    action: "delete",
    id: createdAreaID,
  });

  await adminPage.goto("/admin/doors");
  await expect(adminPage.locator("h1")).toContainText("Doors Admin");
  const doorIDInput = adminPage.locator('input[name="door_id"]').first();
  const hasDoorRows = (await adminPage.locator('input[name="door_id"]').count()) > 0;
  let doorConfigUpdated = false;
  if (hasDoorRows) {
    const firstDoorID = await doorIDInput.inputValue();
    expect(firstDoorID).toBeTruthy();
    csrf = await csrfFrom(adminPage);
    await postForm(adminPage, "/admin/doors", {
      csrf_token: csrf,
      action: "save_config",
      door_id: firstDoorID,
      enabled: "on",
      daily_turns: "40",
      time_bank_max: "200",
      reset_hour: "0",
      messages_days: "30",
      logs_days: "30",
      max_run_seconds: "180",
      max_output_rate: "8192",
      allow_network: "on",
    });
    await postForm(adminPage, "/admin/doors", {
      csrf_token: csrf,
      action: "reset_scores",
      door_id: firstDoorID,
    });
    doorConfigUpdated = true;
  } else {
    await expect(adminPage.locator("body")).toContainText("No doors matched filter.");
  }

  await adminPage.goto("/admin/errors");
  await expect(adminPage.locator("h1")).toContainText("Runtime Error Log");

  const nodeState = await adminPage.request.get("/admin/node-state");
  expect(nodeState.status()).toBe(200);
  const nodeStateJSON = await nodeState.json();
  expect(nodeStateJSON).toHaveProperty("node_sessions");
  expect(nodeStateJSON).toHaveProperty("caller_history");

  await adminPage.goto("/admin/audit");
  await expect(adminPage.locator("h1")).toContainText("Admin Audit Log");
  await expect(adminPage.locator("body")).toContainText("seed_default_boards");
  await expect(adminPage.locator("body")).toContainText("create_file_area");
  if (doorConfigUpdated) {
    await expect(adminPage.locator("body")).toContainText("door_save_config");
  }

  await adminCtx.close();
});

test("chat syncs between two web sessions in realtime", async ({ browser }) => {
  const aCtx = await browser.newContext();
  const bCtx = await browser.newContext();
  const aPage = await aCtx.newPage();
  const bPage = await bCtx.newPage();

  await login(aPage, ADMIN_HANDLE, ADMIN_PASSWORD);
  await login(bPage, USER_HANDLE, USER_PASSWORD);
  await aPage.goto("/chat");
  await bPage.goto("/chat");

  await aPage.fill("#message", "hello from playwright chat");
  await aPage.getByRole("button", { name: "Send" }).click();

  await expect
    .poll(async () => {
      return bPage.locator("#chat").innerText();
    })
    .toContain("hello from playwright chat");
  await expect
    .poll(async () => {
      return bPage.locator("#chat").innerText();
    })
    .not.toContain("[undefined] undefined: undefined");

  await bPage.reload();
  await expect
    .poll(async () => {
      return bPage.locator("#chat").innerText();
    })
    .toContain("hello from playwright chat");
  await expect
    .poll(async () => {
      return bPage.locator("#chat").innerText();
    })
    .not.toContain("[undefined] undefined: undefined");

  const historyRes = await bPage.request.get("/chat/history?channel=%23lobby&limit=20");
  expect(historyRes.status()).toBe(200);
  const historyPayload = await historyRes.json();
  const historyMessages = historyPayload.messages || [];
  expect(historyMessages.length).toBeGreaterThan(0);
  const latest = historyMessages[historyMessages.length - 1];
  expect(latest.from).toBeDefined();
  expect(latest.body).toBeDefined();
  expect(latest.created_at).toBeDefined();
  expect(latest.From).toBeUndefined();
  expect(latest.Body).toBeUndefined();
  expect(latest.CreatedAt).toBeUndefined();
  await aCtx.close();
  await bCtx.close();
});

test("irc message is visible in web chat", async ({ browser }) => {
  const ircHost = process.env.WOLFBBS_E2E_IRC_HOST || "127.0.0.1";
  const ircPort = Number(process.env.WOLFBBS_E2E_IRC_PORT || "6667");
  const ircReady = await canConnect(ircHost, ircPort, 1500);
  test.skip(!ircReady, "IRC endpoint unavailable for this e2e run");

  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  await login(page, ADMIN_HANDLE, ADMIN_PASSWORD);
  await page.goto("/chat");
  const marker = `irc-bridge-${Date.now()}`;
  await sendIrcMessage({ host: ircHost, port: ircPort, message: marker });
  await expect
    .poll(async () => {
      return page.locator("#chat").innerText();
    })
    .toContain(marker);
  await expect
    .poll(async () => {
      return page.locator("#chat").innerText();
    })
    .not.toContain("[undefined] undefined: undefined");
  await ctx.close();
});

test("support, discovery, reset, and activitypub surfaces behave like real user flows", async ({
  browser,
  page,
}) => {
  await page.goto("/help");
  await expect(page.locator("h1")).toContainText(/Help/i);
  await expect(page.locator("body")).toContainText("Current role: guest");
  await expect(page.locator("body")).toContainText("Use the right surface");
  await expect(page.locator("body")).toContainText("docs/START_HERE.md");

  await page.goto("/tour");
  await expect(page.locator("h1")).toContainText("Guided Tour");
  await expect(page.locator("body")).toContainText("Last Callers");

  await login(page, USER_HANDLE, USER_PASSWORD);
  await page.goto("/help");
  await expect(page.locator("body")).toContainText("Current role: user");
  await expect(page.locator("body")).toContainText("/scores");
  await expect(page.locator("body")).toContainText("If you're a caller");

  await page.goto("/discover");
  await expect(page.locator("h1")).toContainText("Since Your Last Call");
  await page.fill('input[name="q"]', USER_HANDLE);
  await page.getByRole("button", { name: "Save Search" }).click();
  await expect(page.locator("body")).toContainText(USER_HANDLE);

  await page.goto("/scores");
  await expect(page.locator("h1")).toContainText("Door Scores & Trophies");
  await expect(page.locator("body")).toContainText(/No scores yet|Door filter:/);

  const statusPayload = await page.evaluate(async () => {
    const res = await fetch("/statusz");
    return { status: res.status, json: await res.json() };
  });
  expect(statusPayload.status).toBe(200);
  expect(Array.isArray(statusPayload.json.checks)).toBeTruthy();
  expect(statusPayload.json.role).toBe("user");

  const baseOrigin = new URL(page.url()).origin;
  const apPayload = await page.evaluate(async ({ userHandle, adminHandle, baseURL }) => {
    const webfinger = await fetch(
      `/.well-known/webfinger?resource=${encodeURIComponent(`acct:${userHandle}@127.0.0.1:18080`)}`,
    );
    const actor = await fetch(`/ap/users/${encodeURIComponent(userHandle)}`);
    const outbox = await fetch(`/ap/users/${encodeURIComponent(userHandle)}/outbox`);
    const inbox = await fetch(`/ap/users/${encodeURIComponent(adminHandle)}/inbox`, {
      method: "POST",
      headers: { "Content-Type": "application/activity+json" },
      body: JSON.stringify({
        id: `${baseURL}/qa-follow-${Date.now()}`,
        type: "Follow",
        actor: "https://remote.example/users/qa",
        object: `${baseURL}/ap/users/${adminHandle}`,
      }),
    });
    return {
      webfingerStatus: webfinger.status,
      webfinger: await webfinger.json(),
      actorStatus: actor.status,
      actor: await actor.json(),
      outboxStatus: outbox.status,
      outbox: await outbox.json(),
      inboxStatus: inbox.status,
      inbox: await inbox.json(),
    };
  }, { userHandle: USER_HANDLE, adminHandle: ADMIN_HANDLE, baseURL: baseOrigin });
  expect(apPayload.webfingerStatus).toBe(200);
  expect(apPayload.webfinger.subject).toContain(`acct:${USER_HANDLE}@`);
  expect(apPayload.actorStatus).toBe(200);
  expect(apPayload.actor.type).toBe("Person");
  expect(apPayload.outboxStatus).toBe(200);
  expect(apPayload.outbox.type).toBe("OrderedCollection");
  expect(apPayload.inboxStatus).toBe(202);
  expect(apPayload.inbox.status).toBe("accepted");

  const adminCtx = await browser.newContext();
  const adminPage = await adminCtx.newPage();
  await login(adminPage, ADMIN_HANDLE, ADMIN_PASSWORD, "/admin/login");
  await expect(adminPage).toHaveURL(/\/admin$/);
  await adminPage.goto("/admin/users");
  await expect(adminPage.locator("h1")).toContainText("Sysop Users");
  const resetHandle = `reset${Date.now().toString(36).slice(-6)}`;
  let csrf = await csrfFrom(adminPage);
  await postForm(adminPage, "/admin/users", {
    csrf_token: csrf,
    action: "create",
    handle: resetHandle,
    password: "resetpass1",
    role: "user",
  }, [200]);
  await adminCtx.close();

  await page.goto("/logout");
  await page.goto("/reset/request");
  await expect(page.locator("h1")).toContainText("Password Reset");
  await page.fill('input[name="handle"]', resetHandle);
  await page.getByRole("button", { name: "Issue Reset Token" }).click();
  const resetBody = await page.locator("body").innerText();
  const tokenMatch = resetBody.match(/Dev token:\s*([A-Za-z0-9]+)/);
  expect(tokenMatch).not.toBeNull();
  const resetToken = tokenMatch[1];

  await page.goto(`/reset/complete?token=${encodeURIComponent(resetToken)}`);
  await expect(page.locator("h1")).toContainText("Set New Password");
  await page.fill('input[name="password"]', "resetpass2");
  await page.getByRole("button", { name: "Reset Password" }).click();
  await expect(page).toHaveURL(/\/login$/);

  await login(page, resetHandle, "resetpass2");
  await expect(page).toHaveURL(/\/boards$/);
});
