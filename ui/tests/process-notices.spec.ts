import { expect, test } from '@playwright/test';
import { buildConversationEntries } from '../src/lib/atoms/transcript';
import type { ProcessRecord } from '../src/lib/atoms/processes';
import type { Message, Session } from '../src/lib/atoms/types';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';

const envelope = '[process] Runtime update from a background process, not a new user request.\n';
const processID = '159fda487287f5ec0f2246f890a83455';
function message(identifier: string, text: string, overrides: Partial<Message> = {}): Message {
  return {
    ID: identifier,
    SessionID: 'session',
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
function history(session?: Session) {
  return [
    message('launch', '', {
      ToolCalls: [
        {
          ID: 'background-call',
          Name: 'bash',
          Input: { command: 'docker compose up --build --wait', wait: false },
        },
      ],
    }),
    message('receipt', `process ${processID} is started`, {
      Role: 'tool',
      ToolCallID: 'background-call',
    }),
    message(
      'notice',
      envelope +
        `process ${processID} stopped: container hashwatcher-web-1 is unhealthy\nexit status 1`,
      { Role: 'runtime' },
    ),
  ].map((entry) => (session ? { ...entry, SessionID: session.ID } : entry));
}
function record(session: Session): ProcessRecord {
  return {
    ID: processID,
    InstanceID: session.InstanceID,
    SessionID: session.ID,
    PID: 1,
    Status: 'stopped',
    StartedAt: new Date().toISOString(),
    EndedAt: new Date().toISOString(),
    Exit: { Code: 1, Signal: '', Error: 'exit status 1' },
    Spec: { Command: 'sh', Args: [], Cwd: '', Notify: { Mode: 'exit', Interval: 0 }, Timeout: 0 },
  };
}

test('fold exact legacy background receipts without mutating agent history', () => {
  const messages = history();
  const original = JSON.stringify(messages);
  const entries = buildConversationEntries(messages);
  expect(entries).toHaveLength(1);
  expect(entries[0].tools[0].processNotice?.output).toContain('hashwatcher-web-1 is unhealthy');
  expect(JSON.stringify(messages)).toBe(original);
  expect(entries[0].tools[0].result?.Content?.[0].Text).toBe(`process ${processID} is started`);
});

test('keep unrelated runtime and quoted notices; never guess from a foreground command output', () => {
  const messages = history();
  messages[0].ToolCalls![0].Input = { command: `printf 'process ${processID} is started'` };
  messages.push(
    message('plugin', 'Context checkpoint complete.', { Role: 'runtime' }),
    message('quoted', messages[2].Content![0].Text, { Role: 'user' }),
  );
  const entries = buildConversationEntries(messages);
  expect(entries[0].tools[0].processNotice).toBeUndefined();
  expect(entries.find((entry) => entry.message.ID === 'notice')?.processNotice?.kind).toBe('exit');
  expect(entries.find((entry) => entry.message.ID === 'plugin')?.processNotice).toBeUndefined();
  expect(entries.find((entry) => entry.message.ID === 'quoted')?.processNotice).toBeUndefined();
});

test('honor response IDs and session boundaries when calls are reused', () => {
  const messages = history();
  messages.splice(
    2,
    0,
    message('new-launch', '', {
      ToolCalls: [{ ID: 'background-call', Name: 'bash', Input: { command: 'other', wait: true } }],
    }),
  );
  const process = record({ ID: 'session', InstanceID: 'workspace' } as Session);
  process.Spec.ToolMessageID = 'new-launch';
  process.Spec.ToolCallID = 'background-call';
  let entries = buildConversationEntries(messages, [process]);
  expect(entries[0].tools[0].processNotice).toBeUndefined();
  expect(entries[1].tools[0].processNotice?.kind).toBe('exit');
  process.Spec.ToolMessageID = 'outside-preview';
  entries = buildConversationEntries(messages, [process]);
  expect(entries[0].tools[0].processNotice).toBeUndefined();
  expect(entries.at(-1)?.processNotice?.kind).toBe('exit');
  const foreign = { ...messages.at(-1)!, ID: 'foreign', SessionID: 'other-session' };
  entries = buildConversationEntries([...messages, foreign], [process]);
  expect(entries.at(-1)?.message.ID).toBe('foreign');
});

test('coalesce unlinked process notices into one collapsed output entry', () => {
  const messages = [
    message('progress', envelope + `process ${processID}: output before exit`, { Role: 'runtime' }),
    ...history().slice(2),
    message('late', envelope + `process ${processID}: stale progress`, { Role: 'runtime' }),
  ];
  const entries = buildConversationEntries(messages);
  expect(entries).toHaveLength(1);
  expect(entries[0].processNotice?.output).toContain('is unhealthy');
});

test('show a failed process inside its bash section without a Runtime chat dump', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  fixture.histories.set(session.ID, history(session));
  fixture.processes.set(processID, record(session));
  fixture.event(session, 'process.notification', null);
  const shell = page.locator('[data-call-id="background-call"]');
  await expect(shell.locator('.activity-toggle')).toContainText('Exited 1');
  await expect(page.getByRole('article', { name: 'runtime message', exact: true })).toHaveCount(0);
  await expect(
    page.getByText('container hashwatcher-web-1 is unhealthy', { exact: true }),
  ).toHaveCount(0);
  await shell.locator('.activity-toggle').click();
  await expect(shell).toContainText('hashwatcher-web-1 is unhealthy');
  await expect(shell).not.toContainText('Runtime update from a background process');
  expect(fixture.processSockets.size).toBe(0);
  expect(fixture.histories.get(session.ID)?.at(-1)?.Role).toBe('runtime');
});

test('render foreground and background ANSI colors inside structured previews', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  fixture.histories.set(session.ID, [
    message('json-call', '', {
      SessionID: session.ID,
      ToolCalls: [{ ID: 'color-data', Name: 'mcp__build__check', Input: { target: 'test' } }],
    }),
    message(
      'json-result',
      JSON.stringify({
        stdout:
          '\x1b[38;2;18;171;99mCOLOR_OUTPUT\x1b[0m\n\x1b[48;2;16;100;180mCOLOR_BACKGROUND\x1b[0m\n<img src=x onerror=window.injected=true>',
        status: 'ok',
      }),
      { SessionID: session.ID, Role: 'tool', ToolCallID: 'color-data' },
    ),
  ]);
  fixture.event(session, 'tool.end', {});
  const tool = page.locator('[data-call-id="color-data"]');
  await tool.locator('.activity-toggle').click();
  await expect(
    tool.locator('[data-ansi-text] span').filter({ hasText: /^COLOR_OUTPUT$/ }),
  ).toHaveCSS('color', 'rgb(18, 171, 99)');
  await expect(
    tool.locator('[data-ansi-text] span').filter({ hasText: /^COLOR_BACKGROUND$/ }),
  ).toHaveCSS('background-color', 'rgb(16, 100, 180)');
  await expect(tool.locator('img,script')).toHaveCount(0);
  expect(await page.evaluate(() => 'injected' in window)).toBe(false);
});
