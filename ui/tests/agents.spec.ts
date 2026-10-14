import { expect, test, type Page } from '@playwright/test';
import type { Message, Session, ToolCall } from '../src/lib/atoms/types';
import { sessionDescendants, sessionRoot } from '../src/lib/atoms/session-tree';
import { shortID } from '../src/lib/atoms/format';
import { mockHarness } from './api-fixture';
import { connectAndCreate, openWorkbenchAction } from './helpers';

type Fixture = Awaited<ReturnType<typeof mockHarness>>;

function message(
  session: Session,
  identifier: string,
  role: Message['Role'],
  text = '',
  calls: ToolCall[] | null = null,
): Message {
  return {
    ID: identifier,
    SessionID: session.ID,
    Seq: 0,
    Role: role,
    Content: text
      ? [{ Type: 'text', Text: text, Data: null, MIME: '', URL: '', Filename: '', AudioID: '' }]
      : [],
    Reasoning: '',
    ToolCalls: calls,
    ToolCallID: '',
    Usage: null,
    CreatedAt: new Date().toISOString(),
  };
}

function addAgent(fixture: Fixture, parent: Session, identifier: string, task: string): Session {
  const agent: Session = {
    ...parent,
    ID: identifier,
    Parent: parent.ID,
    Depth: parent.Depth + 1,
    CreatedAt: new Date().toISOString(),
    Completed: false,
  };
  fixture.sessions.push(agent);
  fixture.histories.set(identifier, [message(agent, `${identifier}-task`, 'user', task)]);
  return agent;
}

async function connected(fixture: Fixture, sessionID: string) {
  await expect
    .poll(() =>
      [...fixture.openSockets].some((socket) => socket.url().includes(`/sessions/${sessionID}/`)),
    )
    .toBe(true);
}

async function navigation(page: Page) {
  const menu = page.getByRole('navigation', { name: 'Workspaces and sessions', exact: true });
  if (!(await menu.isVisible()))
    await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
  return menu;
}

async function expectParentSelected(page: Page, parent: Session) {
  const menu = await navigation(page);
  await expect(
    menu
      .locator(`[data-session-node="${parent.ID}"]`)
      .getByRole('button', { name: `Session ${shortID(parent.ID)}`, exact: true }),
  ).toHaveAttribute('aria-current', 'page');
  if (page.viewportSize()!.width < 1024)
    await menu.getByRole('button', { name: 'Close navigation', exact: true }).click();
}

test('stream concurrent agents in fixed-height chat panels with active tree children', async ({
  page,
}) => {
  const failures: string[] = [];
  page.on('pageerror', (error) => failures.push(error.message));
  const fixture = await mockHarness(page, { replayEvents: true });
  await connectAndCreate(page);
  const parent = fixture.sessions[0];
  await connected(fixture, parent.ID);
  const calls = [
    {
      ID: 'agent-call-a',
      Name: 'agent',
      Input: { task: 'Inspect the API and report findings.', model: parent.Model },
    },
    {
      ID: 'agent-call-b',
      Name: 'agent',
      Input: { task: 'Inspect the UI and report findings.', model: parent.Model },
    },
  ];
  fixture.running.add(parent.ID);
  fixture.event(parent, 'turn.start', null);
  fixture.event(parent, 'model.call', { message_id: 'parent-delegation', model: parent.Model });
  const first = addAgent(fixture, parent, 'agent-a1', calls[0].Input.task);
  const second = addAgent(fixture, parent, 'agent-b2', calls[1].Input.task);
  for (let index = 0; index < calls.length; index++) {
    fixture.event(parent, 'action.received', calls[index]);
    fixture.event(parent, 'tool.start', calls[index]);
    fixture.event(parent, 'agent.start', {
      session: [first, second][index].ID,
      task: calls[index].Input.task,
    });
  }
  fixture.histories
    .get(parent.ID)!
    .push(message(parent, 'parent-delegation', 'assistant', '', calls));
  fixture.event(parent, 'tool.start', calls[0]);
  await connected(fixture, first.ID);
  await connected(fixture, second.ID);
  fixture.event(first, 'turn.start', null);
  fixture.event(first, 'model.call', { message_id: 'first-output', model: first.Model });
  fixture.event(second, 'turn.start', null);
  fixture.event(second, 'model.call', { message_id: 'second-output', model: second.Model });
  fixture.event(first, 'model.chunk', {
    message_id: 'first-output',
    reasoning: '**Checking routes**\nThe API files are next.',
  });
  fixture.event(first, 'model.chunk', {
    message_id: 'first-output',
    text: 'First agent live output.',
  });
  fixture.event(second, 'model.chunk', {
    message_id: 'second-output',
    text: 'Second agent live output.',
  });
  const firstPanel = page.locator(`[data-agent-id="${first.ID}"]`);
  const secondPanel = page.locator(`[data-agent-id="${second.ID}"]`);
  await expect(firstPanel).toContainText('First agent live output.');
  await expect(secondPanel).toContainText('Second agent live output.');
  await expect(firstPanel).not.toContainText('Second agent live output.');
  await expect(page.getByRole('group', { name: 'Tool agent', exact: true })).toHaveCount(0);
  await expect(firstPanel.getByRole('img', { name: 'Working', exact: true })).toBeVisible();
  const initialHeight = await firstPanel.evaluate(
    (element) => element.getBoundingClientRect().height,
  );
  fixture.event(first, 'model.chunk', {
    message_id: 'first-output',
    text:
      '\n\n' + Array.from({ length: 100 }, (_, index) => `Agent output row ${index}.`).join('\n\n'),
  });
  await expect(firstPanel).toContainText('Agent output row 99.');
  expect(await firstPanel.evaluate((element) => element.getBoundingClientRect().height)).toBe(
    initialHeight,
  );
  expect(initialHeight).toBe(320);
  const output = firstPanel.getByRole('region', {
    name: `Agent ${shortID(first.ID)} output`,
    exact: true,
  });
  await output.evaluate((element) => (element.scrollTop = 0));
  await expect(
    firstPanel.getByRole('button', { name: 'Scroll to latest agent output' }),
  ).toBeVisible();
  fixture.event(first, 'model.chunk', {
    message_id: 'first-output',
    text: '\n\nNew tail after manual scroll.',
  });
  await expect(firstPanel).toContainText('New tail after manual scroll.');
  expect(await output.evaluate((element) => element.scrollTop)).toBeLessThan(20);

  const menu = await navigation(page);
  const parentNode = menu.locator(`[data-session-node="${parent.ID}"]`);
  const childNode = parentNode.locator(`[data-session-node="${first.ID}"]`);
  await expect(
    parentNode.getByRole('list', { name: `Agents of session ${shortID(parent.ID)}`, exact: true }),
  ).toBeVisible();
  await expect(childNode.locator('[data-slot="agent-status"]')).toHaveAttribute(
    'data-active',
    'true',
  );
  const animated = await childNode
    .locator('[data-slot="agent-status"] span')
    .evaluate((element) => getComputedStyle(element).animationName);
  expect(animated).not.toBe('none');
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await expect
    .poll(() =>
      childNode
        .locator('[data-slot="agent-status"] span')
        .evaluate((element) => getComputedStyle(element).animationName),
    )
    .toBe('none');
  await childNode.getByRole('button', { name: `Agent ${shortID(first.ID)}`, exact: true }).click();
  await expectParentSelected(page, parent);
  await expect(firstPanel).toBeVisible();

  const stored = message(
    first,
    'first-output',
    'assistant',
    'First agent live output.\nSaved final output.',
    [{ ID: 'finish-a', Name: 'finish', Input: { result: 'The API review is complete.' } }],
  );
  fixture.histories.get(first.ID)!.push(stored);
  first.Completed = true;
  fixture.event(first, 'turn.end', { status: 'completed' });
  fixture.event(parent, 'agent.end', { session: first.ID });
  await expect(firstPanel).toContainText('The API review is complete.');
  await expect(firstPanel.getByRole('img', { name: 'Finished', exact: true })).toBeVisible();
  await firstPanel.getByRole('button', { name: `Collapse agent ${shortID(first.ID)}` }).click();
  fixture.event(parent, 'agent.end', { session: first.ID });
  await expect(
    firstPanel.getByRole('button', { name: `Expand agent ${shortID(first.ID)}` }),
  ).toHaveAttribute('aria-expanded', 'false');
  await firstPanel.getByRole('button', { name: `Expand agent ${shortID(first.ID)}` }).click();
  await expect(firstPanel.getByText('First agent live output.', { exact: false })).toHaveCount(1);
  await expect
    .poll(() =>
      [...fixture.openSockets].some((socket) => socket.url().includes(`/sessions/${first.ID}/`)),
    )
    .toBe(false);
  const reopened = await navigation(page);
  await expect(
    reopened.locator(`[data-session-node="${first.ID}"] [data-slot="agent-status"]`),
  ).toHaveAttribute('data-active', 'false');
  await expect(
    reopened.locator(`[data-session-node="${second.ID}"] [data-slot="agent-status"]`),
  ).toHaveAttribute('data-active', 'true');
  if (page.viewportSize()!.width < 1024)
    await reopened.getByRole('button', { name: 'Close navigation' }).click();
  if (page.viewportSize()!.width < 1024) {
    await page.setViewportSize({ width: 320, height: 720 });
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    expect(await firstPanel.evaluate((element) => element.getBoundingClientRect().height)).toBe(
      320,
    );
  }
  await page.screenshot({ path: test.info().outputPath('live-agents.png'), fullPage: true });
  expect(failures).toEqual([]);
});

test('nest grandchildren and focus their live output without changing the parent draft', async ({
  page,
}) => {
  const fixture = await mockHarness(page, { replayEvents: true });
  await connectAndCreate(page);
  const parent = fixture.sessions[0];
  await connected(fixture, parent.ID);
  await page.getByLabel('Message', { exact: true }).fill('Keep this parent draft.');
  fixture.running.add(parent.ID);
  fixture.event(parent, 'turn.start', null);
  const child = addAgent(fixture, parent, 'child-21', 'Investigate the project.');
  fixture.event(parent, 'model.call', { message_id: 'parent-call', model: parent.Model });
  fixture.histories
    .get(parent.ID)!
    .push(
      message(parent, 'parent-call', 'assistant', '', [
        { ID: 'spawn-child', Name: 'agent', Input: { task: 'Investigate the project.' } },
      ]),
    );
  fixture.event(parent, 'agent.start', { session: child.ID, task: 'Investigate the project.' });
  await connected(fixture, child.ID);
  fixture.event(child, 'turn.start', null);
  fixture.event(child, 'model.call', { message_id: 'child-call', model: child.Model });
  const grandchild = addAgent(fixture, child, 'nested-3', 'Check a focused detail.');
  fixture.histories
    .get(child.ID)!
    .push(
      message(child, 'child-call', 'assistant', '', [
        { ID: 'spawn-nested', Name: 'agent', Input: { task: 'Check a focused detail.' } },
      ]),
    );
  fixture.event(child, 'agent.start', { session: grandchild.ID, task: 'Check a focused detail.' });
  await connected(fixture, grandchild.ID);
  fixture.event(grandchild, 'turn.start', null);
  fixture.event(grandchild, 'model.call', { message_id: 'nested-reply', model: grandchild.Model });
  const menu = await navigation(page);
  const row = menu.locator(
    `[data-session-node="${parent.ID}"] [data-session-node="${child.ID}"] [data-session-node="${grandchild.ID}"]`,
  );
  await expect(row).toBeVisible();
  await row.getByRole('button', { name: `Agent ${shortID(grandchild.ID)}`, exact: true }).click();
  fixture.event(grandchild, 'model.chunk', {
    message_id: 'nested-reply',
    text: 'The nested agent is reporting live.',
  });
  const panel = page.locator(`[data-agent-id="${child.ID}"]`);
  await expect(panel).toContainText('The nested agent is reporting live.');
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue('Keep this parent draft.');
  await expectParentSelected(page, parent);
  await panel.getByRole('button', { name: 'Back to parent agent' }).click();
  await expect(
    panel.getByRole('region', { name: `Agent ${shortID(child.ID)} output`, exact: true }),
  ).toBeVisible();
  await page.screenshot({ path: test.info().outputPath('nested-agents.png'), fullPage: true });
});

test('restore finished agent panels from history and preserve unknown launch failures', async ({
  page,
}) => {
  const fixture = await mockHarness(page, { replayEvents: true });
  await connectAndCreate(page);
  const parent = fixture.sessions[0];
  await connected(fixture, parent.ID);
  const child = addAgent(fixture, parent, 'saved-44', 'Saved delegated task.');
  const call = { ID: 'old-agent-call', Name: 'agent', Input: { task: 'Saved delegated task.' } };
  fixture.histories.get(parent.ID)!.push(message(parent, 'old-response', 'assistant', '', [call]));
  fixture.histories
    .get(child.ID)!
    .push(message(child, 'saved-answer', 'assistant', 'Saved agent output.'));
  child.Completed = true;
  fixture.event(parent, 'model.call', { message_id: 'old-response', model: parent.Model });
  fixture.event(parent, 'agent.start', { session: child.ID, task: 'Saved delegated task.' });
  fixture.event(child, 'turn.end', { status: 'error' });
  fixture.event(parent, 'tool.end', { call, status: 'error', result: { Error: 'worker failed' } });
  await page.reload();
  const panel = page.locator(`[data-agent-id="${child.ID}"]`);
  await expect(panel).toBeVisible();
  await panel.getByRole('button', { name: `Expand agent ${shortID(child.ID)}` }).click();
  await expect(panel).toContainText('Saved agent output.');
  await expect(panel.getByRole('img', { name: 'Failed', exact: true })).toBeVisible();
  const failedCall = {
    ID: 'not-started',
    Name: 'agent',
    Input: { task: 'A task that could not start.' },
  };
  fixture.histories
    .get(parent.ID)!
    .push(message(parent, 'failed-launch', 'assistant', '', [failedCall]));
  fixture.histories.get(parent.ID)!.push({
    ...message(parent, 'failure', 'tool', 'agent: the agent depth limit is reached'),
    ToolCallID: failedCall.ID,
  });
  fixture.event(parent, 'tool.end', { call: failedCall, status: 'error', result: {} });
  await expect(page.getByRole('region', { name: 'Conversation', exact: true })).toContainText(
    'agent: the agent depth limit is reached',
  );
  await expect(page.getByRole('group', { name: 'Tool agent', exact: true })).toHaveCount(0);
});

test('reconnect a live child, show approval waits, and mute cancellation', async ({ page }) => {
  const fixture = await mockHarness(page, { replayEvents: true });
  await connectAndCreate(page);
  const parent = fixture.sessions[0];
  await connected(fixture, parent.ID);
  fixture.running.add(parent.ID);
  fixture.event(parent, 'turn.start', null);
  fixture.event(parent, 'model.call', { message_id: 'approval-parent', model: parent.Model });
  const child = addAgent(fixture, parent, 'waiting5', 'Check a permission-protected file.');
  fixture.histories.get(parent.ID)!.push(
    message(parent, 'approval-parent', 'assistant', '', [
      {
        ID: 'delegate-permission',
        Name: 'agent',
        Input: { task: 'Check a permission-protected file.' },
      },
    ]),
  );
  fixture.event(parent, 'agent.start', {
    session: child.ID,
    task: 'Check a permission-protected file.',
  });
  await connected(fixture, child.ID);
  fixture.event(child, 'turn.start', null);
  fixture.event(child, 'model.call', { message_id: 'permission-reply', model: child.Model });
  fixture.event(child, 'model.chunk', {
    message_id: 'permission-reply',
    text: 'Before reconnect. ',
  });
  const panel = page.locator(`[data-agent-id="${child.ID}"]`);
  await expect(panel).toContainText('Before reconnect.');
  // Direct child execution does not appear as a running queued turn.
  expect(fixture.running.has(child.ID)).toBe(false);
  await expect(panel.getByRole('img', { name: 'Working', exact: true })).toBeVisible();
  const socket = [...fixture.openSockets].find((socket) =>
    socket.url().includes(`/sessions/${child.ID}/`),
  )!;
  fixture.openSockets.delete(socket);
  socket.close();
  fixture.event(child, 'model.chunk', { message_id: 'permission-reply', text: 'After reconnect.' });
  await connected(fixture, child.ID);
  await expect(panel).toContainText('Before reconnect. After reconnect.');
  await expect(panel.getByText('Before reconnect. After reconnect.', { exact: true })).toHaveCount(
    1,
  );
  fixture.event(child, 'permission.request', {
    ID: 'agent-permission',
    InstanceID: child.InstanceID,
    SessionID: child.ID,
    Target: '/workspace/private.txt',
    Why: 'File access',
  });
  await expect(panel.getByRole('img', { name: 'Waiting for approval', exact: true })).toBeVisible();
  await expect(panel.locator('[data-slot="agent-status"]')).toHaveAttribute('data-active', 'false');
  await panel.getByRole('button', { name: 'Allow once', exact: true }).click();
  await expect(panel.getByRole('img', { name: 'Working', exact: true })).toBeVisible();
  child.Completed = true;
  fixture.event(child, 'turn.end', { status: 'cancelled' });
  await expect(panel.getByRole('img', { name: 'Cancelled', exact: true })).toBeVisible();
  await expect(panel.locator('[data-slot="agent-status"]')).toHaveAttribute('data-active', 'false');
});

test('abort old agent reads and subscriptions when changing conversations', async ({ page }) => {
  const fixture = await mockHarness(page, { replayEvents: true });
  await connectAndCreate(page);
  const parent = fixture.sessions[0];
  await connected(fixture, parent.ID);
  fixture.running.add(parent.ID);
  fixture.event(parent, 'turn.start', null);
  fixture.event(parent, 'model.call', { message_id: 'slow-parent', model: parent.Model });
  const child = addAgent(fixture, parent, 'slowfeed', 'Read source files.');
  let releaseRead = () => {};
  const readGate = new Promise<void>((resolve) => (releaseRead = resolve));
  let reads = 0;
  await page.route(`**/api/sessions/${child.ID}/messages`, async (route) => {
    reads++;
    await readGate;
    await route
      .fulfill({
        json: [message(child, 'stale-output', 'assistant', 'Late output from the previous agent.')],
      })
      .catch(() => {});
  });
  fixture.histories
    .get(parent.ID)!
    .push(
      message(parent, 'slow-parent', 'assistant', '', [
        { ID: 'slow-spawn', Name: 'agent', Input: { task: 'Read source files.' } },
      ]),
    );
  fixture.event(parent, 'agent.start', { session: child.ID, task: 'Read source files.' });
  await connected(fixture, child.ID);
  await expect.poll(() => reads).toBeGreaterThan(0);
  await openWorkbenchAction(page, 'New session');
  await page.getByRole('button', { name: 'Create session', exact: true }).click();
  const newer = fixture.sessions.find((session) => !session.Parent && session.ID !== parent.ID)!;
  await connected(fixture, newer.ID);
  releaseRead();
  await expect(page.locator('[data-agent-id]')).toHaveCount(0);
  await expect
    .poll(() =>
      [...fixture.openSockets].some((socket) => socket.url().includes(`/sessions/${child.ID}/`)),
    )
    .toBe(false);
  fixture.event(child, 'model.call', { message_id: 'late-call', model: child.Model });
  fixture.event(child, 'model.chunk', {
    message_id: 'late-call',
    text: 'Late output from the previous agent.',
  });
  await expect(page.getByRole('region', { name: 'Conversation', exact: true })).not.toContainText(
    'Late output from the previous agent.',
  );
  await page.getByLabel('Message', { exact: true }).fill('New conversation draft.');
  await expectParentSelected(page, newer);
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue('New conversation draft.');
});

test('session trees use parent IDs and terminate on missing or cyclic parents', () => {
  const root = {
    ID: 'root',
    Parent: '',
    InstanceID: 'workspace',
    CreatedAt: '2026-01-01',
  } as Session;
  const child = { ...root, ID: 'child', Parent: root.ID };
  const nested = { ...root, ID: 'nested', Parent: child.ID };
  const foreign = { ...root, ID: 'foreign', Parent: root.ID, InstanceID: 'other' };
  expect(sessionRoot(nested, [nested, root, child])).toEqual(root);
  expect(sessionRoot(nested, [nested, root])).toBeNull();
  expect(sessionRoot(foreign, [root, foreign])).toBeNull();
  expect(sessionRoot(child, [{ ...root, Parent: 'child' }, child])).toBeNull();
  expect(sessionDescendants([nested, root, child], root.ID).map((entry) => entry.ID)).toEqual([
    'child',
    'nested',
  ]);
  expect(sessionDescendants([root, child, foreign], root.ID).map((entry) => entry.ID)).toEqual([
    'child',
  ]);
});
