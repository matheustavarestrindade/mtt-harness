import { test, expect } from '@playwright/test';
import type { Message } from '../src/lib/atoms/types';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';

test('preserve rejected argument text and show its non-execution after reload', async ({
  page,
}, testInfo) => {
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 800 });
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  const raw = '{"path":"例-<img src=x onerror=window.invalidInputExecuted=true>.txt",\n';
  const base: Message = {
    ID: 'rejected-input',
    Seq: 1,
    SessionID: session.ID,
    Role: 'assistant',
    Content: [],
    ToolCallID: '',
    Usage: null,
    CreatedAt: new Date().toISOString(),
    ToolCalls: [
      {
        ID: 'bad-input',
        Name: 'file_actions',
        Input: raw,
        InputError: 'invalid JSON arguments at byte 65: unexpected end of JSON input',
      },
    ],
  };
  fixture.histories.set(session.ID, [
    base,
    {
      ...base,
      ID: 'input-error-result',
      Role: 'tool',
      ToolCalls: null,
      ToolCallID: 'bad-input',
      Content: [
        {
          Type: 'text',
          Text: 'file_actions input was not executed: invalid JSON arguments',
          Data: null,
          URL: '',
          MIME: '',
          Filename: '',
          AudioID: '',
        },
      ],
    },
  ]);
  fixture.event(session, 'tool.end', {});
  for (const reload of [false, true]) {
    if (reload) await page.reload();
    const call = page.locator('[data-call-id="bad-input"]');
    await call.locator('.activity-toggle').click();
    await expect(call.getByRole('status')).toContainText('Input was not executed');
    await expect(call.locator('pre code').first()).toHaveText(raw);
    await expect(call.locator('img,script')).toHaveCount(0);
    expect(await page.evaluate(() => Reflect.get(window, 'invalidInputExecuted'))).toBeUndefined();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  }
});
