import type { Message } from './types';

const contextLabel = /^(?:\s*\[context_message id=\d+ role=assistant\]\s*)+/;
const contextLabelStart = '[context_message id=';

function visibleAssistantText(text: string, streaming: boolean): string {
  const visible = text.replace(contextLabel, '');
  if (!streaming) return visible;
  const prefix = visible.trimStart();
  if (prefix && contextLabelStart.startsWith(prefix)) return '';
  const incomplete = /^\[context_message id=\d+(.*)$/.exec(prefix);
  if (incomplete && ' role=assistant]'.startsWith(incomplete[1])) {
    return '';
  }
  return visible;
}

// Remove only the legacy reserved leading label, including a streamed partial
// label. User text, tool output, and quoted/code examples remain literal.
export function assistantDisplayMessage(message: Message, streaming = false): Message {
  if (message.Role !== 'assistant') return message;
  const position = message.Content?.findIndex((content) => content.Type === 'text') ?? -1;
  if (position < 0 || !message.Content) return message;
  const text = message.Content[position].Text;
  const visible = visibleAssistantText(text, streaming);
  if (text === visible) return message;
  const content = [...message.Content];
  content[position] = { ...content[position], Text: visible };
  return { ...message, Content: content };
}
