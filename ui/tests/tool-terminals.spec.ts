import { test, expect } from '@playwright/test';
import type { Message, Session } from '../src/lib/atoms/types';
import type { ProcessRecord } from '../src/lib/atoms/processes';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';

function message(
  session: Session,
  identifier: string,
  text: string,
  overrides: Partial<Message> = {},
): Message {
  return {
    ID: identifier,
    SessionID: session.ID,
    Seq: 1,
    Role: 'assistant',
    Content: [
      { Type: 'text', Text: text, Data: null, MIME: '', URL: '', Filename: '', AudioID: '' },
    ],
    ToolCalls: null,
    ToolCallID: '',
    Usage: null,
    CreatedAt: new Date().toISOString(),
    ...overrides,
  };
}
function processRecord(
  session: Session,
  messageID = 'shell-plan',
  identifier = 'process-live',
): ProcessRecord {
  return {
    ID: identifier,
    InstanceID: session.InstanceID,
    SessionID: session.ID,
    Spec: {
      Command: 'sh',
      Args: ['-c', 'npm run test'],
      Cwd: '/project',
      Notify: { Mode: 'none', Interval: 0 },
      Timeout: 0,
      IdleTimeout: 10000000000,
      ToolMessageID: messageID,
      ToolCallID: 'shell-call',
    },
    PID: 100,
    Status: 'running',
    Exit: null,
    StartedAt: new Date().toISOString(),
    EndedAt: '',
  };
}

test('show arguments and structured results as readable fields and tables', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  fixture.histories.set(session.ID, [
    message(session, 'tool-plan', '', {
      ToolCalls: [
        {
          ID: 'data-call',
          Name: 'search_tool',
          Input: { query: 'Find project files', limit: 10, categories: ['files', 'project'] },
        },
        {
          ID: 'edit-call',
          Name: 'file_actions',
          Input: {
            paths: ['a.go', 'b.go'],
            actions: [
              { op: 'replace', old_text: 'OldName', new_text: 'DescriptiveName', mode: 'all' },
            ],
            return: { type: 'summary' },
          },
        },
      ],
    }),
    message(
      session,
      'data-result',
      JSON.stringify([
        { Name: 'file_actions', Description: '<img src=x onerror=window.injected=true>' },
      ]),
      { Role: 'tool', ToolCallID: 'data-call' },
    ),
  ]);
  fixture.event(session, 'tool.result', {});
  const data = page.locator('[data-call-id="data-call"]');
  await data.locator('.activity-toggle').click();
  await expect(data.getByText('Find project files', { exact: true })).toBeVisible();
  await expect(data.getByRole('table', { name: 'Tool result' })).toBeVisible();
  await expect(data.getByRole('cell', { name: 'file_actions', exact: true })).toBeVisible();
  await expect(data.locator('img, script')).toHaveCount(0);
  await expect(data.locator('pre')).toHaveCount(0);
  const edit = page.locator('[data-call-id="edit-call"]');
  await edit.locator('.activity-toggle').click();
  await expect(edit.getByRole('heading', { name: '1. Replace' })).toBeVisible();
  await expect(edit.getByText('Replace with', { exact: true })).toBeVisible();
  await expect(edit.locator('code').filter({ hasText: 'DescriptiveName' })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath('formatted-tool-details.png'),
    fullPage: true,
  });
});

test('stream ANSI colors and progress updates before bash returns, reconnect without duplicates', async ({
  page,
}, testInfo) => {
  const failures: string[] = [];
  page.on('pageerror', (error) => failures.push(error.message));
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  let historyReads = 0;
  page.on('request', (request) => {
    if (
      new URL(request.url()).pathname.endsWith(`/sessions/${session.ID}/messages`) &&
      request.method() === 'GET'
    )
      historyReads++;
  });
  fixture.histories.set(session.ID, [
    message(session, 'shell-plan', '', {
      ToolCalls: [
        { ID: 'shell-call', Name: 'bash', Input: { command: 'npm run test', timeout: 60000 } },
      ],
    }),
  ]);
  const record = processRecord(session);
  fixture.processes.set(record.ID, record);
  fixture.event(session, 'tool.start', {});
  fixture.event(session, 'process.start', record);
  const shell = page.locator('[data-call-id="shell-call"]');
  await shell.locator('.activity-toggle').click();
  await expect.poll(() => fixture.processSockets.size).toBe(1);
  fixture.processChunk(
    record.ID,
    'stdout',
    '\x1b[1m\x1b[30m\x1b[46m RUN \x1b[49m\x1b[39m\x1b[22m \x1b[36mv4.1.11\x1b[39m\n',
  );
  fixture.processChunk(record.ID, 'stdout', '\x1b[3');
  fixture.processChunk(record.ID, 'stdout', '2mBuild passed\x1b[0m\n');
  fixture.processChunk(record.ID, 'stdout', new Uint8Array([0xe2]));
  fixture.processChunk(record.ID, 'stderr', 'stderr\n');
  fixture.processChunk(record.ID, 'stdout', new Uint8Array([0x82, 0xac, 0x0a]));
  fixture.processChunk(record.ID, 'stdout', 'Progress 10%\r\x1b[2KProgress 100%\n');
  fixture.processChunk(
    record.ID,
    'stdout',
    '\x1b[38;2;18;171;99mRGB_FOREGROUND\x1b[0m\n\x1b[48;2;16;100;180mRGB_BACKGROUND\x1b[0m\n',
  );
  await expect(shell.locator('.xterm-screen')).toBeVisible();
  await expect(shell).toContainText('Build passed');
  await expect(shell).toContainText('Progress 100%');
  await expect(shell).toContainText('€');
  await expect(
    shell.locator('.xterm-screen span').filter({ hasText: /^RGB_FOREGROUND$/ }),
  ).toHaveCSS('color', 'rgb(18, 171, 99)');
  await expect(
    shell.locator('.xterm-screen span').filter({ hasText: /^RGB_BACKGROUND$/ }),
  ).toHaveCSS('background-color', 'rgb(16, 100, 180)');
  expect(await shell.textContent()).not.toContain('\x1b');
  expect(await shell.textContent()).not.toContain('Progress 10%');
  await expect(shell.getByRole('status')).toHaveText('Live');
  const reads = historyReads;
  for (let index = 0; index < 5; index++)
    fixture.event(session, 'process.output', { ProcessID: record.ID, Bytes: 3 });
  await page.waitForTimeout(350);
  expect(historyReads).toBe(reads);
  fixture.disconnectProcess(record.ID);
  await expect.poll(() => fixture.processConnections.length).toBe(2);
  await expect.poll(() => fixture.processSockets.size).toBe(1);
  await expect(shell).toContainText('Build passed');
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
  await shell.getByRole('button', { name: 'Copy terminal output', exact: true }).click();
  const copied = await page.evaluate(() => navigator.clipboard.readText());
  expect(copied.match(/Build passed/g)).toHaveLength(1);
  expect(copied).not.toContain('\x1b');
  expect(fixture.processConnections).toHaveLength(2);
  for (const socket of fixture.processSockets.keys())
    socket.send(
      JSON.stringify({
        process_id: record.ID,
        stream: 'stdout',
        data_base64: Buffer.from('missing').toString('base64'),
        offset: 999999,
        snapshot: false,
      }),
    );
  await expect.poll(() => fixture.processConnections.length).toBe(3);
  await expect(shell).toContainText('Build passed');
  fixture.processExit(record.ID, 7);
  await expect(shell.getByRole('status')).toHaveText('Exited 7');
  expect(fixture.processConnections).toHaveLength(3);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('live-colored-terminal.png'), fullPage: true });
  await shell.locator('.activity-toggle').click();
  await expect.poll(() => fixture.processSockets.size).toBe(0);
  expect(failures).toEqual([]);
});

test('keep manual terminal scroll and release the stream when the view closes', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  fixture.histories.set(session.ID, [
    message(session, 'shell-plan', '', {
      ToolCalls: [{ ID: 'shell-call', Name: 'bash', Input: { command: 'watch logs' } }],
    }),
  ]);
  const record = processRecord(session);
  fixture.processes.set(record.ID, record);
  fixture.event(session, 'tool.start', {});
  fixture.event(session, 'process.start', record);
  const shell = page.locator('[data-call-id="shell-call"]');
  await shell.locator('.activity-toggle').click();
  await expect.poll(() => fixture.processSockets.size).toBe(1);
  fixture.processChunk(
    record.ID,
    'stdout',
    Array.from({ length: 150 }, (_, index) => `line ${index}\n`).join(''),
  );
  await expect(shell).toContainText('line 149');
  await shell.locator('.xterm-helper-textarea').press('Shift+PageUp');
  await expect(
    shell.getByRole('button', { name: 'Follow terminal output', exact: true }),
  ).toBeVisible();
  fixture.processChunk(record.ID, 'stdout', 'new line while reading history\n');
  await page.waitForTimeout(250);
  await expect(
    shell.getByRole('button', { name: 'Follow terminal output', exact: true }),
  ).toBeVisible();
  await shell.getByRole('button', { name: 'Follow terminal output', exact: true }).click();
  await expect(shell).toContainText('new line while reading history');
  await shell.locator('.activity-toggle').click();
  await expect.poll(() => fixture.processSockets.size).toBe(0);
});

test('bind processes to the exact response when a provider reuses a call ID', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  fixture.histories.set(session.ID, [
    message(session, 'old-plan', '', {
      ToolCalls: [{ ID: 'shell-call', Name: 'bash', Input: { command: 'old command' } }],
    }),
    message(session, 'old-result', '\x1b[31mOld output\x1b[0m', {
      Role: 'tool',
      ToolCallID: 'shell-call',
    }),
    message(session, 'shell-plan', '', {
      ToolCalls: [{ ID: 'shell-call', Name: 'bash', Input: { command: 'new command' } }],
    }),
  ]);
  const record = processRecord(session);
  fixture.processes.set(record.ID, record);
  fixture.event(session, 'tool.start', {});
  fixture.event(session, 'process.start', record);
  const tools = page.locator('[data-call-id="shell-call"]');
  await tools.first().locator('.activity-toggle').click();
  await expect(tools.first()).toContainText('Old output');
  expect(fixture.processSockets.size).toBe(0);
  await tools.last().locator('.activity-toggle').click();
  await expect.poll(() => fixture.processSockets.size).toBe(1);
  fixture.processChunk(record.ID, 'stdout', 'Only the new command\n');
  await expect(tools.last()).toContainText('Only the new command');
  await expect(tools.first()).not.toContainText('Only the new command');
});
