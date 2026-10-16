import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';
import type { Message } from '../src/lib/atoms/types';
import type { ProcessRecord } from '../src/lib/atoms/processes';

test('render raw terminal colors, send input and resize, and close only the socket on collapse', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  const plan: Message = {
    ID: 'terminal-plan',
    SessionID: session.ID,
    Seq: 1,
    Role: 'assistant',
    Content: [],
    ToolCalls: [
      {
        ID: 'terminal-call',
        Name: 'bash',
        Input: { command: 'interactive test', wait: false, terminal: true },
      },
    ],
    ToolCallID: '',
    Usage: null,
    CreatedAt: new Date().toISOString(),
  };
  fixture.histories.set(session.ID, [
    plan,
    {
      ...plan,
      ID: 'terminal-result',
      Seq: 2,
      Role: 'tool',
      ToolCalls: [],
      ToolCallID: 'terminal-call',
      Content: [
        {
          Type: 'text',
          Text: 'process interactive-process is started',
          Data: null,
          MIME: '',
          URL: '',
          Filename: '',
          AudioID: '',
        },
      ],
    },
  ]);
  const record: ProcessRecord = {
    ID: 'interactive-process',
    SessionID: session.ID,
    InstanceID: session.InstanceID,
    PID: 123,
    Status: 'running',
    Exit: null,
    StartedAt: plan.CreatedAt,
    EndedAt: '',
    Spec: {
      Command: 'sh',
      Args: ['-c', 'interactive test'],
      Cwd: '/workspace',
      Notify: { Mode: 'exit', Interval: 0 },
      Timeout: 0,
      Terminal: true,
      ToolCallID: 'terminal-call',
      ToolMessageID: plan.ID,
    },
  };
  fixture.processes.set(record.ID, record);
  fixture.processChunk(record.ID, 'stdout', '\x1b[38;2;18;171;99mTTY READY\x1b[0m\r\n');
  fixture.event(session, 'process.start', record);
  fixture.state(session);
  const call = page.locator('[data-call-id="terminal-call"]');
  await call.locator('.activity-toggle').click();
  await expect(call.getByRole('button', { name: 'Send Ctrl-C' })).toBeVisible();
  await expect
    .poll(() =>
      call
        .locator('.xterm-screen')
        .evaluate((element) =>
          Array.from(element.querySelectorAll('span')).some(
            (span) =>
              span.textContent?.includes('TTY') &&
              getComputedStyle(span).color === 'rgb(18, 171, 99)',
          ),
        ),
    )
    .toBe(true);
  await call.locator('.terminal-host').click();
  await page.keyboard.type('hello');
  await page.keyboard.insertText('🌍');
  await expect
    .poll(() =>
      fixture.processControls
        .filter((entry) => entry.control.type === 'input')
        .map((entry) => Buffer.from(String(entry.control.data_base64), 'base64').toString())
        .join(''),
    )
    .toBe('hello🌍');
  await expect
    .poll(() =>
      fixture.processControls.some(
        (entry) => entry.control.type === 'resize' && Number(entry.control.columns) > 1,
      ),
    )
    .toBe(true);
  await call.locator('.activity-toggle').click();
  await expect.poll(() => fixture.processSockets.size).toBe(0);
  expect(record.Status).toBe('running');
  await call.locator('.activity-toggle').click();
  await expect(call.getByRole('button', { name: 'Send Ctrl-C' })).toBeVisible();
  await call.getByRole('button', { name: 'Send Ctrl-C' }).click();
  await expect.poll(() => record.Status).toBe('stopped');
  expect(fixture.processControls.some((entry) => entry.control.data_base64 === 'Aw==')).toBe(true);
  await expect.poll(() => fixture.processSockets.size).toBe(0);
  await expect(call.getByRole('button', { name: 'Send Ctrl-C' })).toHaveCount(0);
});
