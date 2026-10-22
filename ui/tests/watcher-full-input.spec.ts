import { test, expect, type Page } from '@playwright/test';
import type { Message, Session } from '../src/lib/atoms/types';
import { mockHarness } from './api-fixture';
import { mockWatchers } from './watcher-fixture';
import { connectAndCreate } from './helpers';

async function setupInput(page: Page, contextRows = 1400) {
  const fixture = await mockHarness(page),
    watchers = await mockWatchers(page);
  await connectAndCreate(page);
  const parent = fixture.sessions[0];
  const worker: Session = {
    ...parent,
    ID: 'input-worker',
    Parent: parent.ID,
    Completed: true,
    HistoryRevision: 7,
  };
  fixture.sessions.push(worker);
  const message = (session: Session, id: string, role: Message['Role'], text: string): Message => ({
    ID: id,
    SessionID: session.ID,
    Seq: 0,
    Role: role,
    Content: [
      { Type: 'text', Text: text, Data: null, URL: '', MIME: '', Filename: '', AudioID: '' },
    ],
    ToolCalls: null,
    ToolCallID: '',
    Usage: null,
    CreatedAt: new Date().toISOString(),
  });
  const full =
    '<watcher_event>{"type":"end_turn","source_phase":"completed_turn"}</watcher_event>\n' +
    'Fixed review instruction.\n'.repeat(600) +
    '<conversation_to_analize>\nContext excerpt: earlier messages omitted.\n\nAssistant:\nTool call: bash\ncommand: sed ' +
    'visible context data\n'.repeat(contextRows) +
    '\nFINAL CONTEXT MARKER 🌍\n</conversation_to_analize>\n';
  const preview = full.slice(0, 16384);
  fixture.histories.set(parent.ID, [
    message(parent, 'review-anchor', 'assistant', 'The work is ready for review.'),
  ]);
  fixture.histories.set(worker.ID, [
    message(worker, 'review-task', 'user', full),
    ...Array.from({ length: 150 }, (_, index) =>
      message(worker, `output-${index}`, 'assistant', 'Later reviewer output.'),
    ),
  ]);
  watchers.activity.set(parent.ID, {
    Name: 'watcher',
    SessionID: parent.ID,
    Revision: 1,
    HistoryRevision: parent.HistoryRevision ?? 0,
    NextCursor: '',
    Entries: [
      {
        ID: 'input-review',
        SessionID: parent.ID,
        AnchorID: 'review-anchor',
        AnchorSequence: 1,
        Title: 'Input reviewer',
        CreatedAt: new Date().toISOString(),
        Trigger: 'end_turn',
        Outcome: 'completed',
        Decision: 'continue',
        WorkerSessionID: worker.ID,
        InputPreview: preview,
        InputTruncated: true,
        InputTokens: 12405,
        Estimated: true,
        Notes: [{ Text: 'Review complete.', References: [] }],
      },
    ],
  });
  await page.route(`**/sessions/${parent.ID}/agents`, (route) => route.fulfill({ json: [] }));
  fixture.event(parent, 'model.call', { model: fixture.model.ID });
  const review = page.locator('[data-watcher-run="input-review"]');
  await expect(review).toBeVisible();
  return { fixture, watchers, parent, worker, full, preview, review };
}

test('inspect the exact full prompt through two bounded reads instead of the 16 KiB preview', async ({
  page,
}, testInfo) => {
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 800 });
  const { fixture, watchers, parent, worker, full, review } = await setupInput(page);
  const reads: URL[] = [];
  page.on('request', (request) => {
    const url = new URL(request.url());
    if (url.pathname.endsWith(`/sessions/${worker.ID}/history`)) reads.push(url);
  });
  await review.locator('.activity-toggle').click();
  await review.getByRole('button', { name: 'Review input', exact: true }).click();
  const inspector = review.getByLabel('Reviewer prompt inspector');
  await expect(inspector.getByText(/first 16 KiB/)).toBeVisible();
  await expect(inspector.getByRole('button', { name: 'Prompt text', exact: true })).toHaveAttribute(
    'aria-expanded',
    'false',
  );
  await expect(inspector.getByText('FINAL CONTEXT MARKER 🌍')).toHaveCount(0);
  expect(reads).toHaveLength(0);
  await inspector.getByRole('button', { name: 'Load full prompt' }).click();
  await expect(inspector.getByText(/Full saved prompt/)).toBeVisible();
  await expect(inspector.getByText('FINAL CONTEXT MARKER 🌍', { exact: false })).toBeVisible();
  expect(reads).toHaveLength(2);
  expect(reads[0].searchParams.get('metadata')).toBe('true');
  expect(reads[1].searchParams.get('limit')).toBe('1');
  expect(reads[1].searchParams.get('revision')).toBe('7');
  expect(Number(reads[1].searchParams.get('before'))).toBe(
    fixture.historyPage(worker).TaskSeq! + 1,
  );

  const activity = watchers.activity.get(parent.ID)!;
  activity.Revision++;
  activity.Entries![0].DurationMS = 17;
  fixture.event(parent, 'plugin.activity', { plugin: 'watcher' });
  await expect(review.getByTitle('17 ms', { exact: true })).toBeVisible();
  await expect(inspector.getByText(/Full saved prompt/)).toBeVisible();
  expect(reads).toHaveLength(2);
  await inspector.getByRole('button', { name: 'Review input actions' }).click();
  await page.getByRole('menuitem', { name: 'Show original prompt', exact: true }).click();
  expect(await inspector.getByLabel('Original watcher prompt').textContent()).toBe(full);
  await inspector.getByRole('button', { name: 'Review input actions' }).click();
  const download = page.waitForEvent('download');
  await page.getByRole('menuitem', { name: 'Download full prompt', exact: true }).click();
  const stream = await (await download).createReadStream();
  const chunks: Buffer[] = [];
  for await (const chunk of stream!) chunks.push(Buffer.from(chunk));
  expect(Buffer.concat(chunks).toString('utf8')).toBe(full);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await review.getByRole('button', { name: 'Review input', exact: true }).click();
  await expect(review.getByLabel('Reviewer prompt inspector')).toHaveCount(0);
  await review.getByRole('button', { name: 'Review input', exact: true }).click();
  await expect(review.getByRole('button', { name: 'Load full prompt' })).toBeVisible();
  expect(reads).toHaveLength(2);
});

test('bound large prompt rendering while downloads retain the full text', async ({ page }) => {
  const { full, review } = await setupInput(page, 6000);
  expect(full.length).toBeGreaterThan(100000);
  await review.locator('.activity-toggle').click();
  await review.getByRole('button', { name: 'Review input', exact: true }).click();
  await review.getByRole('button', { name: 'Load full prompt' }).click();
  await expect(review.getByText(/Full saved prompt/)).toBeVisible();
  await expect(review.getByText(/Display limited to 100,000 characters/)).toBeVisible();
  await review.getByRole('button', { name: 'Review input actions' }).click();
  await page.getByRole('menuitem', { name: 'Show original prompt', exact: true }).click();
  expect(
    (await review.getByLabel('Original watcher prompt').textContent())!.length,
  ).toBeLessThanOrEqual(100000);
  await review.getByRole('button', { name: 'Review input actions' }).click();
  const download = page.waitForEvent('download');
  await page.getByRole('menuitem', { name: 'Download full prompt', exact: true }).click();
  const stream = await (await download).createReadStream();
  const chunks: Buffer[] = [];
  for await (const chunk of stream!) chunks.push(Buffer.from(chunk));
  expect(Buffer.concat(chunks).toString('utf8')).toBe(full);
});

test('keep the preview when history changes between metadata and input reads', async ({ page }) => {
  const { fixture, worker, review } = await setupInput(page);
  let invalidated = false;
  await page.route(`**/sessions/${worker.ID}/history?*`, async (route) => {
    if (new URL(route.request().url()).searchParams.has('before') && !invalidated) {
      worker.HistoryRevision = 8;
      invalidated = true;
    }
    await route.fallback();
  });
  await review.locator('.activity-toggle').click();
  await review.getByRole('button', { name: 'Review input', exact: true }).click();
  await review.getByRole('button', { name: 'Load full prompt' }).click();
  await expect(review.getByRole('alert')).toBeVisible();
  await expect(review.getByText(/Full saved prompt/)).toHaveCount(0);
  await review.getByRole('button', { name: 'Load full prompt' }).click();
  await expect(review.getByText(/Full saved prompt/)).toBeVisible();
  expect(fixture.running.has(worker.ID)).toBe(false);
});

test('cancel a full-input read on collapse and discard its late result', async ({ page }) => {
  const { fixture, worker, review } = await setupInput(page);
  let release!: () => void, started!: () => void;
  const held = new Promise<void>((resolve) => {
      release = resolve;
    }),
    requested = new Promise<void>((resolve) => {
      started = resolve;
    });
  await page.route(`**/sessions/${worker.ID}/history?*`, async (route) => {
    const query = new URL(route.request().url()).searchParams;
    if (!query.has('before')) return route.fallback();
    started();
    await held;
    await route.fulfill({ json: fixture.historyPage(worker, query) }).catch(() => {});
  });
  await review.locator('.activity-toggle').click();
  const inputToggle = review.getByRole('button', { name: 'Review input', exact: true });
  await inputToggle.click();
  await review.getByRole('button', { name: 'Load full prompt' }).click();
  await requested;
  await inputToggle.click();
  await expect(review.getByLabel('Reviewer prompt inspector')).toHaveCount(0);
  await inputToggle.click();
  release();
  await expect(review.getByRole('button', { name: 'Load full prompt' })).toBeVisible();
  await expect(review.getByText(/Full saved prompt/)).toHaveCount(0);
});

test('reject an input body that differs from the recorded preview', async ({ page }) => {
  const { fixture, worker, review } = await setupInput(page);
  fixture.histories.get(worker.ID)![0].Content![0].Text = 'A different saved task.';
  await review.locator('.activity-toggle').click();
  await review.getByRole('button', { name: 'Review input', exact: true }).click();
  await review.getByRole('button', { name: 'Load full prompt' }).click();
  await expect(review.getByRole('alert')).toContainText(
    'does not match the recorded review preview',
  );
  await expect(review.getByText(/Full saved prompt/)).toHaveCount(0);
});
