import { expect, test } from '@playwright/test';
import type { Message, Session } from '../src/lib/atoms/types';
import type { PluginActivityEntry, PluginActivityPage } from '../src/lib/atoms/plugin-activity';
import { placePluginActivity } from '../src/lib/atoms/plugin-activity';
import { buildConversationEntries } from '../src/lib/atoms/transcript';
import { mockHarness } from './api-fixture';
import { connectAndCreate, openWorkbenchAction } from './helpers';

function message(
  session: Session,
  ID: string,
  text: string,
  overrides: Partial<Message> = {},
): Message {
  return {
    ID,
    SessionID: session.ID,
    Seq: 1,
    Role: 'assistant',
    Content: [
      { Type: 'text', Text: text, Data: null, MIME: '', URL: '', Filename: '', AudioID: '' },
    ],
    ToolCalls: null,
    ToolCallID: '',
    Usage: null,
    CreatedAt: '2026-01-01T12:00:00Z',
    ...overrides,
  };
}
function hint(session: Session, ID: string, AnchorID: string): PluginActivityEntry {
  return {
    ID,
    SessionID: session.ID,
    AnchorID,
    AnchorSequence: 2,
    Title: 'Check persistence ownership',
    CreatedAt: '2026-01-01T12:00:01Z',
    Notes: [
      {
        Text: 'Use **transaction-scoped** session gates. <script>window.injected=true</script>',
        References: ['file: internal/store/events.go:12-30', 'memory: session persistence policy'],
      },
    ],
    InputTokens: 210,
    Estimated: true,
  };
}
function activity(
  session: Session,
  entries: PluginActivityEntry[],
  revision = 1,
  historyRevision = 0,
): PluginActivityPage {
  return {
    Name: 'sidekick',
    SessionID: session.ID,
    Revision: revision,
    HistoryRevision: historyRevision,
    Entries: entries,
  };
}

test('place Sidekick notes after their exact tool-result owner, without fabricating chat messages', () => {
  const session = { ID: 'root', InstanceID: 'workspace' } as Session;
  const history = [
    message(session, 'first', '', { ToolCalls: [{ ID: 'reused', Name: 'task_state', Input: {} }] }),
    message(session, 'result', 'Done', { Role: 'tool', ToolCallID: 'reused' }),
    message(session, 'second', '', {
      ToolCalls: [{ ID: 'reused', Name: 'task_state', Input: {} }],
    }),
  ];
  const original = JSON.stringify(history);
  const plan = placePluginActivity(buildConversationEntries(history), [
    hint(session, 'hint', 'result'),
    hint({ ...session, ID: 'other' }, 'foreign', 'result'),
  ]);
  expect(plan.anchored.get('first')?.map((entry) => entry.ID)).toEqual(['hint']);
  expect(plan.anchored.has('second')).toBe(false);
  expect(plan.unplaced.map((entry) => entry.ID)).toEqual(['foreign']);
  expect(JSON.stringify(history)).toBe(original);
});

test('show compact cited Sidekick disclosures, preserve their open state, and recover on refresh', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  fixture.histories.set(session.ID, [
    message(session, 'task-call', '', {
      ToolCalls: [
        { ID: 'task', Name: 'task_state', Input: { doing: { title: 'Check persistence' } } },
      ],
    }),
    message(session, 'task-result', 'Updated', { Role: 'tool', ToolCallID: 'task' }),
    message(session, 'answer', 'Checking the storage boundary.'),
  ]);
  fixture.sidekickActivity.set(
    session.ID,
    activity(session, [hint(session, 'first-hint', 'task-result')]),
  );
  fixture.event(session, 'model.call', { message_id: 'answer', model: session.Model });
  const entry = page.locator('[data-sidekick-id="first-hint"]');
  await expect(entry).toBeVisible();
  await expect(entry.getByRole('button').first()).toHaveAttribute('aria-expanded', 'false');
  await entry.getByRole('button').first().click();
  await expect(entry).toContainText('transaction-scoped');
  await expect(entry).toContainText('internal/store/events.go:12-30');
  await expect(entry).toContainText('≈ 210 input tokens');
  await expect(entry.locator('script')).toHaveCount(0);
  expect(
    await page.evaluate(() => (window as unknown as { injected?: boolean }).injected),
  ).toBeUndefined();
  const order = await page
    .locator('[data-call-id="task"], [data-sidekick-id="first-hint"]')
    .evaluateAll((elements) =>
      elements.map(
        (element) =>
          element.getAttribute('data-call-id') ?? element.getAttribute('data-sidekick-id'),
      ),
    );
  expect(order).toEqual(['task', 'first-hint']);
  fixture.sidekickActivity.set(
    session.ID,
    activity(
      session,
      [hint(session, 'first-hint', 'task-result'), hint(session, 'next-hint', 'answer')],
      2,
    ),
  );
  fixture.event(session, 'model.call', { message_id: 'next-response', model: session.Model });
  await expect(page.locator('[data-sidekick-id="next-hint"]')).toBeVisible();
  await expect(entry.getByRole('button').first()).toHaveAttribute('aria-expanded', 'true');
  await page.reload();
  await expect(page.locator('[data-sidekick-id="first-hint"]')).toBeVisible();
  await page.locator('[data-sidekick-id="first-hint"]').getByRole('button').first().click();
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 640 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('sidekick-disclosure.png'), fullPage: true });
});

test('keep earlier hints in a collapsed group, paginate, and remove reverted deliveries', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  fixture.histories.set(session.ID, [message(session, 'current', 'Current conversation')]);
  let reverted = false;
  await page.route(`**/sessions/${session.ID}/plugins/sidekick/activity?*`, (route) => {
    const cursor = new URL(route.request().url()).searchParams.get('cursor');
    const result = reverted
      ? activity(session, [], 9, 1)
      : cursor
        ? activity(session, [hint(session, 'older-hint', 'archived')], 4)
        : {
            ...activity(session, [hint(session, 'recent-hint', 'current')], 4),
            NextCursor: 'older-page',
          };
    return route.fulfill({ json: result });
  });
  fixture.event(session, 'model.call', { message_id: 'live', model: session.Model });
  await expect(page.locator('[data-sidekick-id="recent-hint"]')).toBeVisible();
  const earlier = page.getByRole('group', { name: 'Earlier Sidekick context', exact: true });
  await earlier.locator('summary').click();
  await earlier.getByRole('button', { name: 'Older Sidekick entries', exact: true }).click();
  await expect(page.locator('[data-sidekick-id="older-hint"]')).toBeVisible();
  reverted = true;
  fixture.event(session, 'run.end', {});
  await expect(page.locator('[data-sidekick-id]')).toHaveCount(0);
});

test('scope activity reads to the selected session and its open agent preview', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const parent = fixture.sessions[0];
  await expect
    .poll(() =>
      [...fixture.openSockets].some((socket) => socket.url().includes(`/sessions/${parent.ID}/`)),
    )
    .toBe(true);
  const child: Session = {
    ...parent,
    ID: 'child-sidekick',
    Parent: parent.ID,
    Depth: 1,
    Completed: false,
  };
  fixture.sessions.push(child);
  fixture.histories.set(parent.ID, [
    message(parent, 'delegation', '', {
      ToolCalls: [
        { ID: 'delegate', Name: 'agent', Input: { task: 'Inspect context', model: parent.Model } },
      ],
    }),
  ]);
  fixture.histories.set(child.ID, [message(child, 'child-source', 'Child is checking sources')]);
  fixture.sidekickActivity.set(
    child.ID,
    activity(child, [hint(child, 'child-hint', 'child-source')]),
  );
  fixture.event(parent, 'model.call', { message_id: 'delegation', model: parent.Model });
  fixture.event(parent, 'agent.start', { session: child.ID, task: 'Inspect context' });
  const childPanel = page.locator('[data-agent-id="child-sidekick"]');
  await expect(childPanel.locator('[data-sidekick-id="child-hint"]')).toBeVisible();
  await openWorkbenchAction(page, 'New session');
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Create session', exact: true })
    .click();
  await expect(page.locator('[data-sidekick-id]')).toHaveCount(0);
  await expect(page.getByLabel('Message', { exact: true })).toBeEnabled();
});

test('ignore a delayed Sidekick response after session navigation', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const previous = fixture.sessions[0];
  fixture.histories.set(previous.ID, [message(previous, 'old-source', 'Previous conversation')]);
  let release!: () => void;
  const delayed = new Promise<void>((resolve) => (release = resolve));
  let entered!: () => void;
  const requested = new Promise<void>((resolve) => (entered = resolve));
  await page.route(`**/sessions/${previous.ID}/plugins/sidekick/activity?*`, async (route) => {
    entered();
    await delayed;
    await route
      .fulfill({ json: activity(previous, [hint(previous, 'late-hint', 'old-source')], 90) })
      .catch(() => {});
  });
  fixture.event(previous, 'model.call', { message_id: 'old-reply', model: previous.Model });
  await requested;
  await openWorkbenchAction(page, 'New session');
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Create session', exact: true })
    .click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  release();
  await expect(page.locator('[data-sidekick-id="late-hint"]')).toHaveCount(0);
  await expect(page.getByLabel('Message', { exact: true })).toBeEnabled();
});
