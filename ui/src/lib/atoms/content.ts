import type { Content } from './types';

export function safeContentURL(value: string, baseURL: string, image = false): string {
  try {
    if (!value.trim()) return '';
    const url = new URL(value, baseURL);
    const schemes = image ? ['http:', 'https:'] : ['http:', 'https:', 'mailto:'];
    return schemes.includes(url.protocol) && !url.username && !url.password ? url.href : '';
  } catch {
    return '';
  }
}

export function fileLanguage(filename: string): string {
  const extension = filename.toLowerCase().split('.').pop() ?? '';
  return (
    (
      {
        js: 'javascript',
        jsx: 'javascript',
        ts: 'typescript',
        tsx: 'typescript',
        py: 'python',
        sh: 'bash',
        yml: 'yaml',
        html: 'xml',
        svg: 'xml',
        md: 'markdown',
        go: 'go',
        json: 'json',
        css: 'css',
        sql: 'sql',
        yaml: 'yaml',
        csv: 'csv',
        tsv: 'tsv',
        diff: 'diff',
        patch: 'diff',
        txt: 'text',
      } as Record<string, string>
    )[extension] ?? 'text'
  );
}

export function contentFileText(content: Content): string | null {
  if (content.Text) return content.Text;
  if (!content.Data || content.Data.length > 2_000_000) return null;
  try {
    const bytes = Uint8Array.from(atob(content.Data), (character) => character.charCodeAt(0));
    return new TextDecoder('utf-8', { fatal: true }).decode(bytes);
  } catch {
    return null;
  }
}
