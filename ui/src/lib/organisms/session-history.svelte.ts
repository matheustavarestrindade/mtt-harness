import { SvelteMap } from 'svelte/reactivity';
import type { Message } from '../atoms/types';
import type { ProcessRecord } from '../atoms/processes';
import type { HistoryPage, HistoryQuery, ResponseUsageSnapshot } from '../atoms/session-view';
import { reconcileMessageHistory } from '../atoms/message-history';
import { ApiError, HarnessApi } from '../molecules/api/client';

export const historyPageSize = 50;
export const historyWindowLimit = 150;
export const historyByteLimit = 8 * 1024 * 1024;

// One viewer owns a sliding saved-message window. Eviction removes data as well
// as DOM nodes; the latest usage snapshot remains independent of scroll position.
export class SessionHistory {
  messages = $state<Message[]>([]);
  context = $state<Message[]>([]);
  latest = $state<ResponseUsageSnapshot | null>(null);
  models = new SvelteMap<string, string>();
  revision = $state<number | null>(null);
  headSeq = $state(0);
  hasOlder = $state(false);
  hasNewer = $state(false);
  following = $state(true);
  tailAttached = $state(true);
  loading = $state(false);
  direction = $state<'latest' | 'older' | 'newer' | 'refresh' | null>(null);
  error = $state('');
  version = $state(0);
  private api: HarnessApi | null = null;
  private sessionID = '';
  private request = new AbortController();
  private requestVersion = 0;
  private refreshPending = false;
  private sizes = new Map<string, number>();
  constructor(
    private onWindow: (records: ProcessRecord[], messages: Message[]) => void = () => {},
  ) {}
  get window() {
    const identifiers = new Set(this.messages.map((message) => message.ID));
    return [...this.context.filter((message) => !identifiers.has(message.ID)), ...this.messages];
  }
  get bytes() {
    return this.window.reduce((total, message) => total + this.sizeOf(message), 0);
  }
  get lastSeq() {
    return this.messages.at(-1)?.Seq ?? 0;
  }
  private sizeOf(message: Message) {
    return (
      this.sizes.get(message.ID) ??
      256 +
        (message.Reasoning?.length ?? 0) +
        JSON.stringify(message.Content ?? []).length +
        JSON.stringify(message.ToolCalls ?? []).length
    );
  }
  open(api: HarnessApi, sessionID: string) {
    if (this.api === api && this.sessionID === sessionID) return;
    this.close();
    this.api = api;
    this.sessionID = sessionID;
    this.request = new AbortController();
  }
  close() {
    this.request.abort();
    this.requestVersion++;
    this.api = null;
    this.sessionID = '';
    this.messages = [];
    this.context = [];
    this.models.clear();
    this.sizes.clear();
    this.latest = null;
    this.revision = null;
    this.headSeq = 0;
    this.hasOlder = this.hasNewer = false;
    this.following = this.tailAttached = true;
    this.loading = false;
    this.direction = null;
    this.error = '';
    this.refreshPending = false;
    this.version++;
  }
  observeMetadata(page: HistoryPage) {
    if (page.SessionID !== this.sessionID) return;
    if (this.revision !== null && page.Revision < this.revision) return;
    if (this.revision !== null && page.Revision !== this.revision) {
      this.request.abort();
      this.requestVersion++;
      this.loading = false;
      this.messages = [];
      this.context = [];
      this.models.clear();
      this.sizes.clear();
      this.headSeq = 0;
      this.latest = null;
      this.following = true;
      this.revision = page.Revision;
      this.updateMetadata(page);
      void this.loadLatest();
      return;
    }
    this.updateMetadata(page);
    this.hasNewer = this.lastSeq < this.headSeq;
    if ((this.following || this.tailAttached) && this.hasNewer) void this.refresh();
  }
  private updateMetadata(page: HistoryPage) {
    this.revision = page.Revision;
    this.headSeq = Math.max(this.headSeq, page.HeadSeq);
    const latest = page.LatestResponse;
    if (latest && (!this.latest || latest.Seq >= this.latest.Seq)) {
      this.latest = latest;
      if (latest.Model) this.models.set(latest.MessageID, latest.Model);
    }
    if (!page.HeadSeq) this.latest = null;
  }
  async loadLatest() {
    this.following = true;
    if (this.loading && this.direction !== 'latest') {
      this.request.abort();
      this.requestVersion++;
      this.loading = false;
    }
    await this.fetch('latest', {});
  }

  async recover() {
    if (this.api && this.tailAttached && this.messages.length && !this.loading)
      await this.fetch('refresh', {});
  }
  async older() {
    if (!this.hasOlder || this.loading || !this.messages.length) return;
    this.following = false;
    await this.fetch('older', {
      before: this.messages[0].Seq,
      revision: this.revision ?? undefined,
    });
  }
  async newer() {
    if (!this.hasNewer || this.loading || !this.messages.length) return;
    await this.fetch('newer', { after: this.lastSeq, revision: this.revision ?? undefined });
  }
  compactTail() {
    if (this.loading || this.hasNewer || this.messages.length <= historyPageSize) return;
    this.apply(
      {
        SessionID: this.sessionID,
        Revision: this.revision ?? 0,
        Messages: this.messages.slice(-historyPageSize),
        Context: this.context,
        Models: Object.fromEntries(this.models),
        Sizes: Object.fromEntries(this.sizes),
        FirstSeq: 0,
        LastSeq: 0,
        HeadSeq: this.headSeq,
        HasOlder: true,
        HasNewer: false,
        LatestResponse: this.latest,
        Processes: [],
      },
      'latest',
    );
  }
  async refresh() {
    if (!this.api || (!this.following && !this.tailAttached)) return;
    if (this.loading) {
      this.refreshPending = true;
      return;
    }
    if (!this.messages.length) {
      await this.loadLatest();
      return;
    }
    for (
      let page = 0;
      page < (this.following ? 3 : 1) &&
      (this.following || this.tailAttached) &&
      this.lastSeq < this.headSeq;
      page++
    ) {
      await this.fetch('refresh', { after: this.lastSeq, revision: this.revision ?? undefined });
      if (this.error) return;
    }
    if (this.following && this.lastSeq < this.headSeq) await this.loadLatest();
  }
  private async fetch(direction: NonNullable<SessionHistory['direction']>, query: HistoryQuery) {
    const api = this.api,
      sessionID = this.sessionID;
    if (!api || !sessionID) return;
    if (this.loading) {
      if (direction === 'refresh') this.refreshPending = true;
      return;
    }
    this.request.abort();
    this.request = new AbortController();
    const signal = this.request.signal;
    const version = ++this.requestVersion;
    this.loading = true;
    this.direction = direction;
    try {
      const page = await api.history(sessionID, { limit: historyPageSize, ...query }, signal);
      if (
        signal.aborted ||
        version !== this.requestVersion ||
        this.api !== api ||
        page.SessionID !== this.sessionID
      )
        return;
      if (this.revision !== null && page.Revision < this.revision) return;
      if (this.revision !== null && page.Revision !== this.revision && direction !== 'latest') {
        this.loading = false;
        this.observeMetadata(page);
        return;
      }
      this.apply(page, direction);
      this.error = '';
    } catch (failure) {
      if (signal.aborted || version !== this.requestVersion) return;
      if (failure instanceof ApiError && failure.status === 409) {
        this.loading = false;
        this.messages = [];
        this.context = [];
        this.models.clear();
        this.sizes.clear();
        this.headSeq = 0;
        this.revision = null;
        await this.loadLatest();
        return;
      }
      this.error = failure instanceof Error ? failure.message : 'Cannot load conversation history.';
    } finally {
      if (!signal.aborted && version === this.requestVersion) {
        this.loading = false;
        this.direction = null;
        if (this.refreshPending) {
          this.refreshPending = false;
          void this.refresh();
        }
      }
    }
  }
  private apply(page: HistoryPage, direction: NonNullable<SessionHistory['direction']>) {
    const previous = this.messages;
    const candidates = [...this.context, ...previous, ...page.Context];
    for (const [identifier, size] of Object.entries(page.Sizes)) this.sizes.set(identifier, size);
    for (const [identifier, model] of Object.entries(page.Models))
      this.models.set(identifier, model);
    const collected = new Map<string, Message>();
    if (direction !== 'latest') for (const message of previous) collected.set(message.ID, message);
    let retainedBytes = previous.reduce((total, message) => total + this.sizeOf(message), 0);
    for (const message of page.Messages) {
      if (direction === 'refresh' && !this.following && !collected.has(message.ID)) {
        const size = this.sizeOf(message);
        if (collected.size >= historyWindowLimit || retainedBytes + size > historyByteLimit) break;
        retainedBytes += size;
      }
      collected.set(message.ID, message);
    }
    let messages = [...collected.values()].sort((first, second) => first.Seq - second.Seq);
    let bytes = messages.reduce((total, message) => total + this.sizeOf(message), 0),
      trimmedHead = false,
      trimmedTail = false;
    while (
      messages.length > 1 &&
      (messages.length > historyWindowLimit || bytes > historyByteLimit)
    ) {
      const removed = direction === 'older' ? messages.pop()! : messages.shift()!;
      bytes -= this.sizeOf(removed);
      if (direction === 'older') trimmedTail = true;
      else trimmedHead = true;
    }
    this.messages = reconcileMessageHistory(previous, messages);
    this.updateMetadata(page);
    if (direction === 'latest' || direction === 'older') this.hasOlder = page.HasOlder;
    if (trimmedHead) this.hasOlder = true;
    if (direction === 'older' && !page.Messages.length) this.hasOlder = false;
    this.hasNewer = trimmedTail || this.lastSeq < this.headSeq;
    this.tailAttached = !this.hasNewer;
    this.context = [];
    if (messages[0]?.Role === 'tool') {
      const first = messages[0];
      const owner = candidates
        .filter(
          (message) =>
            message.Role === 'assistant' &&
            message.Seq < first.Seq &&
            message.ToolCalls?.some((call) => call.ID === first.ToolCallID),
        )
        .sort((first, second) => second.Seq - first.Seq)[0];
      if (owner) {
        const needed = new Set<string>();
        for (const message of messages) {
          if (message.Role !== 'tool') break;
          needed.add(message.ToolCallID);
        }
        this.context = [
          {
            ...owner,
            Content: [],
            Reasoning: '',
            Usage: null,
            ToolCalls: owner.ToolCalls?.filter((call) => needed.has(call.ID)) ?? [],
          },
        ];
        this.sizes.set(owner.ID, 256 + JSON.stringify(this.context[0].ToolCalls).length);
      }
    }
    const retained = new Set([...this.messages, ...this.context].map((message) => message.ID));
    for (const identifier of this.sizes.keys())
      if (!retained.has(identifier)) this.sizes.delete(identifier);
    for (const identifier of this.models.keys())
      if (!retained.has(identifier) && identifier !== this.latest?.MessageID)
        this.models.delete(identifier);
    this.version++;
    this.onWindow(page.Processes, this.window);
  }
}
