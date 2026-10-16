import type { Message } from './types';

export interface QueuedMessageView {
  id: string;
  message?: Message;
}

// Queue IDs are authoritative; receipts provide this client's bounded previews.
// Other clients' messages have an ID-only view, without invented user content.
export function queuedMessageViews(
  receipts: Message[],
  pending: string[],
  saved: Set<string>,
): QueuedMessageView[] {
  const previews = new Map(receipts.map((message) => [message.ID, message]));
  return [...new Set([...pending, ...receipts.map((message) => message.ID)])]
    .filter((identifier) => !saved.has(identifier))
    .map((id) => ({ id, message: previews.get(id) }));
}
