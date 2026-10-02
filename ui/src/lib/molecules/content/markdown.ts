import { Marked, Renderer, type Token, type Tokens } from 'marked';
import DOMPurify from 'dompurify';
import { safeContentURL } from '../../atoms/content';

export type MarkdownBlock =
  { type: 'html'; html: string } | { type: 'code'; text: string; language: string };
export const markdownPreviewLimit = 100_000;
const escapeHTML = (value: string) =>
  value.replace(
    /[&<>"']/g,
    (character) =>
      ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[character]!,
  );
const renderer = new Renderer();
// HTML from the model is displayed as source; generated Markdown markup is sanitized below.
renderer.html = ({ text }) => escapeHTML(text);
renderer.link = function ({ href, tokens }: Tokens.Link) {
  const label = this.parser.parseInline(tokens);
  const url = safeContentURL(href, window.location.href);
  return url
    ? `<a href="${escapeHTML(url)}" target="_blank" rel="noopener noreferrer">${label}</a>`
    : label;
};
renderer.image = ({ href, text }: Tokens.Image) => {
  const url = safeContentURL(href, window.location.href, true);
  return url
    ? `<img src="${escapeHTML(url)}" alt="${escapeHTML(text)}" loading="lazy">`
    : escapeHTML(text);
};
const renderTable = renderer.table;
renderer.table = function (token) {
  return `<div class="markdown-table" tabindex="0" role="region" aria-label="Markdown table">${renderTable.call(this, token)}</div>`;
};
const markdown = new Marked({ gfm: true, breaks: true, async: false, renderer });

export function parseMarkdown(text: string): MarkdownBlock[] {
  const blocks: MarkdownBlock[] = [];
  let prose: Token[] = [];
  const flushProse = () => {
    if (!prose.length) return;
    const html = DOMPurify.sanitize(markdown.parser(prose), {
      ALLOWED_TAGS: [
        'p',
        'br',
        'h1',
        'h2',
        'h3',
        'h4',
        'h5',
        'h6',
        'strong',
        'em',
        'del',
        'a',
        'img',
        'ul',
        'ol',
        'li',
        'blockquote',
        'pre',
        'code',
        'hr',
        'table',
        'thead',
        'tbody',
        'tr',
        'th',
        'td',
        'div',
        'input',
      ],
      ALLOWED_ATTR: [
        'href',
        'src',
        'alt',
        'loading',
        'target',
        'rel',
        'class',
        'align',
        'start',
        'type',
        'checked',
        'disabled',
        'tabindex',
        'role',
        'aria-label',
      ],
      ALLOW_DATA_ATTR: false,
    });
    if (html.trim()) blocks.push({ type: 'html', html });
    prose = [];
  };
  for (const token of markdown.lexer(text.slice(0, markdownPreviewLimit))) {
    if (token.type !== 'code') {
      prose.push(token);
      continue;
    }
    flushProse();
    const code = token as Tokens.Code;
    blocks.push({ type: 'code', text: code.text, language: (code.lang ?? 'text').split(/\s/)[0] });
  }
  flushProse();
  return blocks;
}
