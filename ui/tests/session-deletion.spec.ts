import { expect, test, type Page } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';
import { shortID } from '../src/lib/atoms/format';

async function openSessions(page: Page) {
  const navigationButton = page.getByRole('button', { name: 'Open navigation', exact: true });
  if (await navigationButton.isVisible()) await navigationButton.click();
  const navigation = page.getByRole('navigation', { name: 'Workspaces and sessions' });
  const selectedWorkspace = navigation.locator('[data-workspace-id][aria-current="page"]');
  if (await selectedWorkspace.isVisible()) await selectedWorkspace.click();
  await expect(navigation.getByRole('heading', { name: 'Sessions', exact: true })).toBeVisible();
  return navigation;
}

async function openDelete(page: Page, sessionID: string) {
  const navigation = await openSessions(page);
  const button = navigation.getByRole('button', {
    name: `Delete session ${shortID(sessionID)}`,
    exact: true,
  });
  await button.hover();
  await button.click();
  return page.getByRole('dialog', { name: `Delete session ${shortID(sessionID)}?`, exact: true });
}

test('delete an old conversation and children without disturbing the open draft', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const current = fixture.sessions[0];
  const old = { ...current, ID: 'older-session', CreatedAt: '2020-01-01T00:00:00Z' };
  const child = { ...old, ID: 'older-child', Parent: old.ID, Completed: true };
  fixture.sessions.push(old, child);
  fixture.histories.set(old.ID, []);
  fixture.histories.set(child.ID, []);
  await page.reload();
  const input = page.getByLabel('Message', { exact: true });
  await expect(input).toBeEnabled();
  await input.fill('Keep this draft');
  let dialog = await openDelete(page, old.ID);
  await expect(dialog).toContainText('child sessions');
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
  expect(fixture.sessions).toHaveLength(3);
  await expect(input).toHaveValue('Keep this draft');
  dialog = await openDelete(page, old.ID);
  await page.screenshot({ path: testInfo.outputPath('delete-session.png'), fullPage: true });
  await dialog.getByRole('button', { name: 'Delete session', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(fixture.sessions.map((entry) => entry.ID)).toEqual([current.ID]);
  expect(fixture.histories.has(old.ID)).toBe(false);
  expect(fixture.histories.has(child.ID)).toBe(false);
  await expect(input).toHaveValue('Keep this draft');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.reload();
  await expect(input).toBeEnabled();
  await openSessions(page);
  await expect(
    page.getByRole('button', { name: `Delete session ${shortID(old.ID)}`, exact: true }),
  ).toHaveCount(0);
});

test('deleting the current last session clears the conversation', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  await page.getByLabel('Message', { exact: true }).fill('Saved conversation');
  await page.getByLabel('Message', { exact: true }).press('Enter');
  await expect(page.getByRole('article', { name: 'assistant message' })).toContainText(
    'Saved conversation',
  );
  const dialog = await openDelete(page, session.ID);
  await dialog.getByRole('button', { name: 'Delete session', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByLabel('Message', { exact: true })).toBeDisabled();
  await expect(page.getByRole('article', { name: 'assistant message' })).toHaveCount(0);
  expect(fixture.sessions).toHaveLength(0);
  await page.reload();
  await expect(
    page
      .getByRole('region', { name: 'Conversation' })
      .getByRole('button', { name: 'New session', exact: true }),
  ).toBeVisible();
});

test('show a busy-session error without removing its history', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  await page.getByLabel('Message', { exact: true }).fill('wait');
  await page.getByLabel('Message', { exact: true }).press('Enter');
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeVisible();
  const dialog = await openDelete(page, fixture.sessions[0].ID);
  await dialog.getByRole('button', { name: 'Delete session', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('stop it before deleting');
  expect(fixture.sessions).toHaveLength(1);
  expect(fixture.histories.get(fixture.sessions[0].ID)).toHaveLength(1);
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeVisible();
});

test('retain the session after a deletion API failure', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  await page.route(`**/api/sessions/${session.ID}`, async (route) => {
    if (route.request().method() !== 'DELETE') return route.fallback();
    await route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'Deletion storage is unavailable' }),
    });
  });
  const dialog = await openDelete(page, session.ID);
  await dialog.getByRole('button', { name: 'Delete session', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('Deletion storage is unavailable');
  expect(fixture.sessions).toHaveLength(1);
  await expect(dialog.getByRole('button', { name: 'Delete session', exact: true })).toBeEnabled();
});
