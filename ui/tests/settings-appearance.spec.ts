import { expect, test, type Locator } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { mockWatchers } from './watcher-fixture';
import { connectAndCreate, openWorkbenchAction } from './helpers';

const pages = [
  'General',
  'Instructions',
  'Sidekick',
  'Watchers',
  'Usage',
  'Providers',
  'Connection',
];

async function titleGeometry(title: Locator) {
  return title.evaluate((element) => {
    const heading = element.querySelector('h2')!;
    const description = element.querySelector('p')!;
    const icon = element.querySelector('svg')!;
    const viewport = element.closest('[data-settings-content]')!;
    const headerBounds = element.getBoundingClientRect();
    const headingBounds = heading.getBoundingClientRect();
    const descriptionBounds = description.getBoundingClientRect();
    const iconBounds = icon.getBoundingClientRect();
    const viewportBounds = viewport.getBoundingClientRect();
    const headingStyle = getComputedStyle(heading);
    const descriptionStyle = getComputedStyle(description);
    return {
      inset: headerBounds.left - viewportBounds.left,
      top: headerBounds.top - viewportBounds.top,
      titleLeft: headingBounds.left - headerBounds.left,
      titleTop: headingBounds.top - headerBounds.top,
      fontSize: headingStyle.fontSize,
      fontWeight: headingStyle.fontWeight,
      lineHeight: headingStyle.lineHeight,
      iconLeft: iconBounds.left - headerBounds.left,
      iconTop: iconBounds.top - headerBounds.top,
      iconWidth: iconBounds.width,
      iconHeight: iconBounds.height,
      descriptionLeft: descriptionBounds.left - headerBounds.left,
      descriptionGap: descriptionBounds.top - headingBounds.bottom,
      descriptionFont: descriptionStyle.fontSize,
      descriptionLine: descriptionStyle.lineHeight,
    };
  });
}

test('all seven Settings pages use one aligned title including Connection', async ({
  page,
}, testInfo) => {
  await mockHarness(page);
  await mockWatchers(page);
  await connectAndCreate(page);
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 900 });
  await openWorkbenchAction(page, 'Settings');
  const settingsTabs = page.getByRole('tablist', { name: 'Settings pages', exact: true });
  await expect(settingsTabs.getByRole('tab')).toHaveCount(7);
  let baseline: Awaited<ReturnType<typeof titleGeometry>> | undefined;
  for (const name of ['Sidekick', ...pages.filter((name) => name !== 'Sidekick')]) {
    const tab = settingsTabs.getByRole('tab', { name, exact: true });
    await tab.click();
    await expect(tab).toHaveAttribute('aria-selected', 'true');
    const title = page.locator('[data-settings-page-title]');
    await expect(title).toHaveCount(1);
    await expect(title.getByRole('heading', { level: 2 })).toHaveText(name);
    await expect
      .poll(() => page.locator('[data-settings-content]').evaluate((element) => element.scrollTop))
      .toBe(0);
    const geometry = await titleGeometry(title);
    expect(geometry.fontSize).toBe('18px');
    expect(geometry.fontWeight).toBe('500');
    expect(geometry.lineHeight).toBe('28px');
    expect(geometry.iconWidth).toBe(20);
    expect(geometry.iconHeight).toBe(20);
    expect(geometry.titleLeft).toBe(32);
    expect(geometry.descriptionLeft).toBe(32);
    expect(geometry.descriptionGap).toBe(4);
    expect(geometry.descriptionFont).toBe('14px');
    expect(geometry.descriptionLine).toBe('20px');
    if (baseline)
      for (const key of Object.keys(baseline) as (keyof typeof geometry)[])
        expect(geometry[key], `${name}: ${key}`).toBe(baseline[key]);
    else baseline = geometry;
    await page.screenshot({
      path: testInfo.outputPath(`settings-${name.toLowerCase()}-title.png`),
    });
    await page
      .locator('[data-settings-content]')
      .evaluate((element) => (element.scrollTop = element.scrollHeight));
  }
  const overflow = await page.evaluate(() =>
    Array.from(document.querySelectorAll<HTMLElement>('body *'))
      .filter((element) => {
        const bounds = element.getBoundingClientRect();
        return bounds.width > 0 && bounds.right > innerWidth + 1;
      })
      .slice(0, 10)
      .map((element) => ({
        tag: element.tagName,
        classes: element.className,
        width: element.getBoundingClientRect().width,
        left: element.getBoundingClientRect().left,
      })),
  );
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    JSON.stringify(overflow),
  ).toBe(true);
});

test('Watcher highlights fit their track and nested tab keyboard navigation is isolated', async ({
  page,
}, testInfo) => {
  await mockHarness(page);
  await mockWatchers(page);
  await connectAndCreate(page);
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 900 });
  await openWorkbenchAction(page, 'Settings');
  const outer = page.getByRole('tablist', { name: 'Settings pages', exact: true });
  const parent = outer.getByRole('tab', { name: 'Watchers', exact: true });
  await parent.click();
  const inner = page.getByRole('tablist', { name: 'Watcher sections', exact: true });
  await expect(inner).toHaveAttribute('data-orientation', 'horizontal');
  await expect(outer).toHaveAttribute(
    'data-orientation',
    testInfo.project.name === 'mobile' ? 'horizontal' : 'vertical',
  );
  for (const name of ['Watchers', 'Usage', 'Activity']) {
    const tab = inner.getByRole('tab', { name, exact: true });
    await tab.click();
    await expect(tab).toHaveAttribute('aria-selected', 'true');
    const bounds = await inner.boundingBox(),
      selected = await tab.boundingBox();
    expect(bounds!.height).toBe(50);
    expect(selected!.height).toBe(44);
    expect(selected!.y - bounds!.y).toBeCloseTo(3, 1);
    expect(bounds!.y + bounds!.height - selected!.y - selected!.height).toBeCloseTo(3, 1);
    expect(selected!.x).toBeGreaterThanOrEqual(bounds!.x + 2.5);
    expect(selected!.x + selected!.width).toBeLessThanOrEqual(bounds!.x + bounds!.width - 2.5);
    await expect(parent).toHaveAttribute('aria-selected', 'true');
    await inner.screenshot({
      path: testInfo.outputPath(`watcher-${name.toLowerCase()}-highlight.png`),
    });
  }
  await inner.getByRole('tab', { name: 'Watchers', exact: true }).click();
  await page.keyboard.press('ArrowRight');
  await expect(inner.getByRole('tab', { name: 'Usage', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  await expect(parent).toHaveAttribute('aria-selected', 'true');
  await parent.focus();
  await page.keyboard.press(testInfo.project.name === 'mobile' ? 'ArrowRight' : 'ArrowDown');
  await expect(outer.getByRole('tab', { name: 'Usage', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  await expect(inner).toHaveCount(0);
});

test('disconnected pages use the shared title and the connection tab stays accessible', async ({
  page,
}) => {
  await mockHarness(page);
  await page.goto('/');
  await page.getByRole('button', { name: 'Connect harness', exact: true }).click();
  const tabs = page.getByRole('tablist', { name: 'Settings pages', exact: true });
  await expect(tabs.getByRole('tab', { name: 'Connection', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  await expect(page.locator('[data-settings-page-title] h2')).toHaveText('Connection');
  await tabs.getByRole('tab', { name: 'General', exact: true }).click();
  await expect(page.locator('[data-settings-page-title] h2')).toHaveText('Connect your harness');
  await page.getByRole('button', { name: 'Open connection settings', exact: true }).click();
  await expect(page.getByLabel('API token', { exact: true })).toBeVisible();
  expect((await titleGeometry(page.locator('[data-settings-page-title]'))).titleLeft).toBe(32);
});
