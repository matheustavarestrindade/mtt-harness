import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';
import type { Message, Session } from '../src/lib/atoms/types';

function pendingMessage(session: Session, id: string, text: string): Message {
  return {
    ID: id,
    SessionID: session.ID,
    Seq: 0,
    Role: 'user',
    Content: [
      { Type: 'text', Text: text, Data: null, MIME: '', URL: '', Filename: '', AudioID: '' },
    ],
    ToolCalls: [],
    ToolCallID: '',
    Usage: null,
    CreatedAt: new Date().toISOString(),
  };
}

test('restore queued text, blue user bars and cancel buttons after reload', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  fixture.running.add(session.ID);
  fixture.state(session);
  for (const text of ['Keep this first request', 'This one can be cancelled']) {
    await page.getByLabel('Message', { exact: true }).fill(text);
    await page.getByRole('button', { name: 'Queue message', exact: true }).click();
    await expect(page.getByLabel('Message', { exact: true })).toHaveValue('');
  }
  const reads: string[] = [];
  page.on('request', (request) => {
    if (request.method() === 'GET' && new URL(request.url()).pathname.includes('/queue/'))
      reads.push(request.url());
  });
  await page.reload();
  const cards = page.locator('[data-queued-message]');
  await expect(cards).toHaveCount(2);
  for (const text of ['Keep this first request', 'This one can be cancelled']) {
    const card = cards.filter({ hasText: text });
    await expect(card).toHaveClass(/user-message/);
    await expect(card.getByRole('status')).toHaveText('Queued');
    await expect(card.getByRole('button', { name: /Cancel queued message/ })).toBeVisible();
    const border = await card.evaluate((element) => ({
      width: getComputedStyle(element).borderLeftWidth,
      color: getComputedStyle(element).borderLeftColor,
    }));
    expect(parseFloat(border.width)).toBeGreaterThan(0);
    expect(border.color).not.toBe('rgba(0, 0, 0, 0)');
  }
  expect(reads).toEqual([]);
  expect(await page.evaluate(() => JSON.stringify(sessionStorage))).not.toContain(
    'Keep this first request',
  );
  await expect(page.getByRole('region', { name: 'Queued messages' })).toHaveCount(0);
  await cards
    .filter({ hasText: 'This one can be cancelled' })
    .getByRole('button', { name: /Cancel queued message/ })
    .click();
  await expect(cards).toHaveCount(1);
  await page.reload();
  await expect(cards).toHaveCount(1);
  await expect(cards).toContainText('Keep this first request');
  await page.screenshot({ path: testInfo.outputPath('restored-queue.png'), fullPage: true });
});

test('restore messages submitted by another client and keep runtime data out of user cards', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  fixture.queued.set(session.ID, [
    pendingMessage(session, 'remote-user', 'Request from another client'),
    { ...pendingMessage(session, 'runtime-notice', 'Background process output'), Role: 'runtime' },
  ]);
  fixture.running.add(session.ID);
  fixture.state(session);
  await page.reload();
  const cards = page.locator('[data-queued-message]');
  await expect(cards).toHaveCount(1);
  await expect(cards).toContainText('Request from another client');
  await expect(cards).toHaveClass(/user-message/);
  await expect(page.getByText('Background process output', { exact: true })).toHaveCount(0);
});

test('bound restored previews and load original text and media only on request', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  const text = pendingMessage(session, 'long-user', 'Unicode 🌍 '.repeat(2000) + 'END_OF_ORIGINAL');
  const image = pendingMessage(session, 'image-user', 'Queued picture');
  image.Content!.push({
    Type: 'image',
    Text: '',
    Data: 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j7d8AAAAASUVORK5CYII=',
    MIME: 'image/png',
    URL: '',
    Filename: 'queued-image.png',
    AudioID: '',
  });
  fixture.queued.set(session.ID, [text, image]);
  fixture.running.add(session.ID);
  fixture.state(session);
  await page.reload();
  const longCard = page.locator('[data-queued-message="long-user"]'),
    imageCard = page.locator('[data-queued-message="image-user"]');
  await expect(longCard).toContainText('Preview limited');
  expect((await longCard.textContent())!.length).toBeLessThan(4500);
  await expect(longCard).not.toContainText('END_OF_ORIGINAL');
  await expect(imageCard).toContainText('queued-image.png');
  await expect(imageCard.locator('img')).toHaveCount(0);
  await imageCard.getByRole('button', { name: 'Show full message', exact: true }).click();
  await expect(imageCard.locator('img')).toHaveCount(1);
  await longCard.getByRole('button', { name: 'Show full message', exact: true }).click();
  await expect(longCard).toContainText('END_OF_ORIGINAL');
  expect(fixture.queued.get(session.ID)?.map((message) => message.ID)).toEqual([
    'long-user',
    'image-user',
  ]);
  expect(fixture.histories.get(session.ID)).toEqual([]);
});

test('a late queued-body read cannot restore an admitted message', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  const message = pendingMessage(session, 'late-body', 'Long queued input. '.repeat(1000));
  fixture.queued.set(session.ID, [message]);
  fixture.running.add(session.ID);
  fixture.state(session);
  await page.reload();
  let release!: () => void, entered!: () => void;
  const waiting = new Promise<void>((resolve) => (release = resolve)),
    started = new Promise<void>((resolve) => (entered = resolve));
  await page.route(`**/sessions/${session.ID}/queue/${message.ID}`, async (route) => {
    if (route.request().method() !== 'GET') return route.fallback();
    entered();
    await waiting;
    await route.fulfill({ json: message });
  });
  const card = page.locator('[data-queued-message]');
  await card.getByRole('button', { name: 'Show full message', exact: true }).click();
  await started;
  fixture.queued.set(session.ID, []);
  fixture.histories.set(session.ID, [message]);
  fixture.state(session);
  release();
  await expect(card).toHaveCount(0);
  await expect(page.locator(`[data-history-id="${message.ID}"]`)).toHaveCount(1);
});
