import { expect, test } from '@playwright/test';
import {
  parseWatcherInput,
  watcherConversation,
  watcherPreviewLimit,
} from '../src/lib/atoms/watcher-presentation';

const completion =
  '<watcher_event>{"context_tokens":0,"in_progress_message_id":"","model":"","permission_id":"","source_active":false,"source_phase":"completed_turn","type":"end_turn"}</watcher_event>';

test('project the real completion envelope without treating defaults as usage', () => {
  const source = `${completion}\nReview the task.\n<todo>[{"ID":"todo-1","Title":"Check behavior","Status":"pending"}]</todo>`;
  const preview = parseWatcherInput(source);
  expect(preview.fields).toEqual([
    ['Trigger', 'AI finish'],
    ['Phase', 'Completed turn'],
  ]);
  expect(preview.parts[0].text).toBe('Review the task.');
  expect(preview.parts[1].value).toEqual([
    { ID: 'todo-1', Title: 'Check behavior', Status: 'pending' },
  ]);
  expect(source.startsWith(completion)).toBe(true);
  const growth = parseWatcherInput(
    '<watcher_event>{"type":"context_growth","context_tokens":0,"model":"provider/model","source_active":true}</watcher_event>',
  );
  expect(growth.fields).toContainEqual(['Context at trigger', '0 tokens']);
  expect(growth.fields).toContainEqual(['Reviewed model', 'provider/model']);
});

test('preserve malformed metadata and literal or quoted tags', () => {
  const malformed = '<watcher_event>{"type":"end_turn"</watcher_event>\nLiteral text';
  expect(parseWatcherInput(malformed).parts[0].text).toBe(malformed);
  const example = `${completion}\nRead this example:\n\`\`\`xml\n<todo>Literal example</todo>\n\`\`\`\nA literal <watcher_event>quote</watcher_event> stays visible.`;
  const preview = parseWatcherInput(example);
  expect(preview.parts).toHaveLength(1);
  expect(preview.parts[0].text).toContain('<todo>Literal example</todo>');
  expect(preview.parts[0].text).toContain('<watcher_event>quote</watcher_event>');
  expect(
    parseWatcherInput('<watcher_event>{"type":"unrecognized"}</watcher_event>').event,
  ).toBeNull();
});

test('keep complete conversation rows when a saved preview ends inside JSON', () => {
  const first = {
    Role: 'user',
    Content: [{ Type: 'text', Text: 'Keep {these} brackets and "quoted text".' }],
  };
  const source = `${completion}\n<conversation_to_analize>[${JSON.stringify(first)},\n{"Role":"assistant","Content":[{"Type":"text","Text":"unfinished`;
  const part = parseWatcherInput(source, true).parts[0];
  expect(part.truncated).toBe(true);
  expect(watcherConversation(part)).toEqual({ messages: [first], omitted: false, partial: true });
  const full = parseWatcherInput(
    `${completion}\n<conversation_to_analize>${JSON.stringify([first, { Role: 'assistant', InProgress: true, Content: [] }])}</conversation_to_analize>`,
  ).parts[0];
  expect(watcherConversation(full)?.messages[1].InProgress).toBe(true);
  const duplicate = `${completion}\n<trigger>${completion.slice('<watcher_event>'.length, -'</watcher_event>'.length)}</trigger>\nReview`;
  expect(parseWatcherInput(duplicate).parts).toHaveLength(1);
});

test('bound large previews and transcript rows without changing source data', () => {
  const source = 'x'.repeat(watcherPreviewLimit - 1) + '🌍tail';
  const result = parseWatcherInput(source);
  expect(result.limited).toBe(true);
  expect(result.parts[0].text).toHaveLength(watcherPreviewLimit - 1);
  expect(source.endsWith('🌍tail')).toBe(true);
  const messages = Array.from({ length: 70 }, (_, index) => ({
    Role: 'user',
    Content: [{ Type: 'text', Text: String(index) }],
  }));
  const conversation = watcherConversation(
    parseWatcherInput(
      `<conversation_to_analize>${JSON.stringify(messages)}</conversation_to_analize>`,
    ).parts[0],
  );
  expect(conversation?.messages).toHaveLength(40);
  expect(conversation?.omitted).toBe(true);
});
