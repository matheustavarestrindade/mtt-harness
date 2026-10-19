import { test, expect, type Page } from '@playwright/test';
import type { Instance, Session, Message } from '../src/lib/atoms/types';
import { mockHarness } from './api-fixture';

async function seed(page: Page, secondWorkspace = true) {
  const fixture = await mockHarness(page);
  const first: Instance = {
    ID: 'workspace-first',
    Workspace: '/project/first',
    Models: [],
    DefaultModel: fixture.model.ID,
    ProcessLimit: 8,
    AgentDepthLimit: 2,
    Stopped: false,
    CreatedAt: '2026-01-01T00:00:00Z',
  };
  const second: Instance = { ...first, ID: 'workspace-second', Workspace: '/project/second' };
  const firstSession: Session = {
    ID: 'first-session',
    InstanceID: first.ID,
    Parent: '',
    Model: fixture.model.ID,
    ReasoningEffort: '',
    Depth: 0,
    Completed: false,
    CreatedAt: '2026-01-01T00:00:00Z',
  };
  const secondSession: Session = { ...firstSession, ID: 'second-session', InstanceID: second.ID };
  fixture.instances.push(first, ...(secondWorkspace ? [second] : []));
  fixture.sessions.push(firstSession, ...(secondWorkspace ? [secondSession] : []));
  const saved: Message = {
    ID: 'saved-user-message',
    SessionID: firstSession.ID,
    Seq: 1,
    Role: 'user',
    Content: [
      {
        Type: 'text',
        Text: 'Saved conversation remains',
        Data: null,
        MIME: '',
        URL: '',
        Filename: '',
        AudioID: '',
      },
    ],
    ToolCalls: [],
    ToolCallID: '',
    Usage: null,
    CreatedAt: firstSession.CreatedAt,
  };
  fixture.histories.set(firstSession.ID, [saved]);
  await page.addInitScript(() =>
    sessionStorage.setItem(
      'mtt-console-connection',
      JSON.stringify({ base: '/api', token: 'test-token' }),
    ),
  );
  await page.goto(`/?workspace=${first.ID}&session=${firstSession.ID}`);
  await expect(page.getByLabel('Message', { exact: true })).toBeEnabled();
  return { fixture, first, second, firstSession, secondSession, saved };
}

async function menu(page: Page) {
  const navigation = page.getByRole('navigation', { name: 'Workspaces and sessions', exact: true });
  if (!(await navigation.isVisible()))
    await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
  await navigation.getByRole('button', { name: 'Switch workspace', exact: true }).click();
  return page.getByRole('menu', { name: 'Workspace menu' });
}

async function openUnlink(page: Page) {
  const workspaces = await menu(page);
  await workspaces.getByRole('menuitem', { name: 'Unlink workspace', exact: true }).click();
  return page.getByRole('dialog', { name: 'Unlink workspace?' });
}

test('unlink the selected workspace, preserve saved data, and restore the next URL', async ({
  page,
}, testInfo) => {
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 740 });
  const { fixture, first, second, firstSession, saved } = await seed(page);
  const requests: string[] = [];
  page.on('request', (request) => {
    if (request.url().includes('/api/') && request.method() !== 'GET')
      requests.push(`${request.method()} ${new URL(request.url()).pathname}`);
  });
  const dialog = await openUnlink(page);
  await expect(dialog).toContainText('/project/first');
  await expect(dialog).toContainText('Files and folders stay untouched');
  await expect(dialog).toContainText('Saved chats, memories, and usage records are kept');
  await page.screenshot({ path: testInfo.outputPath('unlink-workspace.png') });
  await dialog.getByRole('button', { name: 'Unlink workspace', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect.poll(() => new URL(page.url()).searchParams.get('workspace')).toBe(second.ID);
  expect(fixture.unlinkedInstances).toEqual([first.ID]);
  expect(fixture.histories.get(firstSession.ID)).toEqual([saved]);
  expect(fixture.sessions.some((session) => session.ID === firstSession.ID)).toBe(true);
  expect(requests).toEqual([`POST /api/instances/${first.ID}/unlink`]);
  await page.reload();
  await expect(page.getByLabel('Message', { exact: true })).toBeEnabled();
  const workspaces = await menu(page);
  await expect(workspaces.locator(`[data-workspace-id="${first.ID}"]`)).toHaveCount(0);
  await expect(workspaces.locator(`[data-workspace-id="${second.ID}"]`)).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('cancel unlinking and retain the current draft', async ({ page }) => {
  const { fixture, first } = await seed(page);
  await page.getByLabel('Message', { exact: true }).fill('Keep the draft');
  const dialog = await openUnlink(page);
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
  expect(fixture.unlinkedInstances).toEqual([]);
  expect(new URL(page.url()).searchParams.get('workspace')).toBe(first.ID);
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue('Keep the draft');
});

test('unlink the last workspace without deleting its sessions or recreating it on refresh', async ({
  page,
}) => {
  const { fixture, firstSession } = await seed(page, false);
  const dialog = await openUnlink(page);
  await dialog.getByRole('button', { name: 'Unlink workspace', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect.poll(() => new URL(page.url()).searchParams.get('workspace')).toBe(null);
  expect(new URL(page.url()).searchParams.get('session')).toBe(null);
  await expect(page.getByLabel('Message', { exact: true })).toBeDisabled();
  expect(fixture.sessions[0].ID).toBe(firstSession.ID);
  await page.reload();
  await expect(page.getByLabel('Message', { exact: true })).toBeDisabled();
  const workspaces = await menu(page);
  await expect(workspaces).toContainText('No workspaces yet.');
  await expect(
    workspaces.getByRole('menuitem', { name: 'New workspace', exact: true }),
  ).toBeEnabled();
  expect(fixture.instances).toHaveLength(0);
});

test('reject busy workspaces without cancelling work or removing the reference', async ({
  page,
}) => {
  const { fixture, first, firstSession } = await seed(page);
  fixture.running.add(firstSession.ID);
  const dialog = await openUnlink(page);
  await dialog.getByRole('button', { name: 'Unlink workspace', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('workspace has active or queued work');
  expect(fixture.running.has(firstSession.ID)).toBe(true);
  expect(fixture.instances.some((instance) => instance.ID === first.ID)).toBe(true);
  expect(fixture.unlinkedInstances).toEqual([]);
});

test('ignore a stale workspace listing after unlink succeeds', async ({ page }) => {
  const { fixture, first, second } = await seed(page);
  const previous = JSON.stringify(fixture.instances);
  let release!: () => void;
  const held = new Promise<void>((resolve) => (release = resolve));
  let entered = false;
  await page.route(
    (url) => url.pathname === '/api/instances',
    async (route) => {
      entered = true;
      await held;
      await route.fulfill({ json: JSON.parse(previous) });
    },
  );
  let workspaces = await menu(page);
  await workspaces.getByRole('menuitem', { name: 'Refresh workspaces', exact: true }).click();
  await expect.poll(() => entered).toBe(true);
  await workspaces.getByRole('menuitem', { name: 'Unlink workspace', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Unlink workspace?' });
  await dialog.getByRole('button', { name: 'Unlink workspace', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  release();
  await expect.poll(() => new URL(page.url()).searchParams.get('workspace')).toBe(second.ID);
  workspaces = await menu(page);
  await expect(workspaces.locator(`[data-workspace-id="${first.ID}"]`)).toHaveCount(0);
});

test('a delayed unlink response cannot replace another workspace or its draft', async ({
  page,
}) => {
  const { first, second } = await seed(page);
  let release!: () => void;
  const held = new Promise<void>((resolve) => (release = resolve));
  let entered = false;
  await page.route(`**/instances/${first.ID}/unlink`, async (route) => {
    entered = true;
    await held;
    await route.fallback();
  });
  const dialog = await openUnlink(page);
  await dialog.getByRole('button', { name: 'Unlink workspace', exact: true }).click();
  await expect.poll(() => entered).toBe(true);
  await page.keyboard.press('Escape');
  const workspaces = await menu(page);
  await workspaces.locator(`[data-workspace-id="${second.ID}"]`).click();
  if (page.viewportSize()!.width < 1024)
    await page.getByRole('button', { name: 'Close navigation', exact: true }).click();
  await expect.poll(() => new URL(page.url()).searchParams.get('workspace')).toBe(second.ID);
  await page.getByLabel('Message', { exact: true }).fill('Unrelated workspace draft');
  release();
  await expect(page.getByText('Workspace unlinked', { exact: true })).toBeVisible();
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue(
    'Unrelated workspace draft',
  );
  expect(new URL(page.url()).searchParams.get('workspace')).toBe(second.ID);
});
