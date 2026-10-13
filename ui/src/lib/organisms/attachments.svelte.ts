import {
  attachmentByteLimit,
  attachmentCountLimit,
  attachmentFormat,
  attachmentIssue,
  attachmentSize,
  emptyContent,
  textAttachmentByteLimit,
  textAttachmentPrefix,
  type DraftAttachment,
} from '../atoms/attachments';
import type { Model } from '../atoms/types';

export class AttachmentDrafts {
  drafts = $state<Record<string, DraftAttachment[]>>({});
  errors = $state<Record<string, string>>({});
  private readers = new Map<string, FileReader>();

  addFiles(sessionID: string, files: File[], model: Model | undefined, protocol?: string) {
    if (!sessionID) return;
    this.errors[sessionID] = '';
    const failures: string[] = [];
    for (const file of files) {
      try {
        const existing = this.drafts[sessionID] ?? [];
        if (existing.length >= attachmentCountLimit)
          throw new Error(`A message can have up to ${attachmentCountLimit} attachments.`);
        if (existing.reduce((size, item) => size + item.size, 0) + file.size > attachmentByteLimit)
          throw new Error(`${file.name}: attachments can total up to 10 MB per message.`);
        const format = attachmentFormat(file);
        if (format.kind === 'text' && file.size > textAttachmentByteLimit)
          throw new Error(
            `${file.name}: text attachments can be up to ${attachmentSize(textAttachmentByteLimit)}.`,
          );
        if (file.size === 0) throw new Error(`${file.name}: the file is empty.`);
        const item: DraftAttachment = {
          id: crypto.randomUUID(),
          name: file.name,
          size: file.size,
          ...format,
          preview: '',
          status: 'ready',
          content: null,
          error: '',
        };
        const issue = attachmentIssue(item, model, protocol);
        if (issue) throw new Error(issue);
        if (['image', 'audio', 'video'].includes(item.kind))
          item.preview = URL.createObjectURL(file);
        item.status = 'reading';
        this.drafts[sessionID] = [...existing, item];
        this.readFile(sessionID, item, file);
      } catch (error) {
        failures.push(error instanceof Error ? error.message : 'Cannot attach the file.');
      }
    }
    this.errors[sessionID] = failures.join('\n');
  }

  private readFile(sessionID: string, item: DraftAttachment, file: File) {
    const reader = new FileReader();
    this.readers.set(item.id, reader);
    const update = (patch: Partial<DraftAttachment>) => {
      const existing = this.drafts[sessionID];
      if (!existing?.some((entry) => entry.id === item.id)) return;
      this.drafts[sessionID] = existing.map((entry) =>
        entry.id === item.id ? { ...entry, ...patch } : entry,
      );
    };
    reader.onload = () => {
      try {
        const content = { ...emptyContent(item.kind), Filename: item.name, MIME: item.mime };
        if (item.kind === 'text') {
          const text = new TextDecoder('utf-8', { fatal: true }).decode(
            reader.result as ArrayBuffer,
          );
          if (text.includes('\0')) throw new Error('The file contains binary data.');
          content.Text = textAttachmentPrefix(item.name) + text;
        } else {
          const data = String(reader.result);
          const separator = data.indexOf(',');
          if (separator < 0) throw new Error('The file encoding is invalid.');
          content.Data = data.slice(separator + 1);
        }
        update({ status: 'ready', content });
      } catch (error) {
        update({
          status: 'error',
          error: `${item.name}: ${error instanceof Error ? error.message : 'Cannot read file.'}`,
        });
      }
      this.readers.delete(item.id);
    };
    reader.onerror = () => {
      update({ status: 'error', error: `${item.name}: cannot read the file.` });
      this.readers.delete(item.id);
    };
    reader.onabort = () => this.readers.delete(item.id);
    if (item.kind === 'text') reader.readAsArrayBuffer(file);
    else reader.readAsDataURL(file);
  }

  remove(sessionID: string, id: string) {
    const item = this.drafts[sessionID]?.find((entry) => entry.id === id);
    this.readers.get(id)?.abort();
    this.readers.delete(id);
    if (item?.preview) URL.revokeObjectURL(item.preview);
    const remaining = (this.drafts[sessionID] ?? []).filter((entry) => entry.id !== id);
    if (remaining.length) this.drafts[sessionID] = remaining;
    else delete this.drafts[sessionID];
  }
  clearSession(sessionID: string) {
    for (const item of this.drafts[sessionID] ?? []) this.remove(sessionID, item.id);
    delete this.errors[sessionID];
  }
  clearAll() {
    for (const sessionID of Object.keys(this.drafts)) this.clearSession(sessionID);
    this.errors = {};
  }
}
