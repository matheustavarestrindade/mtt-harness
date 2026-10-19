import { comparePluginActivity, type PluginActivityEntry } from '../atoms/plugin-activity';
import type { HarnessEvent } from '../atoms/types';
import { ApiError, HarnessApi } from '../molecules/api/client';

const maximumEntries = 1000;

// Delivery commits precede model.call. Lifecycle/reconnect reads recover missed
// updates without another socket or per-chunk/full-chat polling.
export class SessionActivity {
  entries = $state<PluginActivityEntry[]>([]);
  error = $state('');
  loading = $state(false);
  nextCursor = $state('');
  private api: HarnessApi | null = null;
  private sessionID = '';
  private lifetime = new AbortController();
  private reading = false;
  private pending = false;
  private revision = -1;
  private historyRevision = -1;
  private expandedPages = false;
  private timer: ReturnType<typeof setTimeout> | undefined;
  constructor(readonly plugin = 'sidekick') {}

  open(api: HarnessApi, sessionID: string) {
    if (this.api === api && this.sessionID === sessionID) return;
    this.close();
    this.api = api;
    this.sessionID = sessionID;
    this.lifetime = new AbortController();
    void this.refresh();
  }
  close() {
    this.lifetime.abort();
    clearTimeout(this.timer);
    this.timer = undefined;
    this.api = null;
    this.sessionID = '';
    this.entries = [];
    this.error = '';
    this.nextCursor = '';
    this.loading = this.reading = this.pending = false;
    this.revision = this.historyRevision = -1;
    this.expandedPages = false;
  }
  observe(event: HarnessEvent) {
    if (event.SessionID !== this.sessionID) return;
    if (
      event.Name === 'plugin.activity' &&
      (event.Payload as Record<string, unknown>)?.plugin === this.plugin
    ) {
      this.scheduleRefresh();
      return;
    }
    if (
      [
        'model.call',
        'run.end',
        'run.error',
        'turn.end',
        'session.revert',
        'session.reverted',
      ].includes(event.Name)
    )
      this.scheduleRefresh();
  }
  scheduleRefresh() {
    if (!this.api || this.timer) return;
    this.timer = setTimeout(() => {
      this.timer = undefined;
      void this.refresh();
    }, 150);
  }
  refresh() {
    return this.read('');
  }
  more() {
    return this.nextCursor && this.entries.length < maximumEntries
      ? this.read(this.nextCursor)
      : Promise.resolve();
  }

  private async read(cursor: string) {
    const api = this.api,
      sessionID = this.sessionID,
      signal = this.lifetime.signal;
    if (!api || !sessionID || signal.aborted) return;
    if (this.reading) {
      if (!cursor) this.pending = true;
      return;
    }
    this.reading = this.loading = true;
    try {
      const page = await api.pluginActivity(sessionID, this.plugin, cursor, signal);
      if (
        signal.aborted ||
        page.SessionID !== sessionID ||
        page.Name !== this.plugin ||
        !Number.isSafeInteger(page.Revision) ||
        !Number.isSafeInteger(page.HistoryRevision) ||
        page.Revision < this.revision ||
        page.HistoryRevision < this.historyRevision
      )
        return;
      const incoming = page.Entries ?? [];
      if (incoming.some((entry) => entry.SessionID !== sessionID))
        throw new Error(`${this.plugin} activity scope is invalid.`);
      const changedHistory = page.HistoryRevision !== this.historyRevision;
      if (changedHistory) {
        this.entries = [];
        this.expandedPages = false;
      }
      const previous = new Map(this.entries.map((entry) => [entry.ID, entry]));
      const overlap = incoming.some((entry) => previous.has(entry.ID));
      if (!cursor && !overlap) this.expandedPages = false;
      for (const entry of incoming) previous.set(entry.ID, { ...entry, Notes: entry.Notes ?? [] });
      this.entries = [...previous.values()]
        .sort((first, second) => comparePluginActivity(second, first))
        .slice(0, maximumEntries);
      if (cursor) this.expandedPages = true;
      if (cursor || !this.expandedPages || changedHistory) this.nextCursor = page.NextCursor ?? '';
      if (this.entries.length >= maximumEntries) this.nextCursor = '';
      this.revision = page.Revision;
      this.historyRevision = page.HistoryRevision;
      this.error = '';
    } catch (failure) {
      if (signal.aborted) return;
      if (failure instanceof ApiError && failure.status === 404) {
        this.entries = [];
        this.nextCursor = '';
        this.error = '';
        return;
      }
      if (failure instanceof ApiError && failure.status === 409) {
        this.entries = [];
        this.nextCursor = '';
        this.expandedPages = false;
        if (cursor) this.pending = true;
        return;
      }
      this.error =
        failure instanceof Error ? failure.message : `Cannot load ${this.plugin} activity.`;
    } finally {
      if (!signal.aborted) {
        this.reading = this.loading = false;
        if (this.pending) {
          this.pending = false;
          this.scheduleRefresh();
        }
      }
    }
  }
}
