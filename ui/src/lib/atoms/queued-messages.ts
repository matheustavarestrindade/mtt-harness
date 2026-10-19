import type { Message } from './types';

export interface QueuedMessageView {
  id: string;
  message?: Message;
  preview?: QueuedMessagePreview;
}

export interface QueuedMessagePreview {
  ID: string;
  SessionID: string;
  Role: Message['Role'];
  Content: Message['Content'];
  CreatedAt: string;
  Truncated: boolean;
}

// Queue IDs define membership. Server previews survive reload/reconnect; a local
// accepted receipt or explicit full read can supply the original media content.
export function queuedMessageViews(
  receipts: Message[],
  pending: string[],
  saved: Set<string>,
  restored: QueuedMessagePreview[] = [],
): QueuedMessageView[] {
  const previews = new Map(receipts.map((message) => [message.ID, message]));
  const serverPreviews = new Map(restored.map((preview) => [preview.ID, preview]));
  return [...new Set([...pending, ...receipts.map((message) => message.ID)])]
    .filter((identifier) => !saved.has(identifier))
    .map((id) => ({ id, message: previews.get(id), preview: serverPreviews.get(id) }))
    .filter((entry) => (entry.message?.Role ?? entry.preview?.Role) !== 'runtime');
}
