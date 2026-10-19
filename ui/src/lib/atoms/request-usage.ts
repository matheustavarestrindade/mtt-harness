import type { Message, Model } from './types';

// A provider report belongs to one completed/partial assistant response. It is
// not a token count for queued messages, later tool results, or other sessions.
export interface RequestUsage {
  messageID: string;
  modelID: string | null;
  reportedAt: string;
  inputTokens: number | null;
  outputTokens: number | null;
  contextTokens: number | null;
  contextLimit: number | null;
  contextPercentage: number | null;
  cacheReadTokens: number | null;
  cacheWriteTokens: number | null;
  cacheHitPercentage: number | null;
}

function tokenCount(value: unknown): number | null {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 ? value : null;
}

export function latestRequestUsage(
  messages: Message[],
  sessionID: string,
  requestModels: ReadonlyMap<string, string>,
  models: Model[],
): RequestUsage | null {
  let message: Message | undefined;
  for (let index = messages.length - 1; index >= 0; index--) {
    if (messages[index].SessionID !== sessionID || messages[index].Role !== 'assistant') continue;
    message = messages[index];
    break;
  }
  if (!message) return null;
  const usage = message.Usage;
  const uncached = tokenCount(usage?.Input);
  const cacheRead = tokenCount(usage?.CacheRead);
  const cacheWrite = tokenCount(usage?.CacheWrite);
  const input =
    uncached === null || cacheRead === null || cacheWrite === null
      ? null
      : tokenCount(uncached + cacheRead + cacheWrite);
  // Output already includes reasoning tokens. Adding Reasoning again would
  // inflate both the output value and the last-reported context footprint.
  const output = tokenCount(usage?.Output);
  const context = input === null || output === null ? null : tokenCount(input + output);
  const modelID = requestModels.get(message.ID) ?? null;
  const maximum = tokenCount(models.find((model) => model.ID === modelID)?.ContextMax);
  const limit = maximum !== null && maximum > 0 ? maximum : null;
  return {
    messageID: message.ID,
    modelID,
    reportedAt: message.CreatedAt,
    inputTokens: input,
    outputTokens: output,
    contextTokens: context,
    contextLimit: limit,
    contextPercentage: context === null || limit === null ? null : (context / limit) * 100,
    cacheReadTokens: cacheRead,
    cacheWriteTokens: cacheWrite,
    cacheHitPercentage:
      input === null || input === 0 || cacheRead === null ? null : (cacheRead / input) * 100,
  };
}
