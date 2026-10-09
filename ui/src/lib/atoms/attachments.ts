import type { Content, Model } from './types';

export const attachmentByteLimit = 10 * 1024 * 1024;
export const textAttachmentByteLimit = 512 * 1024;
export const attachmentCountLimit = 8;
export const messageBodyByteLimit = 16 * 1024 * 1024;
export type AttachmentKind = Content['Type'];
export interface DraftAttachment {
  id: string;
  name: string;
  size: number;
  mime: string;
  kind: AttachmentKind;
  preview: string;
  status: 'reading' | 'ready' | 'error';
  content: Content | null;
  error: string;
}

const textExtensions = [
  'txt',
  'md',
  'markdown',
  'csv',
  'tsv',
  'json',
  'jsonl',
  'yaml',
  'yml',
  'toml',
  'xml',
  'html',
  'css',
  'js',
  'jsx',
  'ts',
  'tsx',
  'svelte',
  'vue',
  'go',
  'py',
  'rs',
  'java',
  'c',
  'h',
  'cpp',
  'hpp',
  'cs',
  'rb',
  'php',
  'sh',
  'bash',
  'sql',
  'log',
  'ini',
  'cfg',
  'conf',
  'diff',
  'patch',
  'svg',
];
const imageFormats: Record<string, string> = {
  png: 'image/png',
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  webp: 'image/webp',
  gif: 'image/gif',
};
const audioFormats: Record<string, string> = { mp3: 'audio/mpeg', wav: 'audio/wav' };
const videoFormats: Record<string, string> = {
  mp4: 'video/mp4',
  mpeg: 'video/mpeg',
  mpg: 'video/mpeg',
  mov: 'video/quicktime',
  webm: 'video/webm',
};
const documentFormats: Record<string, string> = {
  pdf: 'application/pdf',
  doc: 'application/msword',
  docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  xls: 'application/vnd.ms-excel',
  xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  ppt: 'application/vnd.ms-powerpoint',
  pptx: 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
  odt: 'application/vnd.oasis.opendocument.text',
  rtf: 'application/rtf',
};

export function attachmentCapabilities(model: Model | undefined, protocol?: string) {
  const input = model?.Input?.length ? model.Input : ['text'];
  return {
    text: input.includes('text'),
    image: input.includes('image'),
    video: input.includes('video'),
    file: input.includes('file'),
    // The current Responses adapter does not implement audio input.
    audio: input.includes('audio') && protocol !== undefined && protocol !== 'responses',
  };
}

export function attachmentAccept(model: Model | undefined, protocol?: string): string {
  const allowed = attachmentCapabilities(model, protocol);
  return [
    ...(allowed.text ? textExtensions.map((extension) => '.' + extension) : []),
    ...(allowed.image ? Object.values(imageFormats) : []),
    ...(allowed.audio
      ? ['audio/mpeg', 'audio/mp3', 'audio/wav', 'audio/x-wav', '.mp3', '.wav']
      : []),
    ...(allowed.video
      ? [...Object.values(videoFormats), '.mp4', '.mpeg', '.mpg', '.mov', '.webm']
      : []),
    ...(allowed.file ? Object.keys(documentFormats).map((extension) => '.' + extension) : []),
  ].join(',');
}

export function attachmentFormat(file: Pick<File, 'name' | 'type'>): {
  kind: AttachmentKind;
  mime: string;
} {
  const extension = file.name.toLowerCase().split('.').pop() ?? '';
  const declared = file.type.toLowerCase().split(';')[0];
  const mime = declared === 'application/octet-stream' ? '' : declared;
  if (mime === 'image/svg+xml' || extension === 'svg')
    return { kind: 'text', mime: 'image/svg+xml' };
  if (mime.startsWith('image/') || imageFormats[extension]) {
    const selected = mime || imageFormats[extension];
    if (!Object.values(imageFormats).includes(selected))
      throw new Error(`${file.name}: supported image formats are PNG, JPEG, WebP and GIF.`);
    return { kind: 'image', mime: selected };
  }
  if (mime.startsWith('audio/') || audioFormats[extension]) {
    const selected =
      ({ 'audio/x-wav': 'audio/wav', 'audio/mp3': 'audio/mpeg' } as Record<string, string>)[mime] ||
      mime ||
      audioFormats[extension];
    if (!Object.values(audioFormats).includes(selected))
      throw new Error(`${file.name}: supported audio formats are WAV and MP3.`);
    return { kind: 'audio', mime: selected };
  }
  if (mime.startsWith('video/') || videoFormats[extension]) {
    const selected = mime || videoFormats[extension];
    if (![...Object.values(videoFormats), 'video/mov'].includes(selected))
      throw new Error(`${file.name}: supported video formats are MP4, MPEG, MOV and WebM.`);
    return { kind: 'video', mime: selected };
  }
  if (
    mime.startsWith('text/') ||
    [
      'application/json',
      'application/x-ndjson',
      'application/xml',
      'application/yaml',
      'application/toml',
      'application/javascript',
    ].includes(mime) ||
    textExtensions.includes(extension) ||
    /^(dockerfile|makefile|license|readme)$/i.test(file.name)
  )
    return { kind: 'text', mime: mime || 'text/plain' };
  if (documentFormats[extension] || mime === 'application/pdf')
    return { kind: 'file', mime: mime || documentFormats[extension] };
  throw new Error(`${file.name}: this file format is not supported by the attachment uploader.`);
}

export function attachmentIssue(
  item: Pick<DraftAttachment, 'kind' | 'name' | 'status' | 'error'>,
  model: Model | undefined,
  protocol?: string,
): string {
  if (item.error) return item.error;
  if (!attachmentCapabilities(model, protocol)[item.kind])
    return `${item.name}: the selected model or provider does not accept ${item.kind} input.`;
  if (item.status === 'reading') return `${item.name}: reading file…`;
  return '';
}

export function textAttachmentPrefix(filename: string): string {
  return `Attached file ${JSON.stringify(filename)}:\n`;
}
export function attachmentSize(bytes: number): string {
  return bytes >= 1024 * 1024
    ? `${(bytes / 1024 / 1024).toFixed(1)} MB`
    : `${Math.max(1, Math.ceil(bytes / 1024))} KB`;
}
export function emptyContent(type: Content['Type']): Content {
  return { Type: type, Text: '', Data: null, MIME: '', URL: '', Filename: '', AudioID: '' };
}

export function composeMessageContent(
  text: string,
  attachments: DraftAttachment[],
  model: Model | undefined,
  protocol?: string,
): string | Content[] {
  if (!attachments.length) {
    if (
      new TextEncoder().encode(JSON.stringify({ content: text })).byteLength > messageBodyByteLimit
    )
      throw new Error('The message exceeds the API limit of 16 MB.');
    return text;
  }
  const content: Content[] = [];
  if (text.trim()) content.push({ ...emptyContent('text'), Text: text });
  for (const attachment of attachments) {
    const issue = attachmentIssue(attachment, model, protocol);
    if (issue || !attachment.content)
      throw new Error(issue || `${attachment.name}: file is not ready.`);
    content.push(attachment.content);
  }
  if (new TextEncoder().encode(JSON.stringify({ content })).byteLength > messageBodyByteLimit)
    throw new Error(
      'The message exceeds the API limit of 16 MB, including encoded attachments and text.',
    );
  return content;
}
