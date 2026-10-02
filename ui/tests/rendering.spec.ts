import { readFile } from 'node:fs/promises';
import { expect, test } from '@playwright/test';
import type { Message, Session } from '../src/lib/atoms/types';
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

test('format Markdown, highlight code, and parse quoted CSV without executing content', async ({
  page,
}, testInfo) => {
  const errors: string[] = [];
  page.on('pageerror', (failure) => errors.push(failure.message));
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  const csv = 'Name,Note,Count\n"alpha, one","two\nlines",3\nbeta,"a ""quote""",4';
  const text = `## Build notes\n\nUse **clear names** and \`go test\`.\n\n- First step\n- Second step\n\n> A short quote.\n\n[Guide](https://example.com/guide) and [unsafe](javascript:alert(1)).\n\n| Task | Result |\n| --- | --- |\n| Tests | Passed |\n\n\`\`\`go\npackage main\n\nfunc main() { println("hello") }\n\`\`\`\n\n\`\`\`csv\n${csv}\n\`\`\`\n\n<img src=x onerror="window.injected=true">\n<script>window.injected=true</script>`;
  fixture.histories.set(session.ID, [message(session, 'formatted', text)]);
  fixture.event(session, 'turn.end', { status: 'completed' });
  const article = page.getByRole('article', { name: 'assistant message' });
  await expect(article.getByRole('heading', { name: 'Build notes' })).toBeVisible();
  await expect(article.locator('strong')).toHaveText('clear names');
  await expect(article.getByRole('listitem')).toHaveText(['First step', 'Second step']);
  await expect(article.getByRole('link', { name: 'Guide' })).toHaveAttribute(
    'rel',
    'noopener noreferrer',
  );
  await expect(article.getByRole('link', { name: 'unsafe', exact: true })).toHaveCount(0);
  await expect(article.getByRole('cell', { name: 'Passed', exact: true })).toBeVisible();
  const code = article.getByRole('region', { name: 'Code block' });
  await expect(code.locator('.hljs-keyword').first()).toBeVisible();
  const csvBlock = article.getByRole('region', { name: 'CSV block' });
  await expect(csvBlock.getByRole('cell', { name: 'alpha, one', exact: true })).toBeVisible();
  await expect(csvBlock.getByRole('cell', { name: 'two lines', exact: true })).toBeVisible();
  await expect(csvBlock.getByRole('cell', { name: 'a "quote"', exact: true })).toBeVisible();
  await csvBlock.getByRole('button', { name: 'Source', exact: true }).click();
  await expect(csvBlock.locator('code')).toHaveText(csv);
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
  await csvBlock.getByRole('button', { name: 'Copy code', exact: true }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(csv);
  const downloaded = page.waitForEvent('download');
  await csvBlock.getByRole('button', { name: 'Download source', exact: true }).click();
  const download = await downloaded;
  expect(await readFile((await download.path())!, 'utf8')).toBe(csv);
  expect(await page.evaluate(() => 'injected' in window)).toBe(false);
  await expect(article.locator('img, script')).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('rich-message.png'), fullPage: true });
  expect(errors).toEqual([]);
});

test('combine calls and out-of-order results and attach later output to an open section', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  await page.getByLabel('Message', { exact: true }).fill('wait');
  await page.getByRole('button', { name: 'Send message', exact: true }).click();
  const session = fixture.sessions[0];
  fixture.histories.set(session.ID, [
    message(session, 'plan', 'I will read the report and run a check.', {
      ToolCalls: [
        { ID: 'call-read', Name: 'read', Input: { path: 'report.csv' } },
        { ID: 'call-shell', Name: 'bash', Input: { command: 'exit 1' } },
      ],
    }),
    message(session, 'result-shell', 'Command failed: exit code 1', {
      Role: 'tool',
      ToolCallID: 'call-shell',
    }),
  ]);
  fixture.event(session, 'tool.result', { CallID: 'call-shell', Status: 'error' });
  const read = page.getByRole('group', { name: 'Tool read', exact: true });
  const shell = page.getByRole('group', { name: 'Tool bash', exact: true });
  await expect(page.locator('.tool-activity')).toHaveCount(2);
  await expect(page.getByText('Command failed: exit code 1', { exact: true })).toHaveCount(0);
  await read.getByRole('button', { name: /^read Waiting/ }).click();
  await expect(read).toContainText('Waiting for the tool result');
  fixture.histories.get(session.ID)!.push(
    message(session, 'result-read', 'Name,Score\nAlice,10\nBob,9', {
      Role: 'tool',
      ToolCallID: 'call-read',
    }),
  );
  fixture.event(session, 'tool.result', { CallID: 'call-read', Status: 'ok' });
  await expect(read.getByRole('cell', { name: 'Alice', exact: true })).toBeVisible();
  await expect(read).toContainText('report.csv');
  await expect(page.locator('.tool-activity')).toHaveCount(2);
  await shell.getByRole('button', { name: /^bash Result/ }).click();
  await expect(shell).toContainText('Command failed: exit code 1');
  await expect(page.getByRole('article', { name: 'assistant message' })).toHaveCount(1);
  await read.getByRole('button', { name: /^read Result/ }).click();
  await expect(read.getByRole('region', { name: 'CSV table' })).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('unified-tools.png'), fullPage: true });
});

test('bound large code and CSV previews and retain full downloadable source', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  const large = 'const longName = "value";\n'.repeat(8000);
  const csv = `Name,Value\n${Array.from({ length: 1000 }, (_, index) => `row-${index},${index}`).join('\n')}`;
  fixture.histories.set(session.ID, [
    message(session, 'large', '', {
      ToolCalls: [
        { ID: 'code', Name: 'read', Input: { path: 'large.ts' } },
        { ID: 'csv', Name: 'read', Input: { path: 'large.csv' } },
      ],
    }),
    message(session, 'code-output', large, { Role: 'tool', ToolCallID: 'code' }),
    message(session, 'csv-output', csv, { Role: 'tool', ToolCallID: 'csv' }),
  ]);
  fixture.event(session, 'turn.end', {});
  const sections = page.getByRole('group', { name: 'Tool read', exact: true });
  await expect(sections).toHaveCount(2);
  await sections.nth(0).getByRole('button', { name: /^read/ }).click();
  await expect(sections.nth(0)).toContainText('Preview limited to 60,000 characters');
  await expect(sections.nth(0).locator('.hljs-keyword')).toHaveCount(0);
  await sections.nth(1).getByRole('button', { name: /^read/ }).click();
  await expect(sections.nth(1).getByRole('row')).toHaveCount(201);
  await expect(sections.nth(1)).toContainText('Preview limited');
  const downloaded = page.waitForEvent('download');
  await sections.nth(0).getByRole('button', { name: 'Download source' }).last().click();
  const download = await downloaded;
  expect(await readFile((await download.path())!, 'utf8')).toBe(large);
  fixture.histories.get(session.ID)!.push(
    message(session, 'orphan', 'Unmatched output remains available', {
      Role: 'tool',
      ToolCallID: 'missing-call',
    }),
  );
  fixture.event(session, 'turn.end', {});
  const orphan = page.getByRole('group', { name: 'Tool Tool result', exact: true });
  await orphan.getByRole('button', { name: /^Tool result/ }).click();
  await expect(orphan).toContainText('Unmatched output remains available');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
