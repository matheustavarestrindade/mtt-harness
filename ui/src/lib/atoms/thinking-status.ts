const previewCharacters = 120;
const reasoningWindow = 8192;

function plainPreview(text: string): string {
  const cleaned = text
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/<\/?[A-Za-z][^>]*>/g, '')
    .replace(/[*_`]/g, '')
    .replace(/^\s*(?:#{1,6}|>|[-+])\s*/u, '')
    .replace(/\s+/gu, ' ')
    .trim();
  const characters = Array.from(cleaned);
  return characters.length > previewCharacters
    ? `${characters.slice(0, previewCharacters).join('').trimEnd()}…`
    : cleaned;
}

// Preview only provider-exposed reasoning. A bounded tail avoids reparsing a
// growing reasoning document for every streamed update; the full text is kept.
export function thinkingStatus(reasoning: string): string {
  const tail = reasoning.slice(-reasoningWindow);
  const lines = tail.split(/\r?\n/);
  if (reasoning.length > reasoningWindow && lines.length > 1) lines.shift();
  let heading = '';
  let latestLine = '';
  let insideCode = false;
  for (const line of lines) {
    const trimmed = line.trim();
    if (/^(?:`{3,}|~{3,})/.test(trimmed)) {
      insideCode = !insideCode;
      continue;
    }
    if (!trimmed || insideCode) continue;
    const match = trimmed.match(/^(?:#{1,6}\s+(.+?)\s*#*|\*\*(.+?)\*\*|__(.+?)__)$/u);
    if (match) heading = match[1] ?? match[2] ?? match[3] ?? '';
    latestLine = trimmed;
  }
  if (heading) return plainPreview(heading);
  const sentences = latestLine.split(/(?<=[.!?。！？])\s+/u);
  return plainPreview(sentences.at(-1) ?? '');
}
