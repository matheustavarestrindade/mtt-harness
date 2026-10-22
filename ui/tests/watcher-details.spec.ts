import { test, expect, type Page } from '@playwright/test';
import type { Message } from '../src/lib/atoms/types';
import type { WatcherRun } from '../src/lib/atoms/watcher';
import { mockHarness } from './api-fixture';
import { mockWatchers } from './watcher-fixture';
import { connectAndCreate, openWorkbenchAction } from './helpers';

const event =
  '<watcher_event>{"context_tokens":0,"in_progress_message_id":"","model":"","permission_id":"","source_active":false,"source_phase":"completed_turn","type":"end_turn"}</watcher_event>';
const content = (text: string) => ({
  Type: 'text' as const,
  Text: text,
  Data: null,
  MIME: '',
  URL: '',
  Filename: '',
  AudioID: '',
});
const feedback =
  '## Review result\n\n**Run the tests** before finishing.\n\n- Keep the current workspace.\n- Verify the result.';
const input = `${event}\nReview the saved task.\n<todo>[{"ID":"todo-1","Title":"Verify tests","Status":"pending"}]</todo>\n<doing>{"Title":"Check changes","Description":"Read the results."}</doing>\n<conversation_to_analize>${JSON.stringify(
  [
    {
      Role: 'user',
      Content: [content('Check **behavior**. <img src=x onerror="window.watcherUnsafe=true">')],
    },
    {
      Role: 'assistant',
      Content: [content('Reading the project file.')],
      InProgress: true,
      ToolCalls: [
        {
          ID: 'read-1',
          Name: 'file_actions',
          Input: { path: 'README.md', actions: [{ op: 'read' }] },
        },
      ],
    },
    { Role: 'tool', ToolCallID: 'read-1', Content: [content('Project instructions are present.')] },
  ],
)}</conversation_to_analize>`;

function runRecord(sessionID: string, workspaceID: string, id = 'saved-review'): WatcherRun {
  return {
    id,
    workspace_id: workspaceID,
    session_id: sessionID,
    definition_id: 'reviewer',
    name: 'Completion reviewer',
    trigger: 'end_turn',
    status: 'completed',
    decision: 'continue',
    message: '',
    reason: feedback,
    error: '',
    worker_session_id: 'worker-session',
    model: 'test/test-model',
    created_at: new Date().toISOString(),
    finished_at: new Date().toISOString(),
    duration_ms: 1250,
    anchor_id: 'answer',
    anchor_sequence: 1,
    input_preview: input,
    input_truncated: false,
    input_tokens: 1600,
    estimated: true,
  };
}

async function openActivity(page: Page) {
  await openWorkbenchAction(page, 'Workspace settings');
  await page.getByRole('tab', { name: 'Watchers', exact: true }).click();
  await page
    .getByRole('tablist', { name: 'Watcher sections' })
    .getByRole('tab', { name: 'Activity', exact: true })
    .click();
}

test('format saved and live watcher reviews and preserve original input on demand', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page),
    watchers = await mockWatchers(page);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.goto('/');
  await connectAndCreate(page);
  const session = fixture.sessions[0],
    workspace = fixture.instances[0];
  fixture.histories.set(session.ID, [
    {
      ID: 'answer',
      SessionID: session.ID,
      Role: 'assistant',
      Content: [content('Initial answer')],
      ToolCalls: [],
      ToolCallID: '',
      Usage: null,
      CreatedAt: new Date().toISOString(),
      Seq: 1,
    } satisfies Message,
  ]);
  const record = runRecord(session.ID, workspace.ID);
  watchers.activity.set(session.ID, {
    Name: 'watcher',
    SessionID: session.ID,
    Revision: 1,
    HistoryRevision: 0,
    Entries: [
      {
        ID: record.id,
        SessionID: session.ID,
        Title: record.name,
        AnchorID: 'answer',
        AnchorSequence: 1,
        Notes: [],
        CreatedAt: record.created_at,
        Estimated: true,
        Outcome: 'running',
        Trigger: 'end_turn',
        InputPreview: '',
        DurationMS: 0,
      },
    ],
  });
  fixture.state(session);
  fixture.event(session, 'plugin.activity', {
    plugin: 'watcher',
    run_id: record.id,
    status: 'running',
  });
  const review = page.locator(`[data-watcher-run="${record.id}"]`);
  await expect(review).toBeVisible();
  await expect(review.locator('[data-watcher-details]')).toHaveCount(1);
  await review.locator('.activity-toggle').first().click();
  await expect(review.locator('[data-watcher-details]')).toHaveCount(0);
  await review.locator('.activity-toggle').first().click();
  await expect(review.getByText('Reviewing', { exact: true }).first()).toBeVisible();
  await expect(review.getByTitle('0 ms', { exact: true })).toHaveText('0 ms');
  const activity = watchers.activity.get(session.ID)!;
  activity.Revision = 2;
  Object.assign(activity.Entries![0], {
    Outcome: 'completed',
    Decision: 'continue',
    InputPreview: input,
    Notes: [{ Text: feedback, References: [] }],
    DurationMS: 1250,
  });
  fixture.event(session, 'plugin.activity', {
    plugin: 'watcher',
    run_id: record.id,
    status: 'completed',
  });
  await expect(review.locator('.activity-toggle').first()).toHaveAttribute(
    'aria-expanded',
    'false',
  );
  await review.locator('.activity-toggle').first().click();
  await expect(review.getByRole('heading', { name: 'Review result' })).toBeVisible();
  await expect(review.locator('strong').filter({ hasText: 'Run the tests' })).toBeVisible();
  await expect(review.getByText('AI finish', { exact: true })).toBeVisible();
  await expect(review.getByText('Completed turn', { exact: true })).toBeVisible();
  await expect(review.getByText('Context at trigger', { exact: true })).toHaveCount(0);
  await review.getByRole('button', { name: 'Review input', exact: true }).click();
  const preview = review.getByRole('region', { name: 'Watcher input preview' });
  await expect(preview.getByText('Verify tests', { exact: true })).toBeVisible();
  await expect(preview.getByText('In progress', { exact: true })).toBeVisible();
  await expect(
    preview
      .getByRole('region', { name: 'Referenced tool file_actions' })
      .getByText('README.md', { exact: true }),
  ).toBeVisible();
  await expect(preview.getByText('<watcher_event>', { exact: false })).toHaveCount(0);
  expect(await page.evaluate(() => Reflect.get(window, 'watcherUnsafe'))).toBeUndefined();
  await preview.getByRole('button', { name: 'Review input actions', exact: true }).click();
  await page.getByRole('menuitem', { name: 'Show original preview', exact: true }).click();
  expect(await preview.getByLabel('Original watcher input preview').textContent()).toBe(input);
  await preview.getByRole('button', { name: 'Review input actions', exact: true }).click();
  const download = page.waitForEvent('download');
  await page.getByRole('menuitem', { name: 'Download original preview', exact: true }).click();
  const stream = await (await download).createReadStream();
  const chunks: Buffer[] = [];
  for await (const chunk of stream!) chunks.push(Buffer.from(chunk));
  expect(Buffer.concat(chunks).toString('utf8')).toBe(input);
  await page.reload();
  await expect(review).toBeVisible();
  await review.getByRole('button').click();
  await expect(review.getByRole('heading', { name: 'Review result' })).toBeVisible();
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 900 });
  await review.getByRole('button', { name: 'Review input', exact: true }).click();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('watcher-review-formatted.png') });
  expect(watchers.updates).toEqual([]);
  expect(errors).toEqual([]);
});

test('remove the extra Watcher reviews group while retaining the complete activity history', async ({
  page,
}) => {
  const fixture = await mockHarness(page),
    watchers = await mockWatchers(page);
  await page.goto('/');
  await connectAndCreate(page);
  const session = fixture.sessions[0],
    workspace = fixture.instances[0],
    record = runRecord(session.ID, workspace.ID, 'earlier-review');
  record.anchor_id = 'outside-window';
  record.name = 'Earlier completion reviewer';
  watchers.activity.set(session.ID, {
    Name: 'watcher',
    SessionID: session.ID,
    Revision: 1,
    HistoryRevision: 0,
    Entries: [
      {
        ID: record.id,
        SessionID: session.ID,
        Title: record.name,
        AnchorID: record.anchor_id,
        AnchorSequence: 1,
        Notes: [{ Text: feedback, References: [] }],
        CreatedAt: record.created_at,
        Estimated: true,
        Outcome: 'completed',
        Trigger: record.trigger,
        InputPreview: record.input_preview,
      },
    ],
  });
  watchers.runs.set(workspace.ID, [record]);
  fixture.event(session, 'plugin.activity', {
    plugin: 'watcher',
    run_id: record.id,
    status: 'completed',
  });
  await expect(page.getByRole('heading', { name: 'What are we building today?' })).toBeVisible();
  await expect(page.getByRole('button', { name: /^Watcher reviews/ })).toHaveCount(0);
  await expect(page.locator('[data-watcher-run="earlier-review"]')).toHaveCount(0);
  await openActivity(page);
  const panel = page.getByRole('region', { name: 'Watcher settings' });
  await panel.getByRole('button', { name: /Earlier completion reviewer/ }).click();
  const details = panel.locator('[data-watcher-details="earlier-review"]');
  await expect(details.getByRole('heading', { name: 'Review result' })).toBeVisible();
  await details.getByRole('button', { name: 'Review input', exact: true }).click();
  await expect(details.getByText('Verify tests', { exact: true })).toBeVisible();
  await expect(details.getByText('<watcher_event>', { exact: false })).toHaveCount(0);
  expect(watchers.updates).toEqual([]);
});

test('show bounded truncated input clearly and render legacy reviews without an event envelope', async ({
  page,
}) => {
  const fixture = await mockHarness(page),
    watchers = await mockWatchers(page);
  await page.goto('/');
  await connectAndCreate(page);
  const session = fixture.sessions[0],
    workspace = fixture.instances[0],
    record = runRecord(session.ID, workspace.ID);
  record.status = 'error';
  record.decision = '';
  record.error = 'Review interrupted\n' + 'Reported diagnostic detail.\n'.repeat(120);
  record.reason = '';
  record.input_truncated = true;
  record.input_preview = `${event}\n<conversation_to_analize>[${JSON.stringify({ Role: 'user', Content: [content('Complete source message')] })},\n{"Role":"assistant","Content":[{"Type":"text","Text":"incomplete`;
  const legacy = runRecord(session.ID, workspace.ID, 'legacy-review');
  legacy.name = 'Legacy reviewer';
  legacy.input_preview = 'Read the saved task.';
  watchers.runs.set(workspace.ID, [record, legacy]);
  await openActivity(page);
  const panel = page.getByRole('region', { name: 'Watcher settings' });
  await panel.getByRole('button', { name: /Completion reviewer/ }).click();
  const details = panel.locator('[data-watcher-details="saved-review"]');
  const reportedError = details.getByRole('status');
  await expect(reportedError).toContainText('Review interrupted');
  expect((await reportedError.boundingBox())!.height).toBeLessThanOrEqual(208);
  expect(await reportedError.textContent()).toContain(record.error);
  await details.getByRole('button', { name: 'Review input', exact: true }).click();
  await expect(details.getByText('Complete source message', { exact: true })).toBeVisible();
  await expect(
    details.getByText('This preview ends inside a message.', { exact: false }),
  ).toBeVisible();
  await panel.getByRole('button', { name: /Legacy reviewer/ }).click();
  const earlier = panel.locator('[data-watcher-details="legacy-review"]');
  await earlier.getByRole('button', { name: 'Review input', exact: true }).click();
  await expect(earlier.getByText('Read the saved task.', { exact: true })).toBeVisible();
  await expect(earlier.getByText('AI finish', { exact: true })).toBeVisible();
});
