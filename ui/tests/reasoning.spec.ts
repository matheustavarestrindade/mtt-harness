import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import {
  connectAndCreate,
  chooseSelectOption,
  expectSelectOptions,
  openWorkbenchAction,
} from './helpers';
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
  await expect(thinking.getByRole('button')).toContainText('Plan');
  await expect(thinking.locator('[data-slot="activity-indicator"]')).toBeVisible();
  await expect(thinking.getByText('Compare the inputs.', { exact: true })).toHaveCount(0);
  await thinking.getByRole('button').click();
  await expect(thinking.getByText('Compare the inputs.', { exact: true })).toBeVisible();
  fixture.event(session, 'model.chunk', {
    message_id: 'thinking-message',
    reasoning: '\n\n**Checking sources**\n\nRead the project files.',
  });
  await expect(thinking.getByRole('button')).toContainText('Checking sources');
  await expect(thinking.getByRole('button')).toHaveAttribute('aria-expanded', 'true');
  fixture.event(session, 'model.chunk', {
    message_id: 'thinking-message',
    text: 'The answer is ready.',
  });
  await expect(page.getByRole('article', { name: 'assistant message' })).toContainText(
    'The answer is ready.',
  );
  await expect(thinking.locator('[data-slot="activity-indicator"]')).toHaveCount(0);
  await expect(thinking.locator('[data-slot="thinking-status"]')).toHaveCount(0);
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
    Reasoning: '**Plan**\n\nCompare the inputs.\n\n**Checking sources**\n\nRead the project files.',
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
  await expectSelectOptions(effort, ['Default (Low)', 'Off', 'Low', 'High']);
  await chooseSelectOption(effort, 'high');
  await expect.poll(() => fixture.sessions[0].ReasoningEffort).toBe('high');
  await expect(effort).toHaveAttribute('data-value', 'high');
  await page.reload();
  await page.getByRole('button', { name: 'Session settings', exact: true }).click();
  await expect(page.getByLabel('Thinking effort', { exact: true })).toHaveAttribute(
    'data-value',
    'high',
  );
  await page.keyboard.press('Escape');
  await openWorkbenchAction(page, 'New session');
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByRole('region', { name: 'Model pricing' })).toContainText(
    'Rates per 1 million tokens',
  );
  await expect(dialog.getByRole('region', { name: 'Model pricing' })).toContainText('$2');
  await expect(dialog.getByRole('region', { name: 'Model pricing' })).toContainText('Unavailable');
  await expect(dialog.getByRole('region', { name: 'Model pricing' })).toContainText(
    'Above 10,000 input tokens',
  );
  await chooseSelectOption(dialog.getByLabel('Thinking effort', { exact: true }), 'none');
  await dialog.getByRole('button', { name: 'Create session', exact: true }).click();
  await expect.poll(() => fixture.sessions[1]?.ReasoningEffort).toBe('none');
  await expect(dialog).toHaveCount(0);
  await page.getByRole('button', { name: 'Session settings', exact: true }).click();
  await expect(page.getByLabel('Thinking effort', { exact: true })).toHaveAttribute(
    'data-value',
    'none',
  );
  await chooseSelectOption(page.getByLabel('Thinking effort', { exact: true }), '');
  await expect.poll(() => fixture.sessions[1]?.ReasoningEffort).toBe('');
});

test('keep an effort selection after an older snapshot arrives and recover from a save error', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  fixture.model.Reasoning = true;
  fixture.model.ReasoningEfforts = ['low', 'high'];
  await connectAndCreate(page);
  const previous = { ...fixture.sessions[0] };
  await page.getByRole('button', { name: 'Session settings', exact: true }).click();
  await chooseSelectOption(page.getByLabel('Thinking effort', { exact: true }), 'high');
  await expect.poll(() => fixture.sessions[0].ReasoningEffort).toBe('high');
  fixture.state(fixture.sessions[0], { Session: previous });
  await expect(page.getByLabel('Thinking effort', { exact: true })).toHaveAttribute(
    'data-value',
    'high',
  );
  await page.route('**/api/sessions/*/reasoning', (route) =>
    route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'Cannot save thinking effort' }),
    }),
  );
  await chooseSelectOption(page.getByLabel('Thinking effort', { exact: true }), 'low');
  await expect(page.getByText('Cannot save thinking effort', { exact: true })).toBeVisible();
  await expect(page.getByLabel('Thinking effort', { exact: true })).toHaveAttribute(
    'data-value',
    'high',
  );
});

test('show subscription access without API rates and label estimated usage', async ({ page }) => {
  const fixture = await mockHarness(page);
  fixture.model.Billing = 'subscription';
  fixture.model.Prices = { Currency: 'USD', Input: 90, Output: 100, CacheRead: 10, CacheWrite: 10 };
  fixture.sessionUsage.Cost = { Currency: 'USD', Value: 0.001, Estimated: true };
  fixture.sessionUsage.Costs = [fixture.sessionUsage.Cost];
  await connectAndCreate(page);
  await openWorkbenchAction(page, 'New session');
  await expect(page.getByRole('dialog')).toContainText('Subscription access');
  await expect(page.getByRole('dialog')).not.toContainText('$90');
  await page.keyboard.press('Escape');
  await openWorkbenchAction(page, 'Settings');
  await page
    .getByRole('navigation', { name: 'Settings sections' })
    .getByRole('tab', { name: 'Usage', exact: true })
    .click();
  const usage = page.getByRole('region', { name: 'Usage overview' });
  await chooseSelectOption(usage.getByLabel('Scope', { exact: true }), 'session');
  await expect(usage.getByLabel('Usage totals')).toContainText('≈ $0.001');
  await usage.getByRole('tab', { name: 'Cost', exact: true }).click();
  await expect(usage.getByRole('region', { name: 'Daily cost', exact: true })).toContainText(
    'includes estimates',
  );
});
