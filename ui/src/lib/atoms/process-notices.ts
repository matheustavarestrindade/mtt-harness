import type { Message, ToolCall } from './types';

export interface ProcessNotice {
  processID: string;
  kind: 'exit' | 'inactivity' | 'output';
  output: string;
}

const processEnvelope =
  '[process] Runtime update from a background process, not a new user request.\n';

// Match the harness envelope only in runtime data. Quoted logs in user/assistant
// messages and unrelated plugin runtime messages remain ordinary chat content.
export function processNotice(message: Message): ProcessNotice | null {
  if (message.Role !== 'runtime' || message.ToolCalls?.length || message.Content?.length !== 1)
    return null;
  const content = message.Content[0];
  if (content.Type !== 'text' || !content.Text.startsWith(processEnvelope)) return null;
  const match = /^process ([a-zA-Z0-9_-]+)( stopped)?: ([\s\S]*)$/.exec(
    content.Text.slice(processEnvelope.length),
  );
  if (!match) return null;
  return {
    processID: match[1],
    kind: match[2]
      ? 'exit'
      : match[3].startsWith('state=running; no output for ')
        ? 'inactivity'
        : 'output',
    output: match[3],
  };
}

// Before source IDs were added to ProcessSpec, wait=false returned this exact
// process receipt. Its explicit ID can link old output without command/time guesses.
export function backgroundProcessReceipt(call: ToolCall, result: Message | null): string | null {
  if (
    call.Name !== 'bash' ||
    !call.Input ||
    typeof call.Input !== 'object' ||
    !('wait' in call.Input) ||
    call.Input.wait !== false ||
    result?.Content?.length !== 1
  )
    return null;
  const content = result.Content[0];
  if (content.Type !== 'text') return null;
  return /^process ([a-zA-Z0-9_-]+) is started$/.exec(content.Text)?.[1] ?? null;
}
