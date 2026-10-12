import { expect, test } from '@playwright/test';
import { readFile } from 'node:fs/promises';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';

test('retain playing media through idle polling and authoritative recovery reads', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0]!;
  const video = await readFile('tests/fixtures/attachment.mp4');
  fixture.histories.set(session.ID, [
    {
      ID: 'saved-video',
      SessionID: session.ID,
      Seq: 1,
      Role: 'user',
      CreatedAt: new Date().toISOString(),
      Content: [
        {
          Type: 'video',
          Data: video.toString('base64'),
          MIME: 'video/mp4',
          Filename: 'clip.mp4',
          Text: '',
          URL: '',
          AudioID: '',
        },
      ],
      ToolCalls: null,
      ToolCallID: '',
      Usage: null,
    },
  ]);
  fixture.event(session, 'turn.end', {});
  const player = page.locator('video');
  await expect(player).toBeVisible();
  await expect
    .poll(() => player.evaluate((element: HTMLVideoElement) => element.readyState))
    .toBeGreaterThanOrEqual(2);
  await player.evaluate(async (element: HTMLVideoElement) => {
    element.loop = true;
    await element.play();
  });
  const originalSource = await player.getAttribute('src');
  let historyReads = 0;
  page.on('request', (request) => {
    if (new URL(request.url()).pathname.endsWith('/messages') && request.method() === 'GET')
      historyReads++;
  });
  await page.waitForTimeout(3300);
  expect(historyReads).toBe(0);
  await expect(player).toHaveAttribute('src', originalSource!);
  expect(await player.evaluate((element: HTMLVideoElement) => element.paused)).toBe(false);
  fixture.event(session, 'turn.end', {});
  await expect.poll(() => historyReads).toBeGreaterThan(0);
  await expect(player).toHaveAttribute('src', originalSource!);
  expect(await player.evaluate((element: HTMLVideoElement) => element.paused)).toBe(false);
  // A real resource change must still replace the preview and release the old URL.
  fixture.histories.get(session.ID)![0]!.Content![0]!.Data = Buffer.concat([
    video,
    Buffer.from([0]),
  ]).toString('base64');
  fixture.event(session, 'turn.end', {});
  await expect.poll(() => player.getAttribute('src')).not.toBe(originalSource);
});
