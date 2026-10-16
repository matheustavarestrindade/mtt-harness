import { tick } from 'svelte';
import type { SessionHistory } from './session-history.svelte';

// Preserve a real visible DOM anchor when pages enter or leave the data window.
// A near-bottom transition discards older data, including mounted media/terminals.
export class HistoryScroll {
  element: HTMLElement | undefined;
  following = $state(true);
  private sessionID = '';
  private generation = 0;
  private programmaticTop: number | null = null;
  private revision: number | null = null;
  private paging = false;
  constructor(readonly history: SessionHistory) {}
  dispose() {
    this.generation++;
    this.element = undefined;
  }
  interact() {
    this.following = false;
    this.history.following = false;
    this.programmaticTop = null;
  }
  track(sessionID: string, _changes: number) {
    if (
      sessionID !== this.sessionID ||
      (this.revision !== null && this.history.revision !== this.revision)
    ) {
      this.sessionID = sessionID;
      this.generation++;
      this.following = true;
      this.history.following = true;
      this.paging = false;
      this.programmaticTop = null;
    }
    this.revision = this.history.revision;
    if (this.following && !this.history.loading && !this.paging)
      void tick().then(() => this.toBottom());
  }
  onScroll() {
    const element = this.element;
    if (!element || this.paging) return;
    if (this.programmaticTop !== null) {
      const expected = this.programmaticTop;
      this.programmaticTop = null;
      if (Math.abs(element.scrollTop - expected) < 1) return;
    }
    const bottom = element.scrollHeight - element.scrollTop - element.clientHeight;
    this.following = !this.history.hasNewer && bottom < 100;
    this.history.following = this.following;
    if (this.following) {
      this.history.compactTail();
      void tick().then(() => this.toBottom());
      return;
    }
    if (element.scrollTop < 200 && this.history.hasOlder) void this.older();
    else if (bottom < 200 && this.history.hasNewer) void this.newer();
  }
  private anchor() {
    const element = this.element;
    if (!element) return null;
    const viewport = element.getBoundingClientRect();
    const visible = [...element.querySelectorAll<HTMLElement>('[data-history-id],[data-call-id]')]
      .map((node) => ({ node, rectangle: node.getBoundingClientRect() }))
      .filter(
        (entry) =>
          entry.rectangle.bottom > viewport.top + 4 && entry.rectangle.top < viewport.bottom,
      );
    const anchor =
      visible
        .filter((entry) => entry.rectangle.top <= viewport.top + 4)
        .sort((first, second) => second.rectangle.top - first.rectangle.top)[0] ?? visible[0];
    return anchor ? { node: anchor.node, offset: anchor.rectangle.top - viewport.top } : null;
  }
  private async page(direction: 'older' | 'newer') {
    if (this.paging || this.history.loading) return;
    const generation = this.generation,
      anchor = this.anchor();
    this.paging = true;
    this.following = false;
    this.history.following = false;
    try {
      await this.history[direction]();
      await tick();
      if (generation !== this.generation || !this.element) return;
      if (anchor?.node.isConnected && this.element.contains(anchor.node)) {
        this.element.scrollTop +=
          anchor.node.getBoundingClientRect().top -
          this.element.getBoundingClientRect().top -
          anchor.offset;
      } else if (direction === 'older') this.element.scrollTop = 0;
      this.programmaticTop = this.element.scrollTop;
    } finally {
      if (generation === this.generation) this.paging = false;
    }
  }
  older() {
    return this.page('older');
  }
  newer() {
    return this.page('newer');
  }
  async latest() {
    await this.history.loadLatest();
    this.following = true;
    this.history.following = true;
    await tick();
    this.toBottom();
  }
  private toBottom() {
    if (!this.element || !this.following) return;
    this.element.scrollTop = this.element.scrollHeight;
    this.programmaticTop = this.element.scrollTop;
  }
}
