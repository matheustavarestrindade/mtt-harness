import type { HarnessApi } from './api/client';

/** Read one exact saved task body. Never scan the worker's output history. */
export async function loadReviewerInput(api: HarnessApi, sessionID: string, signal: AbortSignal) {
  const metadata = await api.history(sessionID, { metadata: true }, signal);
  const sequence = metadata.TaskSeq;
  if (metadata.SessionID !== sessionID)
    throw new Error('Reviewer history belongs to a different session.');
  if (
    !metadata.TaskMessageID ||
    !Number.isSafeInteger(sequence) ||
    !sequence ||
    sequence <= 0 ||
    sequence >= Number.MAX_SAFE_INTEGER
  )
    throw new Error('The saved reviewer input reference is unavailable.');
  if (!Number.isSafeInteger(metadata.Revision) || metadata.Revision < 0)
    throw new Error('The reviewer history revision is unavailable.');

  const page = await api.history(
    sessionID,
    { before: sequence + 1, limit: 1, revision: metadata.Revision },
    signal,
  );
  const message = page.Messages.find(
    (value) =>
      value.ID === metadata.TaskMessageID &&
      value.Seq === sequence &&
      value.SessionID === sessionID &&
      value.Role === 'user',
  );
  if (page.SessionID !== sessionID || page.Revision !== metadata.Revision || !message)
    throw new Error('Saved reviewer input no longer matches its history reference.');
  const content = message.Content ?? [];
  if (
    !content.length ||
    content.some((value) => value.Type !== 'text' || typeof value.Text !== 'string')
  )
    throw new Error('The saved reviewer input is not a text prompt.');
  return content.map((value) => value.Text).join('\n');
}
