import { expect, test } from '@playwright/test';
import type { Message } from '../src/lib/atoms/types';
import { assistantDisplayMessage } from '../src/lib/atoms/assistant-display';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';

const label = '[context_message id=27 role=assistant]';

function message(sessionID: string, text: string): Message {
  return {
    ID: 'memory-answer',
    SessionID: sessionID,
    Seq: 2,
    Role: 'assistant',
    Content: [
      { Type: 'text', Text: text, Data: null, MIME: '', URL: '', Filename: '', AudioID: '' },
    ],
    ToolCalls: null,
    ToolCallID: '',
    Usage: null,
    CreatedAt: new Date().toISOString(),
  };
}

test('hide only the reserved leading assistant label, including partial streams', () => {
  for (let length = 1; length <= label.length; length++) {
    expect(
      assistantDisplayMessage(message('session', label.slice(0, length)), true).Content![0].Text,
    ).toBe('');
  }
  for (const text of [`Quoted: ${label}`, `\`\`\`text\n${label}\n\`\`\``, '> ' + label]) {
    expect(assistantDisplayMessage(message('session', text)).Content![0].Text).toBe(text);
  }
  for (const role of ['user', 'tool'] as const) {
    const literal = { ...message('session', label), Role: role };
    expect(assistantDisplayMessage(literal)).toBe(literal);
  }
});

test('hide legacy and streamed labels while preserving the reply and copy text', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const session = fixture.sessions[0];
  fixture.histories.set(session.ID, [
    message(session.ID, `${label}\n\nI'll call you Matheus from now on.`),
  ]);
  fixture.event(session, 'turn.end', { status: 'completed' });
  const answer = page.getByRole('article', { name: 'assistant message' }).first();
  await expect(answer).toContainText("I'll call you Matheus from now on.");
  await expect(answer).not.toContainText('context_message');
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
  await answer.hover();
  await answer.getByRole('button', { name: 'Copy message', exact: true }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    "I'll call you Matheus from now on.",
  );
  await page.getByLabel('Message', { exact: true }).fill('wait');
  await page.getByRole('button', { name: 'Send message', exact: true }).click();
  fixture.event(session, 'model.call', { model: fixture.model.ID, message_id: 'streamed-memory' });
  fixture.event(session, 'model.chunk', {
    message_id: 'streamed-memory',
    text: '[context_message id=27 role=ass',
  });
  await expect(page.getByRole('region', { name: 'Conversation', exact: true })).not.toContainText(
    'context_message',
  );
  fixture.event(session, 'model.chunk', {
    message_id: 'streamed-memory',
    text: 'istant]\nHello again, Matheus.',
  });
  await expect(page.getByText('Hello again, Matheus.', { exact: true })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Conversation', exact: true })).not.toContainText(
    'context_message',
  );
});
