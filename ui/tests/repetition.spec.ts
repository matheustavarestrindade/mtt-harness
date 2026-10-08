import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { chooseSelectOption, connectAndCreate } from './helpers';

test('configure workspace repetition with themed selectors and keep the draft', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  await page.getByLabel('Message', { exact: true }).fill('Keep this draft');
  await page.getByRole('button', { name: 'Open settings', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('button', { name: 'Instructions', exact: true }).click();
  const panel = dialog.getByRole('region', { name: 'Spaced repetition', exact: true });
  await expect(panel.getByRole('status')).toHaveText('Off');
  await panel.getByRole('button', { name: 'Enable instruction reminders' }).click();
  await expect(panel.getByRole('status')).toHaveText('On');
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
      'recovery/queries': 4,
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
          Output: 100,
          Reasoning: 50,
          Cost: { Currency: 'USD', Value: 0.015, Estimated: true },
          Costs: [{ Currency: 'USD', Value: 0.015, Estimated: true }],
        },
      },
    ],
  });
  await page.getByRole('button', { name: 'Open settings', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('button', { name: 'Instructions', exact: true }).click();
  const panel = dialog.getByRole('region', { name: 'Spaced repetition', exact: true });
  await expect(
    panel.getByText('Recovery worker cost', { exact: true }).locator('..'),
  ).toContainText('≈ $0.015');
  await expect(panel.getByText('500', { exact: true })).toBeVisible();
  await panel.getByText('Instruction recovery', { exact: true }).click();
  await expect(panel.getByText('400', { exact: true })).toBeVisible();
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
  if (await page.getByRole('button', { name: 'Open memory panel', exact: true }).isVisible())
    await page.getByRole('button', { name: 'Open memory panel', exact: true }).click();
  await page.getByRole('button', { name: 'Instruction reminders', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(
    dialog.getByRole('heading', { name: 'Spaced repetition', exact: true }),
  ).toBeVisible();
  await expect(page.getByRole('dialog')).toHaveCount(1);
});
