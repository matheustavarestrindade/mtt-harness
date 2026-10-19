import { test, expect } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { mockWatchers } from './watcher-fixture';
import { connectAndCreate, openWorkbenchAction, chooseSelectOption } from './helpers';

const textContent = (text: string) => ({
  Type: 'text' as const,
  Text: text,
  Data: null,
  MIME: '',
  URL: '',
  Filename: '',
  AudioID: '',
});

async function openWatchers(page: import('@playwright/test').Page) {
  await openWorkbenchAction(page, 'Settings');
  await page
    .getByRole('navigation', { name: 'Settings sections' })
    .getByRole('button', { name: 'Watchers', exact: true })
    .click();
  await expect(page.getByRole('region', { name: 'Watcher settings' })).toBeVisible();
}

for (const nativeUUID of [true, false]) {
  test(`configure, preview, duplicate and remove workspace watchers${nativeUUID ? '' : ' without randomUUID'}`, async ({
    page,
  }) => {
    if (!nativeUUID) {
      await page.addInitScript(() => {
        Object.defineProperty(window.crypto, 'randomUUID', {
          value: undefined,
          configurable: true,
        });
      });
    }
    const pageErrors: string[] = [];
    page.on('pageerror', (error) => pageErrors.push(error.message));
    const fixture = await mockHarness(page),
      watchers = await mockWatchers(page);
    await page.goto('/');
    await connectAndCreate(page);
    await openWatchers(page);
    await page.getByRole('button', { name: 'New watcher', exact: true }).click();
    const editor = page.getByRole('form', { name: 'Watcher editor' });
    await expect(editor).toBeVisible();
    await editor.getByLabel('Name', { exact: true }).fill('Completion reviewer');
    await chooseSelectOption(
      editor.getByRole('combobox', { name: 'Watcher model' }),
      'test/test-model',
    );
    await editor.getByLabel('Context growth', { exact: true }).check();
    await editor.getByLabel('Permission request', { exact: true }).check();
    await editor.getByLabel('Only paths outside the workspace').check();
    const selector = editor.getByRole('group', { name: 'Tool access' });
    await expect(selector.getByLabel('read', { exact: true })).toBeChecked();
    await expect(selector.getByLabel('write', { exact: true })).not.toBeChecked();
    await expect(selector.getByLabel('search_memory', { exact: true })).toBeChecked();
    await selector.getByLabel('bash', { exact: true }).check();
    await editor
      .getByLabel('Prompt', { exact: true })
      .fill(
        'Review {todo} {doing}.\n<conversation_to_analize>{last_32k_context}</conversation_to_analize>',
      );
    await editor.getByRole('button', { name: 'Preview input' }).click();
    await expect(editor.getByText('Supplied context', { exact: false })).toBeVisible();
    expect(watchers.actions.at(-1)?.action).toBe('preview');
    await editor.getByRole('button', { name: 'Save watcher' }).click();
    await expect(editor).toHaveCount(0);
    const saved = watchers.states.get(fixture.instances[0].ID)!.Configuration.watchers[0];
    expect(saved.id).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
    );
    expect(saved.model).toBe('test/test-model');
    expect(saved.tools.find((tool) => tool.name === 'file_actions')?.operations).toEqual([
      'read',
      'list',
      'glob',
    ]);
    expect(saved.tools.some((tool) => tool.name === 'bash')).toBe(true);
    expect(saved.triggers).toHaveLength(3);
    await page.getByRole('button', { name: 'Enable Watchers', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Disable Watchers', exact: true })).toBeVisible();
    const card = page.getByRole('article', { name: 'Watcher Completion reviewer', exact: true });
    await card.getByRole('button', { name: 'Duplicate', exact: true }).click();
    await page
      .getByRole('form', { name: 'Watcher editor' })
      .getByRole('button', { name: 'Save watcher' })
      .click();
    await expect(
      page.getByRole('article', { name: 'Watcher Completion reviewer copy', exact: true }),
    ).toBeVisible();
    expect(watchers.states.get(fixture.instances[0].ID)!.Configuration.watchers[1].id).not.toBe(
      saved.id,
    );
    expect(watchers.states.get(fixture.instances[0].ID)!.Configuration.watchers[1].id).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
    );
    await card.getByRole('button', { name: 'Remove', exact: true }).click();
    await card.getByRole('button', { name: 'Remove watcher', exact: true }).click();
    await expect(card).toHaveCount(0);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    expect(pageErrors).toEqual([]);
  });
}

test('show watcher activity and avoid duplicating runtime guidance as a chat message', async ({
  page,
}) => {
  const fixture = await mockHarness(page),
    watchers = await mockWatchers(page);
  await page.goto('/');
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  fixture.histories.set(session.ID, [
    {
      ID: 'answer',
      SessionID: session.ID,
      Role: 'assistant',
      Content: [textContent('Initial answer')],
      ToolCalls: [],
      ToolCallID: '',
      Usage: null,
      CreatedAt: new Date().toISOString(),
      Seq: 1,
    },
    {
      ID: 'correction',
      SessionID: session.ID,
      Role: 'runtime',
      Content: [
        textContent(
          '[watcher]\n' +
            JSON.stringify({
              type: 'watcher.intervention',
              run_ids: ['review-1'],
              notes: [
                { id: 'watcher-1', name: 'Completion reviewer', message: 'Verify the result.' },
              ],
            }),
        ),
      ],
      ToolCalls: [],
      ToolCallID: '',
      Usage: null,
      CreatedAt: new Date().toISOString(),
      Seq: 2,
    },
  ]);
  watchers.activity.set(session.ID, {
    Name: 'watcher',
    SessionID: session.ID,
    Revision: 1,
    HistoryRevision: 0,
    Entries: [
      {
        ID: 'review-1',
        SessionID: session.ID,
        Title: 'Completion reviewer',
        AnchorID: 'answer',
        AnchorSequence: 1,
        Notes: [{ Text: 'Verify the result.', References: [] }],
        CreatedAt: new Date().toISOString(),
        Estimated: true,
        Outcome: 'intervened',
        Trigger: 'end_turn',
        InputPreview: 'Review task context',
        DurationMS: 90,
      },
    ],
  });
  fixture.state(session);
  fixture.event(session, 'plugin.activity', {
    plugin: 'watcher',
    run_id: 'review-1',
    status: 'intervened',
  });
  const review = page.locator('[data-watcher-run="review-1"]');
  await expect(review).toBeVisible();
  await review.getByRole('button').click();
  await expect(review.getByText('Verify the result.', { exact: true })).toBeVisible();
  await expect(page.getByText('[watcher]', { exact: false })).toHaveCount(0);
  await openWatchers(page);
  await page
    .getByRole('navigation', { name: 'Watcher sections' })
    .getByRole('button', { name: 'Activity', exact: true })
    .click();
  await expect(page.getByRole('button', { name: 'Refresh activity' })).toBeVisible();
  expect(watchers.actions.some((action) => action.action === 'runs')).toBe(true);
});
