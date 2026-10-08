import { expect, test, type Page } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';
import { shortID } from '../src/lib/atoms/format';

async function openNavigation(page: Page) {
  const mobileButton = page.getByRole('button', { name: 'Open navigation', exact: true });
  if (await mobileButton.isVisible()) await mobileButton.click();
  const desktopButton = page.getByRole('button', { name: 'Expand navigation', exact: true });
  if (await desktopButton.isVisible()) await desktopButton.click();
  const navigation = page.getByRole('navigation', { name: 'Workspaces and sessions' });
  await expect(navigation).toBeVisible();
  return navigation;
}

test('keep compact sessions visible while the header opens a workspace menu', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  await page.reload();
  await expect(page.getByLabel('Message', { exact: true })).toBeEnabled();
  await page.getByLabel('Message', { exact: true }).fill('Keep my draft');
  const navigation = await openNavigation(page);
  await expect(navigation.getByRole('heading', { name: 'Sessions', exact: true })).toBeVisible();
  await expect(navigation).not.toContainText('Your tools. Your workspace.');
  await expect(navigation).not.toContainText('This console keeps you in the loop.');
  const switcher = navigation.getByRole('button', { name: 'Switch workspace', exact: true });
  await expect(switcher).toContainText('example-project');
  await expect(navigation.getByRole('heading', { name: 'Workspaces', exact: true })).toHaveCount(0);
  await expect(navigation.getByRole('button', { name: 'New workspace', exact: true })).toHaveCount(
    0,
  );
  await expect(navigation.locator('[data-workspace-id]')).toHaveCount(0);
  const sessionButton = navigation.getByRole('button', {
    name: `Session ${shortID(session.ID)}`,
    exact: true,
  });
  await expect(sessionButton).toBeVisible();
  await expect(sessionButton).not.toContainText(session.Model);
  await expect(sessionButton).toHaveAttribute('title', `${session.ID}\n${session.Model}`);
  const row = (await sessionButton.boundingBox())!;
  expect(row.height).toBe(testInfo.project.name === 'mobile' ? 44 : 32);
  if (testInfo.project.name === 'mobile')
    await expect(page.getByRole('dialog', { name: 'Workspaces and sessions' })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('workspace-sessions.png') });
  await switcher.press('Enter');
  const menu = page.getByRole('menu', { name: 'Workspace menu', exact: true });
  await expect(menu).toBeVisible();
  await expect(menu.getByRole('menuitem', { name: 'example-project', exact: true })).toBeVisible();
  await expect(menu.getByRole('menuitem', { name: 'New workspace', exact: true })).toBeVisible();
  await expect(navigation.getByRole('heading', { name: 'Sessions', exact: true })).toBeVisible();
  await expect(sessionButton).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('workspace-menu.png') });
  await page.keyboard.press('Escape');
  await expect(menu).toHaveCount(0);
  await expect(switcher).toBeFocused();
  if (testInfo.project.name === 'mobile')
    await expect(page.getByRole('dialog', { name: 'Workspaces and sessions' })).toBeVisible();
  await switcher.click();
  await menu.getByRole('menuitem', { name: 'example-project', exact: true }).click();
  await expect(menu).toHaveCount(0);
  await sessionButton.click();
  if (testInfo.project.name === 'mobile') await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue('Keep my draft');
  if (testInfo.project.name === 'desktop')
    await page.getByRole('button', { name: 'Collapse navigation', exact: true }).click();
  const reopened = await openNavigation(page);
  await expect(
    reopened.getByRole('button', { name: 'Switch workspace', exact: true }),
  ).toContainText('example-project');
  await expect(reopened.getByRole('heading', { name: 'Sessions', exact: true })).toBeVisible();
});

test('a late workspace read cannot replace the selected session list', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const original = fixture.instances[0];
  const originalSession = fixture.sessions[0];
  const second = { ...original, ID: 'workspace-two', Workspace: '/workspace/another-project' };
  const secondSession = { ...originalSession, ID: 'foreign-session', InstanceID: second.ID };
  fixture.instances.push(second);
  fixture.sessions.push(secondSession);
  let release = () => {};
  let started = () => {};
  const pending = new Promise<void>((resolve) => (release = resolve));
  const requested = new Promise<void>((resolve) => (started = resolve));
  await page.route(`**/instances/${second.ID}/sessions`, async (route) => {
    started();
    await pending;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify([secondSession]) });
  });
  try {
    const navigation = await openNavigation(page);
    await navigation.getByRole('button', { name: 'Switch workspace', exact: true }).click();
    await page.getByRole('menuitem', { name: 'Refresh workspaces', exact: true }).click();
    await page.getByRole('menuitem', { name: 'another-project', exact: true }).click();
    await requested;
    await expect(
      navigation.getByRole('button', { name: 'Switch workspace', exact: true }),
    ).toContainText('another-project');
    await expect(navigation.getByRole('status')).toHaveText('Loading sessions…');
    await expect(
      navigation.getByRole('button', {
        name: `Session ${shortID(originalSession.ID)}`,
        exact: true,
      }),
    ).toHaveCount(0);
    await navigation.getByRole('button', { name: 'Switch workspace', exact: true }).click();
    await page.getByRole('menuitem', { name: 'example-project', exact: true }).click();
    await expect(
      navigation.getByRole('button', {
        name: `Session ${shortID(originalSession.ID)}`,
        exact: true,
      }),
    ).toBeVisible();
    release();
    await expect(
      navigation.getByRole('button', { name: 'Switch workspace', exact: true }),
    ).toContainText('example-project');
    await expect(
      navigation.getByRole('button', { name: `Session ${shortID(secondSession.ID)}`, exact: true }),
    ).toHaveCount(0);
  } finally {
    release();
  }
});

test('show empty and stopped workspaces at 320px and recover a failed session read', async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 740 });
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const second = {
    ...fixture.instances[0],
    ID: 'empty-workspace',
    Workspace: `/workspace/${'long-project-name-'.repeat(10)}`,
    Stopped: true,
  };
  fixture.instances.push(second);
  let reject = true;
  await page.route(`**/instances/${second.ID}/sessions`, async (route) => {
    if (reject)
      return route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Cannot load these sessions.' }),
      });
    return route.fallback();
  });
  const navigation = await openNavigation(page);
  await navigation.getByRole('button', { name: 'Switch workspace', exact: true }).click();
  await page.getByRole('menuitem', { name: 'Refresh workspaces', exact: true }).click();
  await page.getByRole('menu').locator('[data-workspace-id="empty-workspace"]').click();
  await expect(navigation.getByRole('alert')).toHaveText('Cannot load these sessions.');
  await expect(navigation.getByRole('button', { name: 'New session', exact: true })).toBeDisabled();
  reject = false;
  await navigation.getByRole('button', { name: 'Retry loading sessions', exact: true }).click();
  await expect(
    navigation.getByText('No sessions in this workspace yet.', { exact: true }),
  ).toBeVisible();
  await expect(
    navigation.getByRole('button', { name: 'Switch workspace', exact: true }),
  ).toHaveAttribute('title', `${second.Workspace} — switch workspace`);
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  ).toBeTruthy();
  await navigation.getByRole('button', { name: 'Switch workspace', exact: true }).click();
  await expect(page.getByRole('menu', { name: 'Workspace menu', exact: true })).toBeVisible();
  await expect(navigation.getByRole('heading', { name: 'Sessions', exact: true })).toBeVisible();
});

test('create a workspace from the header dropdown', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const navigation = await openNavigation(page);
  await navigation.getByRole('button', { name: 'Switch workspace', exact: true }).click();
  await page.getByRole('menuitem', { name: 'New workspace', exact: true }).click();
  await expect(page.getByRole('menu')).toHaveCount(0);
  const dialog = page.getByRole('dialog');
  await expect(dialog).toHaveCount(1);
  await dialog.getByLabel('Project directory').fill('/workspace/menu-created');
  await dialog.getByRole('button', { name: 'Create workspace', exact: true }).click();
  await page.getByRole('button', { name: 'Create session', exact: true }).click();
  await expect(page.getByLabel('Message', { exact: true })).toBeEnabled();
  const reopened = await openNavigation(page);
  await expect(
    reopened.getByRole('button', { name: 'Switch workspace', exact: true }),
  ).toContainText('menu-created');
  await expect(reopened.getByRole('heading', { name: 'Sessions', exact: true })).toBeVisible();
  expect(fixture.instances.at(-1)?.Workspace).toBe('/workspace/menu-created');
});
