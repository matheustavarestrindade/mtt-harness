import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';
import type { Message } from '../src/lib/atoms/types';

test('identify background output as Runtime rather than a message from the user', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  const text =
    '[process] Runtime update from a background process, not a new user request.\nprocess ended\n<img src=x onerror="window.injected=true">';
  const update: Message = {
    ID: 'runtime-update',
    SessionID: session.ID,
    Seq: 1,
    Role: 'runtime',
    Content: [
      { Type: 'text', Text: text, Data: null, MIME: '', URL: '', Filename: '', AudioID: '' },
    ],
    ToolCalls: null,
    ToolCallID: '',
    Usage: null,
    CreatedAt: new Date().toISOString(),
  };
  fixture.histories.get(session.ID)!.push(update);
  fixture.event(session, 'turn.end', {});
  const message = page.getByRole('article', { name: 'runtime message', exact: true });
  await expect(message.getByText('Runtime', { exact: true })).toBeVisible();
  await expect(message.getByText('You', { exact: true })).toHaveCount(0);
  await expect(message.getByText('Assistant', { exact: true })).toHaveCount(0);
  await expect(message).toContainText(text);
  await expect(message.locator('img')).toHaveCount(0);
  expect(await page.evaluate(() => 'injected' in window)).toBe(false);
});
