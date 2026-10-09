import { expect, test, type Page } from '@playwright/test';
import { readFile } from 'node:fs/promises';
import { mockHarness } from './api-fixture';
import { connectAndCreate, chooseSelectOption } from './helpers';

const png = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jC1kAAAAASUVORK5CYII=',
  'base64',
);
async function setup(page: Page, input: string[] = ['text'], protocol = 'chat_completions') {
  const fixture = await mockHarness(page);
  fixture.model.Input = input;
  await page.route('**/api/providers', (route) =>
    route.fulfill({ json: [{ Name: 'test', Protocol: protocol, Authentication: 'none' }] }),
  );
  await connectAndCreate(page);
  return fixture;
}

test('send a UTF-8 text attachment with a text-only model', async ({ page }) => {
  const fixture = await setup(page);
  const input = page.getByLabel('Attach files', { exact: true });
  await expect(input).toHaveAttribute('accept', /\.txt/);
  await expect(input).not.toHaveAttribute('accept', /image\/|video\/|audio\//);
  const text = 'const message = "Olá 世界";\n';
  await input.setInputFiles({
    name: 'example.ts',
    mimeType: 'text/plain',
    buffer: Buffer.from(text),
  });
  await expect(page.getByRole('button', { name: 'Send message', exact: true })).toBeEnabled();
  await page.getByLabel('Message', { exact: true }).press('Enter');
  await expect(page.getByRole('list', { name: 'Attachments', exact: true })).toHaveCount(0);
  await expect
    .poll(
      () =>
        fixture.histories.get(fixture.sessions[0]!.ID)?.filter((message) => message.Role === 'user')
          .length,
    )
    .toBe(1);
  const attachment = fixture.histories.get(fixture.sessions[0]!.ID)![0]!.Content![0]!;
  expect(attachment.Type).toBe('text');
  expect(attachment.Filename).toBe('example.ts');
  expect(attachment.Text).toContain(text);
  expect(attachment.Data).toBeNull();
  await page.reload();
  await expect(page.getByText('example.ts', { exact: true }).first()).toBeVisible();
});

test('send and render real image, video, audio and document bytes', async ({ page }, testInfo) => {
  const fixture = await setup(page, ['text', 'image', 'audio', 'video', 'file']);
  const input = page.getByLabel('Attach files', { exact: true });
  await expect(input).toHaveAttribute('accept', /audio\/mpeg/);
  const video = await readFile('tests/fixtures/attachment.mp4');
  const pdf = Buffer.from('%PDF-1.4\nattachment fixture\n%%EOF');
  const wav = Buffer.alloc(46);
  wav.write('RIFF');
  wav.writeUInt32LE(38, 4);
  wav.write('WAVEfmt ', 8);
  wav.writeUInt32LE(16, 16);
  wav.writeUInt16LE(1, 20);
  wav.writeUInt16LE(1, 22);
  wav.writeUInt32LE(8000, 24);
  wav.writeUInt32LE(16000, 28);
  wav.writeUInt16LE(2, 32);
  wav.writeUInt16LE(16, 34);
  wav.write('data', 36);
  wav.writeUInt32LE(2, 40);
  await input.setInputFiles([
    { name: 'pixel.png', mimeType: 'image/png', buffer: png },
    { name: 'clip.mp4', mimeType: 'video/mp4', buffer: video },
    { name: 'audio.wav', mimeType: 'audio/wav', buffer: wav },
    { name: 'guide.pdf', mimeType: 'application/pdf', buffer: pdf },
  ]);
  await page.getByLabel('Message', { exact: true }).fill('Describe the attachments');
  await expect(page.getByRole('button', { name: 'Send message', exact: true })).toBeEnabled();
  await page.getByRole('button', { name: 'Send message', exact: true }).click();
  await expect
    .poll(
      () =>
        fixture.histories.get(fixture.sessions[0]!.ID)?.filter((message) => message.Role === 'user')
          .length,
    )
    .toBe(1);
  const parts = fixture.histories.get(fixture.sessions[0]!.ID)![0]!.Content!;
  expect(parts.map((part) => part.Type)).toEqual(['text', 'image', 'video', 'audio', 'file']);
  expect(parts.slice(1).map((part) => part.Data)).toEqual(
    [png, video, wav, pdf].map((data) => data.toString('base64')),
  );
  await expect(page.getByRole('img', { name: 'pixel.png', exact: true })).toBeVisible();
  await expect(page.locator('video[aria-label="clip.mp4"]')).toBeVisible();
  await expect
    .poll(() => page.locator('video').evaluate((element: HTMLVideoElement) => element.readyState))
    .toBeGreaterThan(0);
  await expect(page.locator('audio[aria-label="audio.wav"]')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Download', exact: true })).toHaveCount(4);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({ path: testInfo.outputPath('attachments.png'), fullPage: true });
  await page.reload();
  await expect(page.locator('video[aria-label="clip.mp4"]')).toBeVisible();
});

test('reject unsupported, oversized and binary text files without losing the draft', async ({
  page,
}) => {
  await setup(page);
  const message = page.getByLabel('Message', { exact: true });
  await message.fill('Keep this draft');
  const input = page.getByLabel('Attach files', { exact: true });
  await input.setInputFiles({ name: 'blocked.png', mimeType: 'image/png', buffer: png });
  await expect(page.getByRole('alert')).toContainText('does not accept image');
  await expect(page.getByRole('list', { name: 'Attachments', exact: true })).toHaveCount(0);
  await input.setInputFiles({
    name: 'large.txt',
    mimeType: 'text/plain',
    buffer: Buffer.alloc(512 * 1024 + 1, 65),
  });
  await expect(page.getByRole('alert')).toContainText('512 KB');
  await input.setInputFiles({
    name: 'binary.txt',
    mimeType: 'text/plain',
    buffer: Buffer.from([0, 255, 0]),
  });
  await expect(page.getByRole('list', { name: 'Attachments', exact: true })).toContainText(
    /not valid|binary|encoded/i,
  );
  await expect(page.getByRole('button', { name: 'Send message', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: 'Remove binary.txt', exact: true }).click();
  await expect(message).toHaveValue('Keep this draft');
  await expect(page.getByRole('button', { name: 'Send message', exact: true })).toBeEnabled();
});

test('retain attachments on rejection and clear them only after acceptance', async ({ page }) => {
  const fixture = await setup(page, ['text', 'image']);
  let attempts = 0;
  await page.route('**/api/sessions/*/messages', async (route) => {
    if (route.request().method() !== 'POST') return route.fallback();
    attempts++;
    if (attempts === 1)
      return route.fulfill({ status: 500, json: { error: 'Upload rejected by fixture' } });
    return route.fallback();
  });
  await page
    .getByLabel('Attach files', { exact: true })
    .setInputFiles({ name: 'kept.png', mimeType: 'image/png', buffer: png });
  await page.getByLabel('Message', { exact: true }).fill('Keep on error');
  const send = page.getByRole('button', { name: 'Send message', exact: true });
  await expect(send).toBeEnabled();
  await send.click();
  await expect(page.getByText('Upload rejected by fixture', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Remove kept.png', exact: true })).toBeVisible();
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue('Keep on error');
  await send.click();
  await expect(page.getByRole('list', { name: 'Attachments', exact: true })).toHaveCount(0);
  expect(attempts).toBe(2);
  expect(
    fixture.histories.get(fixture.sessions[0]!.ID)?.filter((message) => message.Role === 'user'),
  ).toHaveLength(1);
});

test('revalidate drafts after model changes and reject Responses audio', async ({ page }) => {
  const fixture = await mockHarness(page);
  fixture.model.Input = ['text', 'image', 'audio'];
  fixture.models.push({ ...fixture.model, ID: 'test/text-only', Input: ['text'] });
  await page.route('**/api/providers', (route) =>
    route.fulfill({ json: [{ Name: 'test', Protocol: 'responses' }] }),
  );
  await connectAndCreate(page);
  const input = page.getByLabel('Attach files', { exact: true });
  await expect(input).not.toHaveAttribute('accept', /audio\//);
  await input.setInputFiles({ name: 'image.png', mimeType: 'image/png', buffer: png });
  await expect(page.getByRole('button', { name: 'Send message', exact: true })).toBeEnabled();
  await page.getByRole('button', { name: 'Session settings', exact: true }).click();
  await chooseSelectOption(
    page.getByRole('button', { name: 'Session model', exact: true }),
    'test/text-only',
  );
  await page.getByRole('dialog').getByRole('button', { name: 'Close', exact: true }).click();
  await expect(page.getByRole('list', { name: 'Attachments', exact: true })).toContainText(
    'does not accept image',
  );
  await expect(page.getByRole('button', { name: 'Send message', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: 'Remove image.png', exact: true }).click();
});

test('retain queued attachments and preserve a new draft during submission', async ({ page }) => {
  const fixture = await setup(page, ['text', 'image']);
  const sessionID = fixture.sessions[0]!.ID;
  await page.getByLabel('Message', { exact: true }).fill('wait');
  await page.getByRole('button', { name: 'Send message', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeVisible();
  await page
    .getByLabel('Attach files', { exact: true })
    .setInputFiles({ name: 'queued.png', mimeType: 'image/png', buffer: png });
  await expect(page.getByRole('button', { name: 'Queue message', exact: true })).toBeEnabled();
  await page.getByRole('button', { name: 'Queue message', exact: true }).click();
  await expect(page.getByRole('list', { name: 'Attachments', exact: true })).toHaveCount(0);
  expect(fixture.running.has(sessionID)).toBe(true);
  await page.getByLabel('Message', { exact: true }).fill('Next draft');
  await expect(
    page
      .getByRole('region', { name: 'Conversation', exact: true })
      .getByRole('img', { name: 'queued.png', exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue('Next draft');
});
