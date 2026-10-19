import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';
import type { Message, Session } from '../src/lib/atoms/types';

function history(session: Session): Message[] {
  return Array.from({ length: 60 }, (_, index) => ({
    ID: `scroll-${index}`,
    SessionID: session.ID,
    Role: 'assistant',
    Seq: index + 1,
    Content: [
      {
        Type: 'text',
        Text: `Entry ${index}. ${'Visible conversation content. '.repeat(12)}`,
        Data: null,
        MIME: '',
        URL: '',
        Filename: '',
        AudioID: '',
      },
    ],
    ToolCalls: [],
    ToolCallID: '',
    Usage: null,
    CreatedAt: new Date(1700000000000 + index * 1000).toISOString(),
  }));
}

for (const input of ['wheel', 'keyboard'] as const) {
  test(`a small upward ${input} scroll stays paused during streamed and snapshot updates`, async ({
    page,
  }) => {
    const fixture = await mockHarness(page);
    await connectAndCreate(page);
    const session = fixture.sessions[0];
    fixture.histories.set(session.ID, history(session));
    fixture.state(session);
    const conversation = page.getByRole('region', { name: 'Conversation', exact: true });
    await expect(conversation.locator('[data-history-id]')).toHaveCount(50);
    fixture.event(session, 'model.call', { message_id: 'live-scroll', model: session.Model });
    fixture.event(session, 'model.chunk', { message_id: 'live-scroll', text: 'Working now. ' });
    const distance = () =>
      conversation.evaluate(
        (element) => element.scrollHeight - element.scrollTop - element.clientHeight,
      );
    await expect.poll(distance).toBeLessThan(3);
    if (input === 'wheel') {
      await conversation.hover();
      await page.mouse.wheel(0, -24);
    } else {
      await conversation.focus();
      await page.keyboard.press('ArrowUp');
    }
    await expect.poll(distance).toBeGreaterThan(5);
    const top = await conversation.evaluate((element) => element.scrollTop);
    for (let index = 0; index < 6; index++)
      fixture.event(session, 'model.chunk', {
        message_id: 'live-scroll',
        text: `Part ${index}. ${'More output. '.repeat(25)}`,
      });
    fixture.state(session);
    await expect(conversation.locator('[data-history-id="live-scroll"]')).toContainText('Part 5.');
    await expect
      .poll(() => conversation.evaluate((element) => element.scrollTop))
      .toBeLessThanOrEqual(top + 3);
    await expect(
      page.getByRole('button', { name: 'Scroll to latest message', exact: true }),
    ).toBeVisible();
    await page.getByRole('button', { name: 'Scroll to latest message', exact: true }).click();
    fixture.event(session, 'model.chunk', {
      message_id: 'live-scroll',
      text: 'Back at the end. '.repeat(30),
    });
    await expect.poll(distance).toBeLessThan(3);
  });
}

test('show queued status and cancellation once on each message, then consume the batch', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  fixture.running.add(session.ID);
  fixture.state(session);
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeVisible();
  for (const text of ['I just canceled', 'ok?', 'last note']) {
    await page.getByLabel('Message', { exact: true }).fill(text);
    await page.getByRole('button', { name: 'Queue message', exact: true }).click();
    await expect(page.getByLabel('Message', { exact: true })).toHaveValue('');
  }
  const cards = page.locator('[data-queued-message]');
  await expect(cards).toHaveCount(3);
  await expect(page.getByRole('region', { name: 'Queued messages' })).toHaveCount(0);
  await expect(page.getByText('Message accepted', { exact: true })).toHaveCount(0);
  for (const text of ['I just canceled', 'ok?', 'last note']) {
    const card = cards.filter({ hasText: text });
    await expect(card.getByRole('status')).toHaveText('Queued');
    await expect(card.getByRole('button', { name: /Cancel queued message/ })).toBeVisible();
    await expect(page.getByText(text, { exact: true })).toHaveCount(1);
  }
  const oldPending = [...fixture.queued.get(session.ID)!];
  await cards
    .filter({ hasText: 'ok?' })
    .getByRole('button', { name: /Cancel queued message/ })
    .click();
  await expect(cards).toHaveCount(2);
  fixture.state(session, {
    Status: {
      running: true,
      queued: 3,
      messages: oldPending.map((message) => message.ID),
      error: '',
    },
  });
  await expect(cards).toHaveCount(2);
  await expect(page.getByText('ok?', { exact: true })).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath('queued-messages.png'), fullPage: true });
  const admitted = [...fixture.queued.get(session.ID)!];
  fixture.histories.set(session.ID, admitted);
  fixture.queued.set(session.ID, []);
  fixture.state(session);
  await expect(cards).toHaveCount(0);
  for (const text of ['I just canceled', 'last note'])
    await expect(page.getByText(text, { exact: true })).toHaveCount(1);
});

test('keep the message queued if cancellation fails', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  fixture.running.add(session.ID);
  fixture.state(session);
  await page.getByLabel('Message', { exact: true }).fill('Keep this queued');
  await page.getByRole('button', { name: 'Queue message', exact: true }).click();
  await page.route('**/queue/*', (route) =>
    route.fulfill({ status: 503, json: { error: 'Queue storage unavailable' } }),
  );
  const card = page.locator('[data-queued-message]').filter({ hasText: 'Keep this queued' });
  await card.getByRole('button', { name: /Cancel queued message/ }).click();
  await expect(page.getByText('Queue storage unavailable', { exact: true })).toBeVisible();
  await expect(card.getByRole('status')).toHaveText('Queued');
  await expect(card.getByRole('button', { name: /Cancel queued message/ })).toBeEnabled();
});
