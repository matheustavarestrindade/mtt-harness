import { test, expect } from '@playwright/test';
import type { Message, Session } from '../src/lib/atoms/types';
import { mockHarness } from './api-fixture';
import { mockWatchers } from './watcher-fixture';
import { connectAndCreate } from './helpers';

const content = (text: string) => ({
  Type: 'text' as const,
  Text: text,
  Data: null,
  MIME: '',
  URL: '',
  Filename: '',
  AudioID: '',
});
const message = (session: Session, id: string, role: Message['Role'], text: string): Message => ({
  ID: id,
  SessionID: session.ID,
  Seq: 0,
  Role: role,
  Content: [content(text)],
  ToolCalls: [],
  ToolCallID: '',
  Usage: null,
  CreatedAt: new Date().toISOString(),
});

test('stream reviewer text, thinking and tools, release closed views, and surface user questions', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page, { replayEvents: true }),
    watchers = await mockWatchers(page);
  const errors: string[] = [];
  let workerSessionReads = 0;
  page.on('request', (request) => {
    if (
      request.method() === 'GET' &&
      new URL(request.url()).pathname.endsWith('/sessions/watcher-private-worker')
    )
      workerSessionReads++;
  });
  page.on('pageerror', (failure) => errors.push(failure.message));
  await connectAndCreate(page);
  const parent = fixture.sessions[0];
  const worker: Session = {
    ...parent,
    ID: 'watcher-private-worker',
    Parent: parent.ID,
    Depth: 1,
    Completed: false,
  };
  fixture.sessions.push(worker);
  fixture.histories.set(worker.ID, [
    message(worker, 'worker-prompt', 'user', 'PRIVATE_REVIEW_INPUT'),
  ]);
  fixture.running.add(worker.ID);
  fixture.histories.set(parent.ID, [
    message(parent, 'answer', 'assistant', 'The original answer.'),
  ]);
  const entry = {
    ID: 'live-review',
    SessionID: parent.ID,
    AnchorID: 'answer',
    AnchorSequence: 1,
    Title: 'Live reviewer',
    Notes: [],
    CreatedAt: new Date().toISOString(),
    Estimated: true,
    Outcome: 'running',
    Trigger: 'end_turn',
    WorkerSessionID: worker.ID,
  };
  watchers.activity.set(parent.ID, {
    Name: 'watcher',
    SessionID: parent.ID,
    Revision: 1,
    HistoryRevision: 0,
    Entries: [entry],
  });
  fixture.state(parent);
  fixture.event(parent, 'plugin.activity', {
    plugin: 'watcher',
    run_id: entry.ID,
    status: 'running',
  });
  const review = page.locator('[data-watcher-run="live-review"]'),
    live = review.getByRole('region', { name: 'Watcher live activity', exact: true });
  await expect(live).toBeVisible();
  await expect
    .poll(() =>
      [...fixture.openSockets].some((socket) => socket.url().includes(`/sessions/${worker.ID}/`)),
    )
    .toBe(true);
  fixture.event(worker, 'turn.start', null);
  fixture.event(worker, 'model.call', { message_id: 'reviewer-response', model: worker.Model });
  fixture.event(worker, 'model.chunk', {
    message_id: 'reviewer-response',
    reasoning: '**Checking the evidence**\nReading the supplied files.',
  });
  await expect(live).toContainText('Checking the evidence');
  fixture.event(worker, 'model.chunk', {
    message_id: 'reviewer-response',
    text: 'Inspecting the implementation live.',
  });
  await expect(live).toContainText('Inspecting the implementation live.');
  await live.getByRole('button', { name: /Thinking/ }).click();
  await expect(live).toContainText('Reading the supplied files.');
  await expect(live).not.toContainText('PRIVATE_REVIEW_INPUT');
  const call = {
    ID: 'reviewer-read',
    Name: 'file_actions',
    Input: { path: 'README.md', actions: [{ op: 'read' }] },
  };
  fixture.event(worker, 'action.received', call);
  fixture.event(worker, 'tool.start', call);
  const tool = live.getByRole('group', { name: 'Tool file_actions', exact: true });
  await expect(tool).toBeVisible();
  await tool.getByRole('button').first().click();
  await expect(tool).toContainText('README.md');
  fixture.event(worker, 'model.chunk', {
    message_id: 'reviewer-response',
    text: '\n\n' + Array.from({ length: 90 }, (_, index) => `Reviewer line ${index}.`).join('\n\n'),
  });
  await expect(live).toContainText('Reviewer line 89.');
  expect(await live.evaluate((element) => element.getBoundingClientRect().height)).toBe(320);
  const output = live.getByRole('region', { name: 'Watcher output', exact: true });
  await output.focus();
  await page.keyboard.press('Home');
  await expect.poll(() => output.evaluate((element) => element.scrollTop)).toBeLessThan(20);
  await expect(live.getByRole('button', { name: 'Latest reviewer output' })).toBeVisible();
  watchers.activity.set(parent.ID, {
    Name: 'watcher',
    SessionID: parent.ID,
    Revision: 2,
    HistoryRevision: 0,
    Entries: [{ ...entry, DurationMS: 17 }],
  });
  fixture.event(parent, 'plugin.activity', {
    plugin: 'watcher',
    run_id: entry.ID,
    status: 'running',
  });
  await expect(review.getByTitle('17 ms', { exact: true })).toBeVisible();
  expect(workerSessionReads).toBe(1);
  fixture.event(worker, 'model.chunk', {
    message_id: 'reviewer-response',
    text: '\nManual scrolling stays put.',
  });
  await expect(live).toContainText('Manual scrolling stays put.');
  expect(await output.evaluate((element) => element.scrollTop)).toBeLessThan(20);
  await review.getByRole('button', { name: 'Reviewer activity', exact: true }).click();
  await expect(live).toHaveCount(0);
  await expect
    .poll(() =>
      [...fixture.openSockets].some((socket) => socket.url().includes(`/sessions/${worker.ID}/`)),
    )
    .toBe(false);
  expect(fixture.running.has(worker.ID)).toBe(true);
  await review.getByRole('button', { name: 'Reviewer activity', exact: true }).click();
  await expect(live).toBeVisible();
  await expect(live).toContainText('Manual scrolling stays put.');
  await review.locator('.activity-toggle').first().click();
  await expect(live).toHaveCount(0);
  worker.Completed = true;
  fixture.running.delete(worker.ID);
  const saved = message(worker, 'reviewer-response', 'assistant', 'Saved reviewer output.');
  saved.Reasoning = 'Checking the evidence';
  fixture.histories.get(worker.ID)!.push(saved);
  fixture.event(worker, 'turn.end', { status: 'completed' });
  watchers.activity.set(parent.ID, {
    Name: 'watcher',
    SessionID: parent.ID,
    Revision: 3,
    HistoryRevision: 0,
    Entries: [
      {
        ...entry,
        Outcome: 'user_input_requested',
        Decision: 'interfere',
        ContinueChat: false,
        Notes: [{ Text: 'Which account should be used?', References: [] }],
      },
    ],
  });
  fixture.event(parent, 'plugin.activity', {
    plugin: 'watcher',
    run_id: entry.ID,
    status: 'user_input_requested',
  });
  await expect(review).toContainText('Which account should be used?');
  await expect(review.getByText('Needs your input', { exact: true }).first()).toBeVisible();
  await expect(review).toContainText('Off — user input requested');
  await expect(live).toHaveCount(0);
  expect(fixture.running.has(parent.ID)).toBe(false);
  await review.screenshot({ path: testInfo.outputPath('watcher-live-output.png') });
  expect(errors).toEqual([]);
});

test('collapse finished reviews and release their live socket while allowing history to reopen', async ({
  page,
}) => {
  const fixture = await mockHarness(page, { replayEvents: true }),
    watchers = await mockWatchers(page);
  await connectAndCreate(page);
  const parent = fixture.sessions[0];
  const worker: Session = {
    ...parent,
    ID: 'completion-worker',
    Parent: parent.ID,
    Depth: 1,
    Completed: false,
  };
  fixture.sessions.push(worker);
  fixture.running.add(worker.ID);
  fixture.histories.set(worker.ID, [message(worker, 'prompt', 'user', 'Private review task')]);
  fixture.histories.set(parent.ID, [message(parent, 'anchor', 'assistant', 'Finished answer.')]);
  const entry = {
    ID: 'auto-close-review',
    SessionID: parent.ID,
    AnchorID: 'anchor',
    AnchorSequence: 1,
    Title: 'Completion reviewer',
    Notes: [],
    CreatedAt: new Date().toISOString(),
    Estimated: true,
    Outcome: 'running',
    Trigger: 'end_turn',
    WorkerSessionID: worker.ID,
  };
  const update = (revision: number, outcome: string) => {
    watchers.activity.set(parent.ID, {
      Name: 'watcher',
      SessionID: parent.ID,
      Revision: revision,
      HistoryRevision: 0,
      Entries: [
        {
          ...entry,
          Outcome: outcome,
          Decision: 'continue',
          Notes: [{ Text: 'The saved review is complete.', References: [] }],
        },
      ],
    });
    fixture.event(parent, 'plugin.activity', {
      plugin: 'watcher',
      run_id: entry.ID,
      status: outcome,
    });
  };
  update(1, 'running');
  fixture.state(parent);
  const review = page.locator('[data-watcher-run="auto-close-review"]');
  const toggle = review.locator('.activity-toggle').first();
  await expect(
    review.getByRole('region', { name: 'Watcher live activity', exact: true }),
  ).toBeVisible();
  await expect
    .poll(() =>
      [...fixture.openSockets].some((socket) => socket.url().includes(`/sessions/${worker.ID}/`)),
    )
    .toBe(true);
  update(2, 'reviewed');
  await expect(toggle).toHaveAttribute('aria-expanded', 'false');
  worker.Completed = true;
  fixture.running.delete(worker.ID);
  fixture.histories
    .get(worker.ID)!
    .push(message(worker, 'finished', 'assistant', 'Completed worker output.'));
  fixture.event(worker, 'turn.end', { status: 'completed' });
  update(3, 'completed');
  await expect(toggle).toHaveAttribute('aria-expanded', 'false');
  await expect(
    review.getByRole('region', { name: 'Watcher live activity', exact: true }),
  ).toHaveCount(0);
  await expect
    .poll(() =>
      [...fixture.openSockets].some((socket) => socket.url().includes(`/sessions/${worker.ID}/`)),
    )
    .toBe(false);
  await toggle.click();
  await expect(review).toContainText('The saved review is complete.');
  await expect(
    review.getByRole('region', { name: 'Watcher live activity', exact: true }),
  ).toHaveCount(0);
  update(4, 'completed');
  await expect(toggle).toHaveAttribute('aria-expanded', 'true');
  await review.getByRole('button', { name: 'Reviewer activity', exact: true }).click();
  await expect(review).toContainText('Completed worker output.');
});

test('close terminal review states without collapsing a question that needs the user', async ({
  page,
}) => {
  const fixture = await mockHarness(page),
    watchers = await mockWatchers(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  fixture.histories.set(session.ID, [message(session, 'answer', 'assistant', 'Answer.')]);
  fixture.state(session);
  let revision = 0;
  for (const outcome of [
    'completed',
    'intervened',
    'error',
    'cancelled',
    'interrupted',
    'stale',
    'limited',
    'not_applied',
    'deferred_for_user',
    'user_input_requested',
  ]) {
    const entry = {
      ID: `terminal-${outcome}`,
      SessionID: session.ID,
      AnchorID: 'answer',
      AnchorSequence: 1,
      Title: `Review ${outcome}`,
      Notes: [{ Text: 'Review feedback.', References: [] }],
      CreatedAt: new Date().toISOString(),
      Estimated: true,
      Trigger: 'end_turn',
      Outcome: 'running',
    };
    const update = (status: string) => {
      watchers.activity.set(session.ID, {
        Name: 'watcher',
        SessionID: session.ID,
        Revision: ++revision,
        HistoryRevision: 0,
        Entries: [{ ...entry, Outcome: status }],
      });
      fixture.event(session, 'plugin.activity', { plugin: 'watcher', run_id: entry.ID, status });
    };
    update('running');
    const toggle = page.locator(`[data-watcher-run="${entry.ID}"] .activity-toggle`).first();
    await expect(toggle).toHaveAttribute('aria-expanded', 'true');
    update(outcome);
    await expect(toggle).toHaveAttribute(
      'aria-expanded',
      outcome === 'user_input_requested' ? 'true' : 'false',
    );
  }
});
