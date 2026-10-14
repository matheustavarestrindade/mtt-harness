import { SvelteMap } from 'svelte/reactivity';
import { processFromEvent, type ProcessRecord } from '../atoms/processes';
import type { HarnessEvent } from '../atoms/types';
import { HarnessApi } from '../molecules/api/client';

// This directory owns metadata only. Output sockets belong to open terminal
// views, so collapsed tools do not retain a separate stream or render logs.
export class SessionProcesses {
  records = new SvelteMap<string, ProcessRecord>();
  error = $state('');
  api: HarnessApi | null = null;
  private sessionID = '';
  private lifetime = new AbortController();
  private refreshTimer: ReturnType<typeof setTimeout> | undefined;
  private reading = false;
  private pending = false;
  private revision = 0;
  private revisions = new Map<string, number>();

  open(api: HarnessApi, sessionID: string) {
    if (this.api === api && this.sessionID === sessionID) return;
    this.close();
    this.lifetime = new AbortController();
    this.api = api;
    this.sessionID = sessionID;
    void this.refresh();
  }
  close() {
    this.lifetime.abort();
    clearTimeout(this.refreshTimer);
    this.refreshTimer = undefined;
    this.api = null;
    this.sessionID = '';
    this.records.clear();
    this.revisions.clear();
    this.reading = this.pending = false;
    this.error = '';
  }
  forCall(messageID: string, callID: string): ProcessRecord | null {
    return (
      [...this.records.values()].find(
        (record) => record.Spec.ToolMessageID === messageID && record.Spec.ToolCallID === callID,
      ) ?? null
    );
  }
  observe(event: HarnessEvent) {
    if (
      event.SessionID !== this.sessionID ||
      !['process.start', 'process.exit'].includes(event.Name)
    )
      return;
    const record = processFromEvent(event);
    if (record) {
      this.revisions.set(record.ID, ++this.revision);
      const previous = this.records.get(record.ID);
      if (previous?.Status === 'stopped' && record.Status !== 'stopped') return;
      this.records.set(record.ID, record);
      return;
    }
    this.scheduleRefresh();
  }
  scheduleRefresh() {
    if (!this.api || this.refreshTimer) return;
    this.refreshTimer = setTimeout(() => {
      this.refreshTimer = undefined;
      void this.refresh();
    }, 150);
  }
  async refresh() {
    const api = this.api,
      sessionID = this.sessionID,
      signal = this.lifetime.signal;
    if (!api || !sessionID || signal.aborted) return;
    if (this.reading) {
      this.pending = true;
      return;
    }
    this.reading = true;
    const revision = this.revision;
    try {
      const records = await api.processes(sessionID, signal);
      if (signal.aborted) return;
      for (const record of records) {
        if (record.SessionID !== sessionID || (this.revisions.get(record.ID) ?? 0) > revision)
          continue;
        if (this.records.get(record.ID)?.Status === 'stopped' && record.Status !== 'stopped')
          continue;
        this.records.set(record.ID, record);
      }
      this.error = '';
    } catch (failure) {
      if (!signal.aborted)
        this.error = failure instanceof Error ? failure.message : 'Cannot load processes.';
    } finally {
      if (!signal.aborted) {
        this.reading = false;
        if (this.pending) {
          this.pending = false;
          this.scheduleRefresh();
        }
      }
    }
  }
}
