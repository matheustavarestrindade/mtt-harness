import type { Message } from './types';

// Saved messages normally do not change. Keep their identity across recovery
// reads without serializing attachment bytes into temporary JSON strings.
function sameJSONValue(first: unknown, second: unknown, depth = 0): boolean {
  if (first === second) return true;
  if (depth >= 32 || !first || !second || typeof first !== 'object' || typeof second !== 'object')
    return false;
  if (Array.isArray(first) || Array.isArray(second)) {
    return (
      Array.isArray(first) &&
      Array.isArray(second) &&
      first.length === second.length &&
      first.every((value, index) => sameJSONValue(value, second[index], depth + 1))
    );
  }
  const firstFields = first as Record<string, unknown>;
  const secondFields = second as Record<string, unknown>;
  const keys = Object.keys(firstFields);
  return (
    keys.length === Object.keys(secondFields).length &&
    keys.every(
      (key) =>
        Object.hasOwn(secondFields, key) &&
        sameJSONValue(firstFields[key], secondFields[key], depth + 1),
    )
  );
}

export function reconcileMessageHistory(previous: Message[], incoming: Message[]): Message[] {
  const saved = new Map(previous.map((message) => [message.ID, message]));
  const reconciled = incoming.map((message) => {
    const existing = saved.get(message.ID);
    return existing && sameJSONValue(existing, message) ? existing : message;
  });
  return previous.length === reconciled.length &&
    previous.every((message, index) => message === reconciled[index])
    ? previous
    : reconciled;
}
