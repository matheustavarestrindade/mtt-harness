import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { connectAndCreate, chooseSelectOption } from './helpers';

test('choose thinking effort by keyboard, preserve the default value, and close only the menu with Escape', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  fixture.model.Reasoning = true;
  fixture.model.ReasoningEfforts = ['none', 'low', 'high'];
  fixture.model.DefaultReasoningEffort = 'low';
  await connectAndCreate(page);
  await page.getByRole('button', { name: 'Session settings', exact: true }).click();
  const thinking = page.getByRole('button', { name: 'Thinking effort', exact: true });
  await expect(thinking).toHaveAttribute('data-slot', 'select-trigger');
  await expect(thinking).toHaveAttribute('aria-haspopup', 'listbox');
  await expect(thinking).toContainText('Default (Low)');
  await thinking.focus();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('listbox')).toBeVisible();
  await page.keyboard.press('End');
  await page.keyboard.press('Enter');
  await expect.poll(() => fixture.sessions[0].ReasoningEffort).toBe('high');
  await expect(thinking).toContainText('High');
  await expect(thinking).toBeFocused();
  await thinking.press('ArrowDown');
  await page.keyboard.press('Home');
  await page.keyboard.press('Enter');
  await expect.poll(() => fixture.sessions[0].ReasoningEffort).toBe('');
  await expect(thinking).toContainText('Default (Low)');
  await thinking.press('Space');
  await expect(page.getByRole('listbox')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('listbox')).toHaveCount(0);
  await expect(page.getByRole('dialog', { name: 'Session settings', exact: true })).toBeVisible();
  await expect(thinking).toBeFocused();
  expect(fixture.sessions).toHaveLength(1);
  await expect(page.locator('select:visible')).toHaveCount(0);
});

test('scroll a large model catalog without horizontal overflow and retain the display label', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  for (let position = 0; position < 60; position++) {
    fixture.models.push({
      ...fixture.model,
      ID: `synthetic/catalog-model-${position}`,
      Name: `Catalog model ${position} with a descriptive label and a long identifier for layout checks`,
    });
  }
  await connectAndCreate(page);
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 640 });
  await page.getByRole('button', { name: 'Session settings', exact: true }).click();
  const model = page.getByRole('button', { name: 'Session model', exact: true });
  await model.click();
  const list = page.getByRole('listbox');
  await expect(page.getByRole('option')).toHaveCount(61);
  expect((await list.boundingBox())!.height).toBeLessThanOrEqual(321);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.keyboard.press('End');
  const last = page.getByRole('option').last();
  await expect(last).toBeInViewport();
  await expect(last).toHaveAttribute('data-highlighted');
  await page.screenshot({ path: testInfo.outputPath('shadcn-model-select.png'), fullPage: true });
  await page.keyboard.press('Enter');
  await expect.poll(() => fixture.sessions[0].Model).toBe('synthetic/catalog-model-59');
  await expect(model).toContainText('Catalog model 59');
  await expect(model).toContainText('synthetic/catalog-model-59');
  await expect(model).toHaveAttribute('data-value', 'synthetic/catalog-model-59');
  await expect(page.locator('select:visible')).toHaveCount(0);
});

test('display an unavailable saved effort as disabled and allow an explicit default choice', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  fixture.model.Reasoning = true;
  fixture.model.ReasoningEfforts = ['low', 'high'];
  await connectAndCreate(page);
  fixture.sessions[0].ReasoningEffort = 'retired-effort';
  await page.reload();
  await page.getByRole('button', { name: 'Session settings', exact: true }).click();
  const thinking = page.getByRole('button', { name: 'Thinking effort', exact: true });
  await expect(thinking).toContainText('Unavailable: retired-effort');
  await thinking.click();
  await expect(
    page.getByRole('option', { name: 'Unavailable: retired-effort', exact: true }),
  ).toBeDisabled();
  await page.keyboard.press('Escape');
  await chooseSelectOption(thinking, '');
  await expect.poll(() => fixture.sessions[0].ReasoningEffort).toBe('');
  await expect(thinking).toContainText('Default');
});
