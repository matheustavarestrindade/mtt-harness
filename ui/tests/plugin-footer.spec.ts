import { test, expect, type Locator } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { mockWatchers } from './watcher-fixture';
import { connectAndCreate, openWorkbenchAction } from './helpers';

async function footerMeasurements(button: Locator, name: string) {
  return button.evaluate((element, name) => {
    const style = getComputedStyle(element);
    function wordBounds(parent: Node, word: string) {
      const walker = document.createTreeWalker(parent, NodeFilter.SHOW_TEXT);
      let node: Node | null;
      while ((node = walker.nextNode())) {
        const start = node.textContent?.indexOf(word) ?? -1;
        if (start < 0) continue;
        const range = document.createRange();
        range.setStart(node, start);
        range.setEnd(node, start + word.length);
        return range.getBoundingClientRect();
      }
      throw new Error(`Missing label word: ${word}`);
    }
    const normalLabel = document.createElement('span');
    normalLabel.textContent = `Refresh ${name}`;
    Object.assign(normalLabel.style, {
      position: 'fixed',
      visibility: 'hidden',
      whiteSpace: 'pre',
      font: style.font,
      letterSpacing: style.letterSpacing,
    });
    document.body.append(normalLabel);
    const expectedGap =
      wordBounds(normalLabel, name).left - wordBounds(normalLabel, 'Refresh').right;
    normalLabel.remove();
    const refresh = wordBounds(element, 'Refresh');
    const actualGap = wordBounds(element, name).left - refresh.right;
    const footer = element.parentElement!;
    const panel = element.closest('section')!;
    return {
      expectedGap,
      actualGap,
      font: style.font,
      fontSize: style.fontSize,
      paddingLeft: style.paddingLeft,
      paddingRight: style.paddingRight,
      height: element.getBoundingClientRect().height,
      iconGap: refresh.left - element.querySelector('svg')!.getBoundingClientRect().right,
      footerInset: footer.getBoundingClientRect().left - panel.getBoundingClientRect().left,
      footerPadding: getComputedStyle(footer).paddingTop,
    };
  }, name);
}

test('preserve ordinary label spacing in the Sidekick reference', async ({ page }) => {
  await mockHarness(page);
  await mockWatchers(page);
  await connectAndCreate(page);
  await openWorkbenchAction(page, 'Settings');
  await page
    .getByRole('navigation', { name: 'Settings sections' })
    .getByRole('tab', { name: 'Sidekick', exact: true })
    .click();
  const button = page.getByRole('button', { name: 'Refresh Sidekick', exact: true });
  await expect(button).toBeVisible();
  const measurement = await footerMeasurements(button, 'Sidekick');
  expect(
    Math.abs(measurement.actualGap - measurement.expectedGap),
    JSON.stringify(measurement),
  ).toBeLessThanOrEqual(0.25);
  expect(measurement.fontSize).toBe('14px');
  expect(measurement.paddingLeft).toBe('10px');
  expect(measurement.paddingRight).toBe('10px');
  expect(measurement.height).toBe(44);
  expect(measurement.footerPadding).toBe('12px');
});

test('match Instructions and Watchers to the original Sidekick footer', async ({
  page,
}, testInfo) => {
  await mockHarness(page);
  await mockWatchers(page);
  if (page.viewportSize()!.width < 1024) await page.setViewportSize({ width: 320, height: 900 });
  await connectAndCreate(page);
  await openWorkbenchAction(page, 'Settings');
  const navigation = page.getByRole('navigation', { name: 'Settings sections' });
  await navigation.getByRole('tab', { name: 'Sidekick', exact: true }).click();
  const sidekick = page.getByRole('button', { name: 'Refresh Sidekick', exact: true });
  await expect(sidekick).toBeVisible();
  const baseline = await footerMeasurements(sidekick, 'Sidekick');
  expect(
    await navigation
      .getByRole('tab', { name: 'Sidekick', exact: true })
      .evaluate((element) => getComputedStyle(element).fontSize),
  ).toBe('12px');
  await sidekick.scrollIntoViewIfNeeded();
  await sidekick.locator('..').screenshot({ path: testInfo.outputPath('sidekick-reference.png') });
  for (const name of ['Instructions', 'Watchers']) {
    await navigation.getByRole('tab', { name, exact: true }).click();
    const button = page.getByRole('button', { name: `Refresh ${name}`, exact: true });
    await expect(button).toBeVisible();
    await button.scrollIntoViewIfNeeded();
    const measured = await footerMeasurements(button, name);
    for (const key of ['font', 'paddingLeft', 'paddingRight', 'height', 'footerPadding'] as const)
      expect(measured[key], `${name}: ${key}`).toBe(baseline[key]);
    expect(Math.abs(measured.iconGap - baseline.iconGap)).toBeLessThanOrEqual(0.5);
    expect(Math.abs(measured.footerInset - baseline.footerInset)).toBeLessThanOrEqual(0.5);
    expect(Math.abs(measured.actualGap - measured.expectedGap)).toBeLessThanOrEqual(0.25);
    await button
      .locator('..')
      .screenshot({ path: testInfo.outputPath(`${name.toLowerCase()}-footer.png`) });
  }
  await expect(
    page.getByRole('button', { name: 'Refresh instruction usage', exact: true }),
  ).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
