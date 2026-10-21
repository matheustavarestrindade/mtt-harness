import { tick } from 'svelte';
import type { SessionHistory } from './session-history.svelte';

// Preserve a real visible DOM anchor when pages enter or leave the data window.
// Returning to the end discards older data, including mounted media/terminals.
export class HistoryScroll {
  element: HTMLElement | undefined;
  following = $state(true);
  private sessionID = '';
  private generation = 0;
  private programmaticTop: number | null = null;
  private revision: number | null = null;
  private paging = false;
  private interaction = 0;
  private lastTop = 0;
  private seekingLatest = false;
  private pageAnchor: { node: HTMLElement; offset: number } | null = null;
  constructor(readonly history: SessionHistory) {}
  dispose() {
    this.generation++;
    this.interaction++;
    this.element = undefined;
  }
  interact(direction: 'up' | 'down' = 'up') {
    this.interaction++;
    this.seekingLatest = false;
    this.programmaticTop = null;
    if (direction === 'down') {
      const element = this.element;
      if (
        element &&
        !this.history.hasNewer &&
        element.scrollHeight - element.scrollTop - element.clientHeight <= 2
      ) {
        this.following = true;
        this.history.following = true;
        this.scheduleFollow();
      }
      return;
    }
    this.following = false;
    this.history.following = false;
  }
  track(sessionID: string, _changes: number) {
    const changedSession = sessionID !== this.sessionID;
    if (
      sessionID !== this.sessionID ||
      (this.revision !== null && this.history.revision !== this.revision)
    ) {
      this.sessionID = sessionID;
      this.generation++;
      if (changedSession) this.following = true;
      this.paging = false;
      this.programmaticTop = null;
      this.lastTop = this.element?.scrollTop ?? 0;
      this.pageAnchor = null;
      this.seekingLatest = false;
      this.interaction++;
    }
    this.revision = this.history.revision;
    this.history.following = this.following;
    if (this.following && !this.history.loading && !this.paging) this.scheduleFollow();
  }
  onScroll() {
    const element = this.element;
    if (!element) return;
    const previousTop = this.lastTop;
    this.lastTop = element.scrollTop;
    if (this.programmaticTop !== null) {
      const expected = this.programmaticTop;
      this.programmaticTop = null;
      if (Math.abs(element.scrollTop - expected) < 1) return;
    }
    if (this.seekingLatest) return;
    if (this.paging) {
      if (this.pageAnchor?.node.isConnected)
        this.pageAnchor.offset =
          this.pageAnchor.node.getBoundingClientRect().top - element.getBoundingClientRect().top;
      return;
    }
    const bottom = element.scrollHeight - element.scrollTop - element.clientHeight;
    const movingUp = element.scrollTop < previousTop - 0.5,
      movingDown = element.scrollTop > previousTop + 0.5;
    if (movingUp && bottom > 2) this.interact();
    // A small upward gesture must not re-enable following inside a bottom band.
    // Resume only on a downward return to the actual end, or the Latest action.
    if (!this.following && movingDown && bottom <= 2 && !this.history.hasNewer)
      this.following = true;
    this.history.following = this.following;
    if (this.following) {
      this.history.compactTail();
      this.scheduleFollow();
      return;
    }
    if (element.scrollTop < 200 && this.history.hasOlder) void this.older();
    else if (movingDown && bottom < 200 && this.history.hasNewer) void this.newer();
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
    this.pageAnchor = anchor;
    this.following = false;
    this.history.following = false;
    try {
      await this.history[direction]();
      await tick();
      if (generation !== this.generation || !this.element) return;
      const currentAnchor = this.pageAnchor;
      if (currentAnchor?.node.isConnected && this.element.contains(currentAnchor.node)) {
        this.element.scrollTop +=
          currentAnchor.node.getBoundingClientRect().top -
          this.element.getBoundingClientRect().top -
          currentAnchor.offset;
      } else if (direction === 'older') this.element.scrollTop = 0;
      this.programmaticTop = this.element.scrollTop;
      this.lastTop = this.element.scrollTop;
    } finally {
      if (generation === this.generation) {
        this.paging = false;
        this.pageAnchor = null;
      }
    }
  }
  older() {
    return this.page('older');
  }
  newer() {
    return this.page('newer');
  }
  async latest() {
    const generation = this.generation,
      interaction = ++this.interaction;
    this.following = true;
    this.history.following = true;
    this.seekingLatest = true;
    // Apply the explicit Latest action before asynchronous paging. Otherwise a
    // remaining native keyboard-scroll event can turn following off during I/O.
    this.toBottom();
    try {
      await this.history.loadLatest();
      await tick();
      if (generation !== this.generation || interaction !== this.interaction) return;
      this.toBottom();
      // Focus changes and the replacement history can emit a final native
      // scroll during layout. Real wheel/key/touch input cancels this seek.
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      if (generation === this.generation && interaction === this.interaction) this.toBottom();
    } finally {
      if (generation === this.generation && interaction === this.interaction)
        this.seekingLatest = false;
    }
  }
  private scheduleFollow() {
    const generation = this.generation,
      interaction = this.interaction;
    void tick().then(() => {
      if (generation === this.generation && interaction === this.interaction) this.toBottom();
    });
  }
  private toBottom() {
    if (!this.element || !this.following) return;
    this.element.scrollTop = this.element.scrollHeight;
    this.programmaticTop = this.element.scrollTop;
    this.lastTop = this.element.scrollTop;
  }
}
