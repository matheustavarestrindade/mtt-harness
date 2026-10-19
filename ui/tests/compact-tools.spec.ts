import { expect, test } from '@playwright/test';
import type { Message, Session } from '../src/lib/atoms/types';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';
function message(
  session: Session,
  id: string,
  text: string,
  overrides: Partial<Message> = {},
): Message {
  return {
    ID: id,
    SessionID: session.ID,
    Seq: 1,
    Role: 'assistant',
    CreatedAt: new Date().toISOString(),
    Content: [
      { Type: 'text', Text: text, Data: null, MIME: '', URL: '', Filename: '', AudioID: '' },
    ],
    ToolCalls: null,
    ToolCallID: '',
    Usage: null,
    ...overrides,
  };
}
test('compact replacements and commands retain content without metadata headings', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  const old =
    '# Uses internal Docker DNS by default\ndocker compose run --rm -e PREVIEW_URL=http://web:8080 checks';
  const replacement = '# Uses internal Docker DNS\ndocker compose run --rm checks';
  fixture.histories.set(session.ID, [
    message(session, 'plan', '', {
      ToolCalls: [
        {
          ID: 'call_replace',
          Name: 'file_actions',
          Input: {
            return: { type: 'summary' },
            actions: [
              { op: 'replace', old_text: old, new_text: replacement },
              { op: 'replace', old_text: 'Old explanation', new_text: 'New explanation' },
            ],
            path: 'docs/docker.md',
          },
        },
        {
          ID: 'call_shell',
          Name: 'bash',
          Input: {
            command: 'docker run --rm hashwatcher-checks node docker/smoke.mjs',
            timeout: 90000,
          },
        },
      ],
    }),
    message(session, 'updated', 'Updated', { Role: 'tool', ToolCallID: 'call_replace' }),
  ]);
  fixture.event(session, 'tool.end', {});
  const edit = page.locator('[data-call-id="call_replace"]');
  await edit.locator('.activity-toggle').click();
  await expect(edit.getByText('docs/docker.md', { exact: true })).toBeVisible();
  for (const label of [
    'Input',
    'Output',
    'Path',
    'Return',
    'Type',
    'markdown',
    'text',
    'Find',
    'Replace with',
  ])
    await expect(edit.getByText(label, { exact: true })).toHaveCount(0);
  await expect(
    edit.getByRole('region', { name: 'Original text', exact: true }).first().locator('code'),
  ).toHaveText(old);
  await expect(
    edit.getByRole('region', { name: 'Replacement text', exact: true }).first().locator('code'),
  ).toHaveText(replacement);
  await expect(edit.getByRole('region', { name: 'Tool output' })).toHaveText('Updated');
  await expect(edit).not.toContainText('Call call_');
  await edit.getByRole('button', { name: 'Tool options', exact: true }).click();
  await expect(edit.getByText('summary', { exact: true })).toBeVisible();
  await edit.getByRole('button', { name: 'Tool options', exact: true }).click();
  const shell = page.locator('[data-call-id="call_shell"]');
  await shell.locator('.activity-toggle').click();
  const input = shell.getByRole('region', { name: 'Tool input' });
  await expect(input.getByText('bash', { exact: true })).toHaveCount(0);
  await expect(input).toContainText('90 s');
  const inputBox = (await input.boundingBox())!,
    codeBox = (await input.locator('pre').boundingBox())!;
  expect(codeBox.y - inputBox.y).toBeLessThan(10);
  expect(inputBox.height).toBeLessThan(90);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('compact-tools.png'), fullPage: true });
});
