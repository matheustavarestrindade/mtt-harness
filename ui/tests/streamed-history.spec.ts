import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';
import type { Message, Session } from '../src/lib/atoms/types';
import type { ProcessRecord } from '../src/lib/atoms/processes';

function message(session: Session, index: number): Message {
  return {
    ID: `history-${index}`,
    SessionID: session.ID,
    Seq: index,
    Role: index % 2 ? 'user' : 'assistant',
    Content: [
      {
        Type: 'text',
        Text: `History entry ${index}: ${'Bounded conversation content. '.repeat(12)}`,
        Data: null,
        MIME: '',
        URL: '',
        Filename: '',
        AudioID: '',
      },
    ],
    ToolCalls: [],
    ToolCallID: '',
    Usage:
      index % 2
        ? null
        : { Input: 100, CacheRead: 40, CacheWrite: 0, Output: 10, Reasoning: 4, Cost: null },
    CreatedAt: new Date(1700000000000 + index * 1000).toISOString(),
  };
}

test('load bounded history pages, preserve scroll position and evict older DOM at the bottom', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  const history = Array.from({ length: 600 }, (_, index) => message(session, index + 1));
  fixture.histories.set(session.ID, history);
  fixture.state(session);
  const requests: string[] = [];
  page.on('request', (request) => {
    const url = new URL(request.url());
    if (url.pathname.startsWith(`/api/sessions/${session.ID}/`))
      requests.push(url.pathname + url.search);
  });
  const conversation = page.getByRole('region', { name: 'Conversation', exact: true });
  await expect(conversation.locator('[data-history-id]')).toHaveCount(50);
  await expect(conversation.getByText(/History entry 600:/)).toBeVisible();
  await expect(conversation.getByText(/History entry 1:/)).toHaveCount(0);
  for (let step = 0; step < 5; step++) {
    const request = page.waitForResponse(
      (response) => response.url().includes('/history?') && response.url().includes('before='),
    );
    await conversation.evaluate((element) => {
      element.dispatchEvent(new WheelEvent('wheel', { deltaY: -100, bubbles: true }));
      element.scrollTop = 0;
    });
    await request;
    await expect
      .poll(() => conversation.locator('[data-history-id]').count())
      .toBeLessThanOrEqual(151);
    await page.waitForTimeout(80);
  }
  await expect(conversation.getByText(/History entry 600:/)).toHaveCount(0);
  await page.getByRole('button', { name: 'Scroll to latest message', exact: true }).click();
  await expect(conversation.getByText(/History entry 600:/)).toBeVisible();
  await expect(conversation.locator('[data-history-id]')).toHaveCount(50);
  expect(requests.some((url) => url.includes('/history?') && url.includes('before='))).toBe(true);
  expect(requests.some((url) => url.endsWith('/messages'))).toBe(false);
  expect(
    requests.filter((url) => url.includes('/history?')).every((url) => url.includes('limit=50')),
  ).toBe(true);
});

test('use socket state while healthy and restore idle/queue/model state without status polling', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  await expect.poll(() => fixture.openSockets.size).toBeGreaterThan(0);
  const requests: string[] = [];
  page.on('request', (request) => {
    const url = new URL(request.url());
    if (url.pathname.startsWith(`/api/sessions/${session.ID}`)) requests.push(url.pathname);
  });
  fixture.running.add(session.ID);
  fixture.state(session);
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeVisible();
  await page.waitForTimeout(5500);
  expect(
    requests.filter(
      (url) =>
        url.endsWith('/status') || url.endsWith('/state') || url === `/api/sessions/${session.ID}`,
    ),
  ).toEqual([]);
  const pending = message(session, 1);
  fixture.queued.set(session.ID, [pending]);
  fixture.state(session);
  await expect(page.locator('[data-queued-message]')).toContainText('Queued');
  fixture.queued.set(session.ID, []);
  fixture.running.delete(session.ID);
  fixture.state(session);
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toHaveCount(0);
  await expect(page.locator('[data-queued-message]')).toHaveCount(0);
});

test('discard a late older page after the history revision changes', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  fixture.histories.set(
    session.ID,
    Array.from({ length: 180 }, (_, index) => message(session, index + 1)),
  );
  fixture.state(session);
  const conversation = page.getByRole('region', { name: 'Conversation', exact: true });
  await expect(conversation.getByText(/History entry 180:/)).toBeVisible();
  let release!: () => void, entered!: () => void;
  const waiting = new Promise<void>((resolve) => (release = resolve)),
    started = new Promise<void>((resolve) => (entered = resolve));
  await page.route(`**/sessions/${session.ID}/history?*`, async (route) => {
    const url = new URL(route.request().url());
    if (!url.searchParams.has('before')) return route.fallback();
    const stale = fixture.historyPage(session, url.searchParams);
    entered();
    await waiting;
    await route.fulfill({ json: stale });
  });
  await conversation.evaluate((element) => {
    element.dispatchEvent(new WheelEvent('wheel', { deltaY: -100, bubbles: true }));
    element.scrollTop = 0;
  });
  await started;
  session.HistoryRevision = 1;
  fixture.histories.set(session.ID, [message(session, 1), message(session, 2)]);
  fixture.state(session);
  release();
  await expect(conversation.getByText(/History entry 2:/)).toBeVisible();
  await expect(conversation.getByText(/History entry 180:/)).toHaveCount(0);
  await expect(conversation.locator('[data-history-id]')).toHaveCount(2);
});

test('evict offscreen terminal and media resources when returning to the latest window', async ({
  page,
}) => {
  await page.addInitScript(() => {
    const active = new Set<string>();
    (window as unknown as { activeMedia: Set<string> }).activeMedia = active;
    const create = URL.createObjectURL.bind(URL),
      revoke = URL.revokeObjectURL.bind(URL);
    URL.createObjectURL = (blob) => {
      const value = create(blob);
      active.add(value);
      return value;
    };
    URL.revokeObjectURL = (value) => {
      active.delete(value);
      revoke(value);
    };
  });
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  const history = Array.from({ length: 160 }, (_, index) => message(session, index + 1));
  history[119].Content = [
    {
      Type: 'image',
      Text: '',
      MIME: 'image/png',
      Data: 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jQioAAAAASUVORK5CYII=',
      URL: '',
      Filename: 'small.png',
      AudioID: '',
    },
  ];
  history[129] = {
    ...history[129],
    Role: 'assistant',
    Content: [],
    ToolCalls: [
      {
        ID: 'evicted-terminal',
        Name: 'bash',
        Input: { command: 'running terminal', wait: false, terminal: true },
      },
    ],
  };
  history[130] = {
    ...history[130],
    Role: 'tool',
    ToolCallID: 'evicted-terminal',
    Content: [
      {
        Type: 'text',
        Text: 'process evicted-process is started',
        Data: null,
        MIME: '',
        URL: '',
        Filename: '',
        AudioID: '',
      },
    ],
  };
  fixture.histories.set(session.ID, history);
  const record: ProcessRecord = {
    ID: 'evicted-process',
    SessionID: session.ID,
    InstanceID: session.InstanceID,
    PID: 123,
    Status: 'running',
    Exit: null,
    StartedAt: history[129].CreatedAt,
    EndedAt: '',
    Spec: {
      Command: 'sh',
      Args: [],
      Cwd: '/workspace',
      Timeout: 0,
      Terminal: true,
      Notify: { Mode: 'none', Interval: 0 },
      ToolMessageID: history[129].ID,
      ToolCallID: 'evicted-terminal',
    },
  };
  fixture.processes.set(record.ID, record);
  fixture.processChunk(record.ID, 'stdout', '\x1b[32mkept while open\x1b[0m\r\n');
  fixture.state(session);
  await expect(page.getByRole('region', { name: 'Conversation', exact: true })).toHaveAttribute(
    'data-history-count',
    '50',
  );
  await page
    .getByRole('region', { name: 'Conversation', exact: true })
    .evaluate((element) =>
      element.dispatchEvent(new WheelEvent('wheel', { deltaY: -24, bubbles: true })),
    );
  await page.getByLabel('small.png', { exact: true }).scrollIntoViewIfNeeded();
  await expect
    .poll(() =>
      page.evaluate(() => (window as unknown as { activeMedia: Set<string> }).activeMedia.size),
    )
    .toBeGreaterThan(0);
  const call = page.locator('[data-call-id="evicted-terminal"]');
  await call.locator('.activity-toggle').click();
  await expect.poll(() => fixture.processSockets.size).toBe(1);
  await expect
    .poll(() =>
      page.evaluate(() => (window as unknown as { activeMedia: Set<string> }).activeMedia.size),
    )
    .toBeGreaterThan(0);
  history.push(...Array.from({ length: 100 }, (_, index) => message(session, index + 161)));
  fixture.state(session);
  const latest = page.getByRole('button', { name: 'Scroll to latest message', exact: true });
  await expect(latest).toBeVisible();
  await latest.click();
  await expect(page.locator('[data-history-id]')).toHaveCount(50);
  await expect(call).toHaveCount(0);
  await expect.poll(() => fixture.processSockets.size).toBe(0);
  await expect
    .poll(() =>
      page.evaluate(() => (window as unknown as { activeMedia: Set<string> }).activeMedia.size),
    )
    .toBe(0);
  expect(record.Status).toBe('running');
});
