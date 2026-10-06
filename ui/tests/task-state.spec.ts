import { expect, test } from '@playwright/test';
import type { TaskState } from '../src/lib/atoms/types';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';

test('show task progress, keep accomplishments during work, and clear finished state', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  const failures: string[] = [];
  page.on('pageerror', (error) => failures.push(error.message));
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  const progress = page.getByRole('region', { name: 'Task progress' });
  await expect(progress).toHaveCount(0);
  await page.getByLabel('Message', { exact: true }).fill('Keep this draft');
  const state: TaskState = {
    SessionID: session.ID,
    Todo: [
      { ID: '1', Title: 'Inspect the current behavior', Status: 'in_progress' },
      { ID: '2', Title: 'Run the regression checks', Status: 'pending' },
    ],
    Doing: {
      Title: 'Inspecting the task state',
      Description:
        'Reading the existing request boundary and checking how the state reaches the model.',
    },
    Revision: 1,
    UpdatedAt: new Date().toISOString(),
  };
  fixture.taskStates.set(session.ID, state);
  fixture.event(session, 'task_state.updated', state);
  await expect(progress).toContainText('0/2 done');
  await expect(progress).toContainText('Inspecting the task state');
  const updated: TaskState = {
    ...state,
    Todo: [
      { ...state.Todo[0], Status: 'done' },
      { ...state.Todo[1], Status: 'in_progress' },
    ],
    Doing: {
      Title: 'Running the checks',
      Description:
        'Checking that saved task state survives a restart and clears when the work finishes.',
    },
    Revision: 2,
  };
  fixture.taskStates.set(session.ID, updated);
  fixture.event(session, 'task_state.updated', updated);
  await expect(progress).toContainText('1/2 done');
  await expect(progress.locator('[data-status="done"]')).toHaveText(
    /Inspect the current behavior.*Done/,
  );
  await expect(progress.locator('[data-status="in_progress"]')).toHaveText(
    /Run the regression checks.*In progress/,
  );
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue('Keep this draft');
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 700 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await progress.getByRole('button', { name: 'Toggle task progress' }).click();
  await expect(progress.getByRole('list', { name: 'TODO items' })).toHaveCount(0);
  await page.keyboard.press('Enter');
  await expect(progress.getByRole('list', { name: 'TODO items' })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('task-progress.png'), fullPage: true });

  await page.reload();
  await expect(progress).toContainText('1/2 done');
  const cleared: TaskState = { ...updated, Todo: [], Doing: null, Revision: 3 };
  fixture.taskStates.set(session.ID, cleared);
  fixture.event(session, 'task_state.updated', cleared);
  await expect(progress).toHaveCount(0);
  fixture.event(session, 'task_state.updated', updated);
  await expect(progress).toHaveCount(0);
  expect(failures).toEqual([]);
});

test('ignore an older task-state read after a newer completion event', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  const progress = page.getByRole('region', { name: 'Task progress' });
  const state: TaskState = {
    SessionID: session.ID,
    Todo: [{ ID: '1', Title: 'Old work', Status: 'in_progress' }],
    Doing: { Title: 'Old activity', Description: 'An earlier request snapshot.' },
    Revision: 1,
    UpdatedAt: new Date().toISOString(),
  };
  fixture.taskStates.set(session.ID, state);
  fixture.event(session, 'task_state.updated', state);
  await expect(progress).toContainText('Old activity');
  let releaseRead!: () => void;
  let enteredRead!: () => void;
  const blocked = new Promise<void>((resolve) => {
    releaseRead = resolve;
  });
  const entered = new Promise<void>((resolve) => {
    enteredRead = resolve;
  });
  let delayed = false;
  await page.route(`**/sessions/${session.ID}/task-state`, async (route) => {
    if (delayed) return route.fallback();
    delayed = true;
    enteredRead();
    await blocked;
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify(state) });
  });
  await entered;
  const cleared: TaskState = { ...state, Todo: [], Doing: null, Revision: 2 };
  fixture.taskStates.set(session.ID, cleared);
  fixture.event(session, 'task_state.updated', cleared);
  await expect(progress).toHaveCount(0);
  const oldResponse = page.waitForResponse(
    (response) =>
      response.url().endsWith(`/sessions/${session.ID}/task-state`) && response.status() === 200,
  );
  releaseRead();
  await oldResponse;
  await page.evaluate(
    () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      ),
  );
  await expect(progress).toHaveCount(0);
  fixture.event(session, 'task_state.updated', cleared);
  await expect(progress).toHaveCount(0);
});

test('task-progress API failure preserves the selected conversation', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  await page.getByLabel('Message', { exact: true }).fill('Keep the session');
  await page.route(`**/sessions/${session.ID}/task-state`, (route) =>
    route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'Progress store unavailable' }),
    }),
  );
  fixture.event(session, 'turn.end', {});
  await expect(page.getByRole('region', { name: 'Task progress' })).toContainText(
    'Progress store unavailable',
  );
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue('Keep the session');
  await expect(page.getByLabel('Message', { exact: true })).toBeEnabled();
});
