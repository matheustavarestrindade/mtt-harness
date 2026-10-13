import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';
import { thinkingStatus } from '../src/lib/atoms/thinking-status';

test('preview only exposed thinking text and prefer the latest complete heading', () => {
  expect(thinkingStatus('**Inspecting inputs**\n\nRead the file.\n\n**Checking the')).toBe(
    'Inspecting inputs',
  );
  expect(thinkingStatus('**Inspecting inputs**\n\nRead the file.\n\n**Checking the result**')).toBe(
    'Checking the result',
  );
  expect(thinkingStatus('## Compare `module` inputs\n\n```ts\n## not a heading\n```')).toBe(
    'Compare module inputs',
  );
  expect(thinkingStatus('Read the first source. Compare the second source.')).toBe(
    'Compare the second source.',
  );
  expect(thinkingStatus('[Checking inputs](https://example.invalid)')).toBe('Checking inputs');
  expect(thinkingStatus('')).toBe('');
  const preview = thinkingStatus(`Old source\n${'x'.repeat(20000)}\n\n**${'🧩'.repeat(200)}**`);
  expect(Array.from(preview)).toHaveLength(121);
  expect(preview.endsWith('…')).toBe(true);
});

test('animate the loader without work text and stop it on cancellation', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  await page.emulateMedia({ reducedMotion: 'no-preference' });
  await connectAndCreate(page);
  await page.getByLabel('Message', { exact: true }).fill('wait');
  await page.getByRole('button', { name: 'Send message', exact: true }).click();
  const activity = page.getByRole('status', { name: 'Processing', exact: true });
  const loader = activity.locator('[data-slot="activity-indicator"]');
  await expect(loader).toBeVisible();
  await expect(activity).toHaveText('');
  await expect(page.getByText('Harness is working', { exact: false })).toHaveCount(0);
  await expect(
    page
      .getByRole('region', { name: 'Conversation', exact: true })
      .getByRole('status', { name: 'Processing', exact: true }),
  ).toHaveCount(0);
  await expect(page.locator('[data-slot="conversation-activity"]')).toHaveCount(0);
  const navigation = page.getByRole('navigation', { name: 'Workspaces and sessions', exact: true });
  const header = page.locator('#conversation-header');
  if (testInfo.project.name === 'mobile') {
    await expect(header.getByRole('status', { name: 'Processing', exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
    await expect(
      navigation
        .locator('[aria-current="page"]')
        .getByRole('status', { name: 'Processing', exact: true }),
    ).toBeVisible();
    await expect(header.getByRole('status', { name: 'Processing', exact: true })).toHaveCount(0);
    await page.getByRole('button', { name: 'Close navigation', exact: true }).click();
  } else {
    await expect(
      navigation
        .locator('[aria-current="page"]')
        .getByRole('status', { name: 'Processing', exact: true }),
    ).toBeVisible();
    await expect(header.getByRole('status', { name: 'Processing', exact: true })).toHaveCount(0);
    await page.getByRole('button', { name: 'Collapse navigation', exact: true }).click();
    await expect(header.getByRole('status', { name: 'Processing', exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Expand navigation', exact: true }).click();
  }
  await expect(activity).toHaveCount(1);
  expect(
    await loader.evaluate(
      (element) =>
        element
          .getAnimations({ subtree: true })
          .filter((animation) => animation.playState === 'running').length,
    ),
  ).toBe(9);
  const firstBit = loader.locator('span').first();
  const firstTransform = await firstBit.evaluate((element) => getComputedStyle(element).transform);
  await expect
    .poll(() => firstBit.evaluate((element) => getComputedStyle(element).transform))
    .not.toBe(firstTransform);
  const session = fixture.sessions[0];
  fixture.event(session, 'model.call', { model: fixture.model.ID, message_id: 'live-thinking' });
  fixture.event(session, 'model.chunk', {
    message_id: 'live-thinking',
    reasoning:
      '**Inspecting the workspace files and tracing the relevant paths before making the requested changes**\n\nOnly the current project.',
  });
  const thinking = page.getByRole('region', { name: 'Thinking', exact: true });
  await expect(thinking.getByRole('button')).toContainText('Inspecting the workspace');
  await expect(thinking.getByRole('button')).toHaveAttribute('aria-expanded', 'false');
  await expect(thinking.locator('[data-slot="activity-indicator"]')).toBeVisible();
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 640 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await expect(page.getByText('Session ready', { exact: true })).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath('sidebar-processing.png'), fullPage: true });
  await page.getByRole('button', { name: 'Stop', exact: true }).click();
  await expect(activity).toHaveCount(0);
  await expect(page.locator('[data-slot="activity-indicator"]')).toHaveCount(0);
});

test('respect reduced motion and remove the loader after failure', async ({ page }) => {
  const fixture = await mockHarness(page);
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await connectAndCreate(page);
  await page.getByLabel('Message', { exact: true }).fill('wait');
  await page.getByRole('button', { name: 'Send message', exact: true }).click();
  const activity = page.getByRole('status', { name: 'Processing', exact: true });
  const loader = activity.locator('[data-slot="activity-indicator"]');
  await expect(loader).toBeVisible();
  expect(await loader.evaluate((element) => element.getAnimations({ subtree: true }).length)).toBe(
    0,
  );
  fixture.running.delete(fixture.sessions[0].ID);
  fixture.event(fixture.sessions[0], 'run.error', { error: 'Provider unavailable' });
  await expect(activity).toHaveCount(0);
});
