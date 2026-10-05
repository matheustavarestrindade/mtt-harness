import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';

test('Enter submits while Shift and Command Enter insert text at the caret', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const input = page.getByLabel('Message', { exact: true });
  await input.fill('first');
  await input.press('Shift+Enter');
  await input.pressSequentially('second');
  await expect(input).toHaveValue('first\nsecond');
  await input.evaluate((element: HTMLTextAreaElement) => element.setSelectionRange(5, 6));
  await input.press('Meta+Enter');
  await expect(input).toHaveValue('first\nsecond');
  expect(await input.evaluate((element: HTMLTextAreaElement) => element.selectionStart)).toBe(6);
  await input.press('Meta+Enter');
  await expect(input).toHaveValue('first\n\nsecond');
  expect(fixture.histories.get(fixture.sessions[0].ID)).toHaveLength(0);
  await input.press('Enter');
  await expect(input).toHaveValue('');
  await expect(page.getByRole('article', { name: 'assistant message' })).toContainText('first');
  const sent = fixture.histories
    .get(fixture.sessions[0].ID)!
    .filter((message) => message.Role === 'user');
  expect(sent).toHaveLength(1);
  expect(sent[0].Content?.[0].Text).toBe('first\n\nsecond');
});

test('IME confirmation, blank drafts and held Enter do not submit', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const input = page.getByLabel('Message', { exact: true });
  await input.press('Enter');
  await input.fill('日本語');
  await input.dispatchEvent('keydown', { key: 'Enter', code: 'Enter', isComposing: true });
  await input.dispatchEvent('keydown', { key: 'Enter', code: 'Enter', keyCode: 229 });
  await input.dispatchEvent('keydown', { key: 'Enter', code: 'Enter', repeat: true });
  await expect(input).toHaveValue('日本語');
  expect(fixture.histories.get(fixture.sessions[0].ID)).toHaveLength(0);
  await input.press('Enter');
  await expect(input).toHaveValue('');
  expect(
    fixture.histories.get(fixture.sessions[0].ID)!.filter((message) => message.Role === 'user'),
  ).toHaveLength(1);
});

test('Enter queues a new message while the current turn is running', async ({ page }) => {
  await mockHarness(page);
  await connectAndCreate(page);
  const input = page.getByLabel('Message', { exact: true });
  await input.fill('wait');
  await input.press('Enter');
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeVisible();
  await input.fill('Next request');
  await input.press('Enter');
  await expect(page.getByRole('region', { name: 'Queued messages' })).toContainText('1 queued');
  await expect(input).toHaveValue('');
});
