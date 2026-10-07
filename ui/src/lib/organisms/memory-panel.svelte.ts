import { ApiError, type HarnessApi } from '../molecules/api/client';
import type { MemoryConfiguration, MemoryMetrics, MemoryPluginState } from '../atoms/memory';

// Each panel lifetime owns its reads, writes, and polling. A workspace or
// connection change invalidates replies even when abort arrives too late.
export class MemoryPanelState {
  state = $state<MemoryPluginState | null>(null);
  metrics = $state<MemoryMetrics | null>(null);
  loading = $state(false);
  saving = $state(false);
  error = $state('');
  actionError = $state('');
  unavailable = $state(false);
  updatedAt = $state<Date | null>(null);
  private api: HarnessApi | null = null;
  private workspaceID = '';
  private generation = 0;
  private readRevision = 0;
  private read = new AbortController();
  private write = new AbortController();
  private timer: ReturnType<typeof setTimeout> | undefined;

  connect(api: HarnessApi | null, workspaceID: string) {
    this.disconnect();
    this.api = api;
    this.workspaceID = workspaceID;
    if (api && workspaceID) void this.refresh();
  }

  disconnect() {
    this.generation++;
    this.readRevision++;
    this.read.abort();
    this.write.abort();
    clearTimeout(this.timer);
    this.api = null;
    this.workspaceID = '';
    this.state = null;
    this.metrics = null;
    this.updatedAt = null;
    this.loading = false;
    this.saving = false;
    this.error = '';
    this.actionError = '';
    this.unavailable = false;
  }

  pause() {
    clearTimeout(this.timer);
    this.readRevision++;
    this.read.abort();
    this.loading = false;
  }

  async refresh() {
    const api = this.api;
    const workspaceID = this.workspaceID;
    if (!api || !workspaceID || this.saving || document.hidden) return;
    clearTimeout(this.timer);
    this.read.abort();
    const controller = new AbortController();
    this.read = controller;
    const generation = this.generation;
    const revision = ++this.readRevision;
    const current = () =>
      !controller.signal.aborted &&
      generation === this.generation &&
      revision === this.readRevision;
    this.loading = true;
    try {
      const [state, metrics] = await Promise.all([
        api.memoryState(workspaceID, controller.signal),
        api.memoryMetrics(workspaceID, controller.signal),
      ]);
      if (!current()) return;
      if (state.WorkspaceID !== workspaceID || metrics.WorkspaceID !== workspaceID)
        throw new Error('Memory response belongs to another workspace.');
      this.state = state;
      this.metrics = metrics;
      this.updatedAt = new Date();
      this.error = '';
      this.unavailable = false;
    } catch (failure) {
      if (!current()) return;
      this.unavailable = failure instanceof ApiError && failure.status === 404;
      this.error = this.unavailable
        ? 'Memory is not available on this harness.'
        : failure instanceof Error
          ? failure.message
          : 'Cannot load workspace memory.';
    } finally {
      if (current()) {
        this.loading = false;
        if (!this.unavailable)
          this.timer = setTimeout(() => void this.refresh(), this.state?.Pending ? 1000 : 5000);
      }
    }
  }

  async configure(patch: Partial<MemoryConfiguration>): Promise<boolean> {
    const api = this.api;
    const workspaceID = this.workspaceID;
    if (!api || !workspaceID || this.saving) return false;
    this.pause();
    this.write.abort();
    const controller = new AbortController();
    this.write = controller;
    const generation = this.generation;
    const current = () => !controller.signal.aborted && generation === this.generation;
    this.saving = true;
    this.actionError = '';
    try {
      const state = await api.configureMemory(workspaceID, patch, controller.signal);
      if (!current()) return false;
      if (state.WorkspaceID !== workspaceID)
        throw new Error('Memory response belongs to another workspace.');
      this.state = state;
      return true;
    } catch (failure) {
      if (current())
        this.actionError =
          failure instanceof Error ? failure.message : 'Cannot update memory settings.';
      return false;
    } finally {
      if (current()) {
        this.saving = false;
        void this.refresh();
      }
    }
  }
}
