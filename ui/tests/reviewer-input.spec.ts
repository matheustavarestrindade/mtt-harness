import { test, expect } from '@playwright/test';
import { loadReviewerInput } from '../src/lib/molecules/reviewer-input';
import type { HarnessApi } from '../src/lib/molecules/api/client';
import type { HistoryQuery } from '../src/lib/atoms/session-view';

test('read exact first input at sequence one with a bounded revision-fenced query', async () => {
  const requests: HistoryQuery[] = [],
    text = 'Literal 🌍\n<script>not executed</script>\n';
  const api = {
    history: async (_session: string, query: HistoryQuery) => {
      requests.push(query);
      return query.metadata
        ? { SessionID: 'worker', Revision: 8, TaskMessageID: 'input', TaskSeq: 1 }
        : {
            SessionID: 'worker',
            Revision: 8,
            Messages: [
              {
                ID: 'input',
                Seq: 1,
                SessionID: 'worker',
                Role: 'user',
                Content: [{ Type: 'text', Text: text }],
              },
            ],
          };
    },
  } as unknown as HarnessApi;
  expect(await loadReviewerInput(api, 'worker', new AbortController().signal)).toBe(text);
  expect(requests).toEqual([{ metadata: true }, { before: 2, limit: 1, revision: 8 }]);
});

test('reject foreign, stale, missing, and mismatched input references', async () => {
  const cases = [
    { metadata: { SessionID: 'other', Revision: 8, TaskMessageID: 'input', TaskSeq: 1 } },
    { metadata: { SessionID: 'worker', Revision: 8 } },
    { metadata: { SessionID: 'worker', Revision: 8, TaskMessageID: 'input', TaskSeq: -1 } },
    { body: { SessionID: 'other', Revision: 8, Messages: [] } },
    { body: { SessionID: 'worker', Revision: 9, Messages: [] } },
    {
      body: {
        SessionID: 'worker',
        Revision: 8,
        Messages: [
          {
            ID: 'wrong',
            Seq: 1,
            SessionID: 'worker',
            Role: 'user',
            Content: [{ Type: 'text', Text: 'wrong' }],
          },
        ],
      },
    },
  ];
  for (const value of cases) {
    const api = {
      history: async (_session: string, query: HistoryQuery) =>
        query.metadata
          ? (value.metadata ?? {
              SessionID: 'worker',
              Revision: 8,
              TaskMessageID: 'input',
              TaskSeq: 1,
            })
          : value.body,
    } as unknown as HarnessApi;
    await expect(loadReviewerInput(api, 'worker', new AbortController().signal)).rejects.toThrow();
  }
});
