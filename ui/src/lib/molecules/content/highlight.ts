import highlight from 'highlight.js/lib/core';
import DOMPurify from 'dompurify';
import bash from 'highlight.js/lib/languages/bash';
import css from 'highlight.js/lib/languages/css';
import diff from 'highlight.js/lib/languages/diff';
import go from 'highlight.js/lib/languages/go';
import javascript from 'highlight.js/lib/languages/javascript';
import json from 'highlight.js/lib/languages/json';
import markdown from 'highlight.js/lib/languages/markdown';
import python from 'highlight.js/lib/languages/python';
import sql from 'highlight.js/lib/languages/sql';
import typescript from 'highlight.js/lib/languages/typescript';
import xml from 'highlight.js/lib/languages/xml';
import yaml from 'highlight.js/lib/languages/yaml';

for (const [name, grammar] of Object.entries({
  bash,
  css,
  diff,
  go,
  javascript,
  json,
  markdown,
  python,
  sql,
  typescript,
  xml,
  yaml,
})) {
  highlight.registerLanguage(name, grammar);
}

export function highlightCode(text: string, language: string): string | null {
  // No automatic language detection: bound work and avoid applying every grammar to tool logs.
  if (text.length > 30_000 || !highlight.getLanguage(language)) return null;
  return DOMPurify.sanitize(highlight.highlight(text, { language, ignoreIllegals: true }).value, {
    ALLOWED_TAGS: ['span'],
    ALLOWED_ATTR: ['class'],
    ALLOW_DATA_ATTR: false,
  });
}
