const { test, expect } = require('@playwright/test');

const USER_HANDLE = process.env.WOLFBBS_E2E_USER_HANDLE || 'caller';
const USER_PASSWORD = process.env.WOLFBBS_E2E_USER_PASSWORD || 'password123';

async function login(page, handle, password) {
  await page.goto('/login');
  await expect(page.locator('h1')).toContainText(/Web Login/i);
  await page.fill('input[name="handle"]', handle);
  await page.fill('input[name="password"]', password);
  await page.click('button[type="submit"]');
}

test('seeded first-call journey behaves like a believable new caller loop', async ({ page }) => {
  await login(page, USER_HANDLE, USER_PASSWORD);
  await expect(page).toHaveURL(/\/(boards|today|start)/);

  const runID = Date.now().toString(36);
  const boardSubject = `Journey Board Post ${runID}`;
  const lobbyLine = `Journey lobby line ${runID}`;
  const mailSubject = `Journey Mail ${runID}`;

  await page.goto('/first-call');
  await expect(page.locator('body')).toContainText('Starter Board Post');
  await expect(page.locator('body')).toContainText('Lobby Hello');
  await expect(page.locator('body')).toContainText('Private Mail Check');

  await page.fill('form[action="/first-call"] input[name="subject"]', boardSubject);
  await page.locator('form[action="/first-call"] textarea[name="body"]').first().fill('End-user journey board body.');
  await page.getByRole('button', { name: 'Create starter post' }).click();
  await expect(page.locator('body')).toContainText('Starter board post created');

  await page.fill('form[action="/first-call"] input[name="body"]', lobbyLine);
  await page.getByRole('button', { name: 'Send lobby message' }).click();
  await expect(page.locator('body')).toContainText('Starter lobby message sent');

  await page.locator('form[action="/first-call"] input[name="subject"]').nth(1).fill(mailSubject);
  await page.locator('form[action="/first-call"] textarea[name="body"]').nth(1).fill('End-user journey private mail body.');
  await page.getByRole('button', { name: 'Send private mail' }).click();
  await expect(page.locator('body')).toContainText('Starter private mail sent');

  await page.goto('/boards');
  await expect(page.locator('body')).toContainText(boardSubject);

  await page.goto('/chat');
  await expect(page.locator('#chat')).toContainText(lobbyLine);

  await page.goto('/mail');
  await expect(page.locator('body')).toContainText(mailSubject);

  await page.goto('/doors');
  await expect(page.locator('body')).toContainText(/Door|Games/i);

  await page.goto('/today');
  await expect(page.locator('body')).toContainText('Today Brief');
});
