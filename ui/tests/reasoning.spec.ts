import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';
import type { Message } from '../src/lib/atoms/types';

test('stream thinking collapsed and retain one expandable section after persistence', async ({
  page,
}, testInfo) => {
  const errors: string[] = [];
  page.on('pageerror', (failure) => errors.push(failure.message));
  const fixture = await mockHarness(page);
  fixture.model.Reasoning = true;
  fixture.model.ReasoningEfforts = ['none', 'low', 'high'];
  await connectAndCreate(page);
  await page.getByLabel('Message', { exact: true }).fill('wait');
  await page.getByRole('button', { name: 'Send message', exact: true }).click();
  const session = fixture.sessions[0];
  fixture.event(session, 'model.call', { model: fixture.model.ID, message_id: 'thinking-message' });
  fixture.event(session, 'model.chunk', {
    message_id: 'thinking-message',
    reasoning: '**Plan**\n\nCompare ',
  });
  fixture.event(session, 'model.chunk', {
    message_id: 'thinking-message',
    reasoning: 'the inputs.',
  });
  const thinking = page.getByRole('region', { name: 'Thinking', exact: true });
  await expect(thinking.getByRole('button')).toHaveAttribute('aria-expanded', 'false');
  await expect(thinking.getByText('Compare the inputs.', { exact: true })).toHaveCount(0);
  await thinking.getByRole('button').click();
  await expect(thinking.getByText('Compare the inputs.', { exact: true })).toBeVisible();
  fixture.event(session, 'model.chunk', {
    message_id: 'thinking-message',
    text: 'The answer is ready.',
  });
  await expect(page.getByRole('article', { name: 'assistant message' })).toContainText(
    'The answer is ready.',
  );
  const completed: Message = {
    ID: 'thinking-message',
    SessionID: session.ID,
    Seq: 2,
    Role: 'assistant',
    Content: [
      {
        Type: 'text',
        Text: 'The answer is ready.',
        Data: null,
        MIME: '',
        URL: '',
        Filename: '',
        AudioID: '',
      },
    ],
    Reasoning: '**Plan**\n\nCompare the inputs.',
    ToolCalls: null,
    ToolCallID: '',
    Usage: null,
    CreatedAt: new Date().toISOString(),
  };
  fixture.histories.get(session.ID)!.push(completed);
  fixture.running.delete(session.ID);
  fixture.event(session, 'turn.end', { status: 'completed' });
  await expect(thinking).toHaveCount(1);
  await expect(thinking.getByRole('button')).toHaveAttribute('aria-expanded', 'true');
  await expect(thinking.getByRole('button')).toHaveText('Thinking');
  await expect(page.getByText('The answer is ready.', { exact: true })).toHaveCount(1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('thinking-expanded.png'), fullPage: true });
  await page.reload();
  await expect(
    page.getByRole('region', { name: 'Thinking', exact: true }).getByRole('button'),
  ).toHaveAttribute('aria-expanded', 'false');
  expect(errors).toEqual([]);
});

test('use model-specific effort choices and show model prices before creating a session', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  fixture.model.Reasoning = true;
  fixture.model.ReasoningEfforts = ['none', 'low', 'high'];
  fixture.model.DefaultReasoningEffort = 'low';
  fixture.model.Prices = {
    Currency: 'USD',
    Input: 2,
    Output: 6,
    CacheRead: 0.2,
    CacheWrite: 0,
    CacheWriteUnknown: true,
    Source: 'https://models.dev/api.json',
    Tiers: [{ AboveInputTokens: 10000, Input: 4, Output: 9, CacheRead: 0.4, CacheWrite: 0 }],
  };
  await connectAndCreate(page);
  const effort = page.getByLabel('Thinking effort', { exact: true });
  await page.getByRole('button', { name: 'Session settings', exact: true }).click();
  await expect(effort.locator('option')).toHaveText(['Default (Low)', 'Off', 'Low', 'High']);
  await effort.selectOption('high');
  await expect.poll(() => fixture.sessions[0].ReasoningEffort).toBe('high');
  await expect(effort).toHaveValue('high');
  await page.reload();
  await page.getByRole('button', { name: 'Session settings', exact: true }).click();
  await expect(page.getByLabel('Thinking effort', { exact: true })).toHaveValue('high');
  await page.keyboard.press('Escape');
  await page.locator('header').getByRole('button', { name: 'New session', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByRole('region', { name: 'Model pricing' })).toContainText(
    'Rates per 1 million tokens',
  );
  await expect(dialog.getByRole('region', { name: 'Model pricing' })).toContainText('$2');
  await expect(dialog.getByRole('region', { name: 'Model pricing' })).toContainText('Unavailable');
  await expect(dialog.getByRole('region', { name: 'Model pricing' })).toContainText(
    'Above 10,000 input tokens',
  );
  await dialog.getByLabel('Thinking effort', { exact: true }).selectOption('none');
  await dialog.getByRole('button', { name: 'Create session', exact: true }).click();
  await expect.poll(() => fixture.sessions[1]?.ReasoningEffort).toBe('none');
  await expect(dialog).toHaveCount(0);
  await page.getByRole('button', { name: 'Session settings', exact: true }).click();
  await expect(page.getByLabel('Thinking effort', { exact: true })).toHaveValue('none');
  await page.getByLabel('Thinking effort', { exact: true }).selectOption('');
  await expect.poll(() => fixture.sessions[1]?.ReasoningEffort).toBe('');
});

test('keep an effort selection after an older poll returns and recover from a save error', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  fixture.model.Reasoning = true;
  fixture.model.ReasoningEfforts = ['low', 'high'];
  await connectAndCreate(page);
  let releasePoll: (() => void) | undefined;
  let captured = false;
  await page.route(/\/api\/sessions\/[^/]+$/, async (route) => {
    if (captured)
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify(fixture.sessions[0]),
      });
    const snapshot = JSON.stringify(fixture.sessions[0]);
    captured = true;
    await new Promise<void>((resolve) => {
      releasePoll = resolve;
    });
    await route.fulfill({ contentType: 'application/json', body: snapshot });
  });
  await expect.poll(() => captured).toBe(true);
  await page.getByRole('button', { name: 'Session settings', exact: true }).click();
  await page.getByLabel('Thinking effort', { exact: true }).selectOption('high');
  await expect.poll(() => fixture.sessions[0].ReasoningEffort).toBe('high');
  releasePoll!();
  await expect(page.getByLabel('Thinking effort', { exact: true })).toHaveValue('high');
  await page.route('**/api/sessions/*/reasoning', (route) =>
    route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'Cannot save thinking effort' }),
    }),
  );
  await page.getByLabel('Thinking effort', { exact: true }).selectOption('low');
  await expect(page.getByText('Cannot save thinking effort', { exact: true })).toBeVisible();
  await expect(page.getByLabel('Thinking effort', { exact: true })).toHaveValue('high');
});

test('show subscription access without API rates and label estimated usage', async ({ page }) => {
  const fixture = await mockHarness(page);
  fixture.model.Billing = 'subscription';
  fixture.model.Prices = { Currency: 'USD', Input: 90, Output: 100, CacheRead: 10, CacheWrite: 10 };
  fixture.sessionUsage.Cost = { Currency: 'USD', Value: 0.001, Estimated: true };
  fixture.sessionUsage.Costs = [fixture.sessionUsage.Cost];
  await connectAndCreate(page);
  await page.locator('header').getByRole('button', { name: 'New session', exact: true }).click();
  await expect(page.getByRole('dialog')).toContainText('Subscription access');
  await expect(page.getByRole('dialog')).not.toContainText('$90');
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'Open settings', exact: true }).click();
  await page
    .getByRole('navigation', { name: 'Settings sections' })
    .getByRole('button', { name: 'Usage', exact: true })
    .click();
  await expect(page.getByRole('dialog')).toContainText('Estimated cost');
});
