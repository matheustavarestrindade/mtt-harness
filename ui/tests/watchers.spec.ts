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
  await openWorkbenchAction(page, 'Workspace settings');
  await page
    .getByRole('navigation', { name: 'Settings sections' })
    .getByRole('tab', { name: 'Watchers', exact: true })
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
    await editor.getByRole('button', { name: 'Run on', exact: true }).click();
    const events = page.getByRole('listbox');
    await expect(events).toHaveAttribute('aria-multiselectable', 'true');
    await events.getByRole('option', { name: 'Context growth', exact: true }).click();
    await events.getByRole('option', { name: 'Permission request', exact: true }).click();
    await expect(events.getByRole('option', { name: 'AI finish', exact: true })).toHaveAttribute(
      'aria-selected',
      'true',
    );
    await page.keyboard.press('Escape');
    await editor.getByRole('switch', { name: 'Only paths outside the workspace' }).click();
    await editor.getByRole('button', { name: 'Select watcher tools' }).click();
    const selector = page.getByRole('menu', { name: 'Watcher tools', exact: true });
    await expect(
      selector.getByRole('menuitemcheckbox', { name: 'search_memory', exact: true }),
    ).toBeChecked();
    await selector.getByRole('menuitemcheckbox', { name: 'bash', exact: true }).click();
    await expect(
      selector.getByRole('menuitemcheckbox', { name: 'bash', exact: true }),
    ).toBeChecked();
    await selector.getByRole('menuitem', { name: 'file_actions operations', exact: true }).click();
    const operations = page.getByRole('menu', { name: 'file_actions operations', exact: true });
    await expect(
      operations.getByRole('menuitemcheckbox', { name: 'read', exact: true }),
    ).toBeChecked();
    await expect(
      operations.getByRole('menuitemcheckbox', { name: 'write', exact: true }),
    ).not.toBeChecked();
    await page.keyboard.press('Escape');
    await expect(selector).toBeHidden();
    await editor
      .getByLabel('Prompt', { exact: true })
      .fill(
        'Review {todo} {doing}.\n<conversation_to_analize>{last_32k_context}</conversation_to_analize>',
      );
    await editor.getByRole('button', { name: 'Preview input' }).click();
    await expect(editor.getByText('Supplied context', { exact: false })).toBeVisible();
    expect(watchers.actions.at(-1)?.action).toBe('preview');
    await editor.getByRole('button', { name: 'Save watcher settings' }).click();
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
    await page.getByRole('switch', { name: 'Watchers enabled', exact: true }).click();
    await expect(page.getByRole('switch', { name: 'Watchers enabled', exact: true })).toBeChecked();
    const card = page.getByRole('article', { name: 'Watcher Completion reviewer', exact: true });
    await card.getByRole('button', { name: 'Duplicate', exact: true }).click();
    await page
      .getByRole('form', { name: 'Watcher editor' })
      .getByRole('button', { name: 'Save watcher settings' })
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
    .getByRole('tablist', { name: 'Watcher sections' })
    .getByRole('tab', { name: 'Activity', exact: true })
    .click();
  await expect(page.getByRole('button', { name: 'Refresh Watchers' })).toBeVisible();
  expect(watchers.actions.some((action) => action.action === 'runs')).toBe(true);
});

test('align Watcher tabs and footer buttons with the Sidekick reference', async ({
  page,
}, testInfo) => {
  await mockHarness(page);
  await mockWatchers(page);
  if (page.viewportSize()!.width < 1024) await page.setViewportSize({ width: 320, height: 900 });
  await connectAndCreate(page);
  await openWorkbenchAction(page, 'Workspace settings');
  const navigation = page.getByRole('navigation', { name: 'Settings sections' });
  await navigation.getByRole('tab', { name: 'Sidekick', exact: true }).click();
  const reference = page.getByRole('button', { name: 'Refresh Sidekick', exact: true });
  await expect(reference).toBeVisible();
  const referenceFont = await reference.evaluate((element) => getComputedStyle(element).fontSize);
  const navigationFont = await navigation
    .getByRole('tab', { name: 'Sidekick', exact: true })
    .evaluate((element) => getComputedStyle(element).fontSize);
  expect(navigationFont).toBe('12px');
  for (const label of ['Instructions', 'Watchers']) {
    expect(
      await navigation
        .getByRole('tab', { name: label, exact: true })
        .evaluate((element) => getComputedStyle(element).fontSize),
    ).toBe(navigationFont);
  }
  await navigation.getByRole('tab', { name: 'Watchers', exact: true }).click();
  const tabs = page.getByRole('tablist', { name: 'Watcher sections' });
  await expect(tabs.getByRole('tab')).toHaveText(['Watchers', 'Usage', 'Activity']);
  const rows = [
    { tab: 'Watchers', heading: 'Definitions', empty: 'No watchers configured.' },
    { tab: 'Usage', heading: 'Worker usage', empty: 'No watcher usage recorded.' },
    { tab: 'Activity', heading: 'Recent reviews', empty: 'No watcher runs yet.' },
  ];
  const positions: {
    headingX: number;
    headingY: number;
    textX: number;
    textY: number;
    footerY: number;
  }[] = [];
  for (const row of rows) {
    await tabs.getByRole('tab', { name: row.tab, exact: true }).click();
    const panel = page.getByRole('tabpanel', { name: row.tab, exact: true });
    await expect(panel.getByText(row.empty, { exact: true })).toBeVisible();
    const heading = await panel
      .getByRole('heading', { name: row.heading, exact: true })
      .boundingBox();
    const text = await panel.getByText(row.empty, { exact: true }).boundingBox();
    const tabBox = await tabs.boundingBox();
    const footer = page.getByRole('button', { name: 'Refresh Watchers', exact: true });
    const footerBox = await footer.boundingBox();
    expect(await footer.evaluate((element) => getComputedStyle(element).fontSize)).toBe(
      referenceFont,
    );
    expect(footerBox!.y).toBeGreaterThan(text!.y + text!.height);
    positions.push({
      headingX: heading!.x,
      headingY: heading!.y - tabBox!.y,
      textX: text!.x,
      textY: text!.y - tabBox!.y,
      footerY: footerBox!.y - tabBox!.y,
    });
    await page.screenshot({ path: testInfo.outputPath(`watcher-${row.tab.toLowerCase()}.png`) });
  }
  for (const position of positions.slice(1)) {
    for (const key of ['headingX', 'headingY', 'textX', 'textY', 'footerY'] as const)
      expect(Math.abs(position[key] - positions[0][key]), key).toBeLessThanOrEqual(1);
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('explain review context and support keyboard selection in shadcn menus', async ({
  page,
}, testInfo) => {
  await mockHarness(page);
  await mockWatchers(page);
  await connectAndCreate(page);
  await openWatchers(page);
  await page.getByRole('button', { name: 'New watcher', exact: true }).click();
  const editor = page.getByRole('form', { name: 'Watcher editor' });
  for (const name of ['General', 'When to run', 'Review input', 'Tool access', 'Execution limits'])
    await expect(editor.getByRole('group', { name, exact: true })).toBeVisible();
  await expect(editor.locator('input[type="checkbox"]')).toHaveCount(0);
  const help = editor.getByRole('button', { name: 'About conversation to review', exact: true });
  await help.click();
  const tooltip = page.getByRole('tooltip');
  await expect(tooltip).toBeVisible();
  await expect(tooltip).toContainText('{last_context}');
  await expect(tooltip).toContainText('Context interval controls how often');
  await expect(tooltip).toBeInViewport();
  // A tapped help control must remain readable after hover/pointer transitions.
  await page.waitForTimeout(400);
  await expect(tooltip).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('watcher-context-help.png') });
  await page.keyboard.press('Escape');
  await expect(editor).toBeVisible();
  const trigger = editor.getByRole('button', { name: 'Run on', exact: true });
  await trigger.focus();
  await trigger.press('ArrowDown');
  const events = page.getByRole('listbox');
  await expect(events).toHaveAttribute('aria-multiselectable', 'true');
  await page.keyboard.press('Home');
  await page.keyboard.press('Enter');
  await expect(events.getByRole('option', { name: 'Context growth', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  await expect(events.getByRole('option', { name: 'AI finish', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  await page.keyboard.press('Escape');
  await expect(trigger).toBeFocused();
  const tools = editor.getByRole('button', { name: 'Select watcher tools', exact: true });
  await tools.click();
  const search = page.getByRole('textbox', { name: 'Search watcher tools', exact: true });
  await expect(search).toBeFocused();
  await search.fill('file');
  await search.press('ArrowDown');
  const operationTrigger = page.getByRole('menuitem', {
    name: 'file_actions operations',
    exact: true,
  });
  await expect(operationTrigger).toBeFocused();
  await operationTrigger.press('ArrowRight');
  const operations = page.getByRole('menu', { name: 'file_actions operations', exact: true });
  await expect(operations).toBeVisible();
  await expect(operations).toBeInViewport();
  const menuBounds = await operations.boundingBox();
  expect(menuBounds!.x).toBeGreaterThanOrEqual(0);
  expect(menuBounds!.x + menuBounds!.width).toBeLessThanOrEqual(page.viewportSize()!.width);
  await expect(
    operations.getByRole('menuitemcheckbox', { name: 'read', exact: true }),
  ).toBeChecked();
  await expect(
    operations.getByRole('menuitemcheckbox', { name: 'write', exact: true }),
  ).not.toBeChecked();
  await page.screenshot({ path: testInfo.outputPath('watcher-tool-menu.png') });
  await operations.getByRole('menuitemcheckbox', { name: 'read', exact: true }).focus();
  await page.keyboard.press('ArrowLeft');
  await expect(operations).toBeHidden();
  await expect(operationTrigger).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(tools).toBeFocused();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('keep the enabled switch at its confirmed value after a failed save', async ({ page }) => {
  await mockHarness(page);
  const fixture = await mockWatchers(page);
  await connectAndCreate(page);
  await openWatchers(page);
  await page.route('**/api/instances/*/plugins/watcher/settings', (route) =>
    route.request().method() === 'PATCH'
      ? route.fulfill({ status: 503, json: { error: 'Settings unavailable' } })
      : route.fallback(),
  );
  const enabled = page.getByRole('switch', { name: 'Watchers enabled', exact: true });
  await expect(enabled).not.toBeChecked();
  await enabled.click();
  await expect(page.getByRole('alert').filter({ hasText: 'Settings unavailable' })).toBeVisible();
  await expect(enabled).not.toBeChecked();
  await expect(enabled).toBeEnabled();
  expect(fixture.updates).toHaveLength(0);
});
