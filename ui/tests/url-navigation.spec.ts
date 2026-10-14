import { expect, test, type Page } from '@playwright/test';
import type { Instance, Session } from '../src/lib/atoms/types';
import { mockHarness } from './api-fixture';
import { shortID } from '../src/lib/atoms/format';

async function seed(page: Page, connected = true) {
  const fixture = await mockHarness(page);
  const first: Instance = {
    ID: 'workspace-one',
    Workspace: '/project/one',
    DefaultModel: fixture.model.ID,
    Models: [],
    Stopped: false,
    ProcessLimit: 8,
    AgentDepthLimit: 3,
    CreatedAt: '2026-01-01T00:00:00Z',
  };
  const second = { ...first, ID: 'workspace-two', Workspace: '/project/two' };
  const older = {
    ID: 'older-session',
    InstanceID: first.ID,
    Parent: '',
    Depth: 0,
    Completed: false,
    Model: fixture.model.ID,
    ReasoningEffort: '',
    CreatedAt: '2026-01-01T00:00:00Z',
  } as Session;
  const newer = { ...older, ID: 'newer-session', CreatedAt: '2026-01-02T00:00:00Z' };
  const foreign = { ...older, ID: 'foreign-session', InstanceID: second.ID };
  const child = { ...older, ID: 'child-session', Parent: older.ID, Depth: 1 };
  fixture.instances.push(first, second);
  fixture.sessions.push(older, newer, foreign, child);
  if (connected)
    await page.addInitScript(() =>
      sessionStorage.setItem(
        'mtt-console-connection',
        JSON.stringify({ base: '/api', token: 'test-token' }),
      ),
    );
  return { fixture, first, second, older, newer, foreign, child };
}
async function navigation(page: Page) {
  const nav = page.getByRole('navigation', { name: 'Workspaces and sessions', exact: true });
  if (!(await nav.isVisible()))
    await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
  return nav;
}
async function expectSession(page: Page, session: Session) {
  await expect(page.getByLabel('Message', { exact: true })).toBeEnabled();
  await expect.poll(() => new URL(page.url()).searchParams.get('session')).toBe(session.ID);
  const nav = await navigation(page);
  await expect(
    nav.getByRole('button', { name: `Session ${shortID(session.ID)}`, exact: true }),
  ).toHaveAttribute('aria-current', 'page');
}

test('restore a non-default workspace and older session after reload without exposing credentials', async ({
  page,
}) => {
  const { second, foreign } = await seed(page);
  await page.goto(`/?workspace=${second.ID}&session=${foreign.ID}&custom=keep#anchor`);
  await expectSession(page, foreign);
  await page.reload();
  await expectSession(page, foreign);
  expect(new URL(page.url()).searchParams.get('workspace')).toBe(second.ID);
  expect(new URL(page.url()).searchParams.get('custom')).toBe('keep');
  expect(new URL(page.url()).hash).toBe('#anchor');
  expect(page.url()).not.toContain('test-token');
  expect(page.url()).not.toContain('/project/');
});

test('Back and Forward restore selections and retain per-session drafts', async ({ page }) => {
  const { first, older, newer } = await seed(page);
  await page.goto(`/?workspace=${first.ID}&session=${older.ID}`);
  await expectSession(page, older);
  if (page.viewportSize()!.width < 1024)
    await page.getByRole('button', { name: 'Close navigation', exact: true }).click();
  await page.getByLabel('Message', { exact: true }).fill('Older draft');
  let nav = await navigation(page);
  await nav.getByRole('button', { name: `Session ${shortID(newer.ID)}`, exact: true }).click();
  await expectSession(page, newer);
  await page.goBack();
  await expectSession(page, older);
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue('Older draft');
  await page.goForward();
  await expectSession(page, newer);
});

test('restore an agent within its parent conversation', async ({ page }) => {
  const { first, older, child } = await seed(page);
  await page.goto(`/?workspace=${first.ID}&session=${older.ID}&agent=${child.ID}`);
  await expectSession(page, older);
  if (page.viewportSize()!.width < 1024)
    await page.getByRole('button', { name: 'Close navigation', exact: true }).click();
  await expect(
    page
      .locator(`[data-agent-id="${child.ID}"]`)
      .getByRole('button', { name: `Collapse agent ${shortID(child.ID)}` }),
  ).toBeVisible();
  await page.reload();
  await expectSession(page, older);
  expect(new URL(page.url()).searchParams.get('agent')).toBe(child.ID);
});

test('normalize session-only links and reject mismatched workspace/session IDs', async ({
  page,
}) => {
  const { first, older, second } = await seed(page);
  await page.goto(`/?session=${older.ID}`);
  await expectSession(page, older);
  expect(new URL(page.url()).searchParams.get('workspace')).toBe(first.ID);
  await page.goto(`/?workspace=${second.ID}&session=${older.ID}`);
  await expect(
    page.getByText('The session in this URL is not available in this workspace.', { exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel('Message', { exact: true })).toBeDisabled();
  expect(new URL(page.url()).searchParams.get('session')).toBeNull();
  expect(new URL(page.url()).searchParams.get('workspace')).toBe(second.ID);
});

test('browser Back fences an unfinished workspace request', async ({ page }) => {
  const { first, second, older } = await seed(page);
  await page.goto(`/?workspace=${first.ID}&session=${older.ID}`);
  await expectSession(page, older);
  let release = () => {};
  const held = new Promise<void>((resolve) => (release = resolve));
  let started = () => {};
  const requested = new Promise<void>((resolve) => (started = resolve));
  await page.route(`**/instances/${second.ID}/sessions`, async (route) => {
    started();
    await held;
    await route.fulfill({ json: [] });
  });
  const nav = await navigation(page);
  await nav.getByRole('button', { name: 'Switch workspace', exact: true }).click();
  await page.getByRole('menuitem', { name: 'two', exact: true }).click();
  await requested;
  await page.goBack();
  await expectSession(page, older);
  release();
  await expectSession(page, older);
  expect(new URL(page.url()).searchParams.get('workspace')).toBe(first.ID);
});

test('workspace changes use one history entry and Forward restores the conversation', async ({
  page,
}) => {
  const { first, second, older, foreign } = await seed(page);
  await page.goto(`/?workspace=${first.ID}&session=${older.ID}`);
  await expectSession(page, older);
  const nav = await navigation(page);
  await nav.getByRole('button', { name: 'Switch workspace', exact: true }).click();
  await page.getByRole('menuitem', { name: 'two', exact: true }).click();
  await expectSession(page, foreign);
  await page.goBack();
  await expectSession(page, older);
  await page.goForward();
  await expectSession(page, foreign);
  expect(new URL(page.url()).searchParams.get('workspace')).toBe(second.ID);
});

test('keep a deep link while the connection form is completed', async ({ page }) => {
  const { second, foreign } = await seed(page, false);
  await page.goto(`/?workspace=${second.ID}&session=${foreign.ID}`);
  await page.getByRole('button', { name: 'Connect harness', exact: true }).click();
  await page.getByLabel('API token', { exact: true }).fill('test-token');
  await page.getByRole('button', { name: 'Connect to API', exact: true }).click();
  await expectSession(page, foreign);
  expect(page.url()).not.toContain('test-token');
});
