import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { chooseSelectOption, connectAndCreate, openWorkbenchAction } from './helpers';

test('configure workspace repetition with themed selectors and keep the draft', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  await page.getByLabel('Message', { exact: true }).fill('Keep this draft');
  await openWorkbenchAction(page, 'Workspace settings');
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('tab', { name: 'Instructions', exact: true }).click();
  const panel = dialog.getByRole('region', { name: 'Spaced repetition', exact: true });
  await expect(panel.getByRole('status').filter({ hasText: /^Off$/ })).toBeVisible();
  const recovery = panel.getByRole('region', { name: 'Recovery activity', exact: true });
  await expect(recovery).toContainText('No recovery activity yet.');
  await expect(recovery.locator('svg')).toHaveCount(0);
  await panel.getByRole('switch', { name: 'Instruction reminders enabled' }).click();
  await expect(panel.getByRole('status').filter({ hasText: /^On$/ })).toBeVisible();
  expect(fixture.repetitionUpdates[0].patch).toEqual({ enabled: true });
  await panel.getByRole('button', { name: 'Configure reminders' }).click();
  await chooseSelectOption(panel.getByLabel('Growth interval', { exact: true }), 'tokens');
  await panel.getByLabel('Interval tokens', { exact: true }).fill('4096');
  await panel.getByLabel('Reminder pattern', { exact: true }).fill('low, medium');
  await chooseSelectOption(
    panel.getByLabel('Recovery worker model', { exact: true }),
    fixture.model.ID,
  );
  await panel.getByRole('button', { name: 'Save reminder settings' }).click();
  await expect(panel.getByText('low → medium → repeat')).toBeVisible();
  expect(fixture.repetitionUpdates.at(-1)?.patch).toMatchObject({
    interval: { mode: 'tokens', tokens: 4096 },
    pattern: ['low', 'medium'],
    worker_model: fixture.model.ID,
    worker_effort: '',
  });
  await expect(panel.locator('select:visible')).toHaveCount(0);
  await page.keyboard.press('Escape');
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue('Keep this draft');
});

test('show recorded repetition metrics and recovery costs at narrow widths', async ({
  page,
}, testInfo) => {
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 740 });
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const workspace = fixture.instances[0].ID;
  fixture.repetitionMetrics.set(workspace, {
    Name: 'spaced_repetition',
    WorkspaceID: workspace,
    Counters: {
      'reminders/low': 3,
      'reminders/medium': 1,
      'reminders/high': 2,
      'reminders/tokens': 1800,
      'reminders/estimated_tokens': 1800,
      'recovery/completed': 2,
      'recovery/running': 1,
      'recovery/failed': 3,
      'recovery/cancelled': 4,
      'recovery/interrupted': 5,
      'recovery/queries': 4,
      'recovery/duration_ms': 1234,
    },
    Agents: [
      {
        Agent: 'spaced_repetition.instruction_recovery',
        ModelID: fixture.model.ID,
        FailedCalls: 0,
        DurationMilliseconds: 1234,
        Statistics: {
          ...fixture.sessionUsage,
          Calls: 4,
          Input: 400,
          CacheRead: 200,
          CacheWrite: 50,
          Output: 100,
          Reasoning: 50,
          Cost: { Currency: 'USD', Value: 0.015, Estimated: true },
          Costs: [{ Currency: 'USD', Value: 0.015, Estimated: true }],
        },
      },
    ],
  });
  await openWorkbenchAction(page, 'Workspace settings');
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('tab', { name: 'Instructions', exact: true }).click();
  const panel = dialog.getByRole('region', { name: 'Spaced repetition', exact: true });
  await expect(
    panel.getByText('Recovery worker cost', { exact: true }).locator('..'),
  ).toContainText('≈ $0.015');
  await expect(panel.getByText('500', { exact: true })).toBeVisible();
  const recovery = panel.getByRole('region', { name: 'Recovery activity', exact: true });
  await expect(recovery.locator('[data-slot="chart"]')).toBeVisible();
  const categories = recovery.getByRole('list', { name: 'Recovery activity categories' });
  for (const label of ['Running', 'Completed', 'Failed', 'Cancelled', 'Interrupted'])
    await expect(categories).toContainText(label);
  await expect(
    recovery
      .locator('.lc-axis-tick-label')
      .filter({ hasText: /Running|Completed|Failed|Cancelled|Interrupted/ }),
  ).toHaveCount(0);
  await recovery.getByRole('button', { name: 'View data', exact: true }).click();
  for (const [label, value] of [
    ['Running', '1'],
    ['Completed', '2'],
    ['Failed', '3'],
    ['Cancelled', '4'],
    ['Interrupted', '5'],
  ])
    await expect(recovery.getByRole('row').filter({ hasText: label }).getByRole('cell')).toHaveText(
      value,
    );
  await expect(panel.getByLabel('Recovery context', { exact: true })).toHaveText(
    '4 memory searches · 1.2 s recovery time',
  );
  await expect(panel.getByText('Running recoveries', { exact: true })).toHaveCount(0);
  await expect(panel.getByText('Completed recoveries', { exact: true })).toHaveCount(0);
  await panel.getByRole('button', { name: 'About instruction recovery usage' }).click();
  await expect(page.getByRole('tooltip')).toContainText(
    'Automatic reminders make no worker-model calls',
  );
  await page.keyboard.press('Escape');
  await expect(dialog).toBeVisible();
  await panel.getByRole('button', { name: 'Worker details', exact: true }).click();
  await panel.getByText('Instruction recovery', { exact: true }).click();
  await expect(panel.getByText('Input tokens', { exact: true }).locator('..')).toContainText('400');
  await expect(panel.getByText('Cache read', { exact: true }).locator('..')).toContainText('200');
  await expect(panel.getByText('Cache write', { exact: true }).locator('..')).toContainText('50');
  await panel.getByRole('button', { name: 'Configure reminders' }).click();
  await panel.getByLabel('Reminder pattern', { exact: true }).fill('high');
  await panel.getByRole('button', { name: 'Save reminder settings' }).click();
  await expect(panel.getByRole('alert')).toContainText('low or medium');
  expect(fixture.repetitionUpdates).toHaveLength(0);
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  ).toBeTruthy();
  await page.screenshot({ path: testInfo.outputPath('repetition-settings.png') });
});

test('open instruction controls directly from the memory panel', async ({ page }) => {
  await mockHarness(page);
  await connectAndCreate(page);
  if (!(await page.getByRole('region', { name: 'Workspace memory', exact: true }).isVisible()))
    await openWorkbenchAction(page, 'Memory');
  await page.getByRole('button', { name: 'Instruction reminders', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByRole('heading', { name: 'Instructions', exact: true })).toBeVisible();
  await expect(page.getByRole('dialog')).toHaveCount(1);
});
