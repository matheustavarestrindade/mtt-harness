<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { ArrowDown, Copy, Download } from 'lucide-svelte';
  import { toast } from 'svelte-sonner';
  import type { Terminal } from '@xterm/xterm';
  import type { TerminalSnapshot } from '../../atoms/processes';
  import { Button } from '../../atoms/ui/button';
  import { copyToClipboard } from '../clipboard';
  let {
    text = '',
    snapshot,
    label = 'Terminal output',
  }: { text?: string; snapshot?: TerminalSnapshot; label?: string } = $props();
  let host: HTMLDivElement;
  let ready = $state(false);
  let failure = $state('');
  let following = $state(true);
  let terminal: Terminal | undefined;
  let disposed = false;
  let writing = false;
  let generation = -1;
  let lastID = 0;
  let previousText: string | undefined;
  function renderLatest() {
    if (!terminal || writing || disposed) return;
    let output = '',
      targetID = lastID;
    if (snapshot) {
      if (generation !== snapshot.generation) {
        terminal.reset();
        generation = snapshot.generation;
        lastID = 0;
      }
      const fresh = snapshot.chunks.filter((chunk) => chunk.id > lastID);
      if (!fresh.length) return;
      if (lastID && fresh[0].id !== lastID + 1) {
        terminal.reset();
        output = snapshot.chunks.map((chunk) => chunk.text).join('');
      } else output = fresh.map((chunk) => chunk.text).join('');
      targetID = fresh[fresh.length - 1].id;
    } else {
      if (previousText === text) return;
      terminal.reset();
      previousText = text;
      output = text.slice(-524_288);
    }
    const follow = following;
    writing = true;
    terminal.write(output, () => {
      writing = false;
      if (disposed || !terminal) return;
      lastID = targetID;
      if (follow && following) terminal.scrollToBottom();
      renderLatest();
    });
  }
  $effect(() => {
    ready;
    snapshot;
    text;
    untrack(renderLatest);
  });
  onMount(() => {
    let resize: ResizeObserver | undefined;
    void Promise.all([import('@xterm/xterm'), import('@xterm/addon-fit')])
      .then(([{ Terminal }, { FitAddon }]) => {
        if (disposed) return;
        const colors = getComputedStyle(host);
        const background = colors.getPropertyValue('--terminal-background').trim();
        const foreground = colors.getPropertyValue('--terminal-foreground').trim();
        terminal = new Terminal({
          disableStdin: true,
          convertEol: true,
          cursorBlink: false,
          cursorInactiveStyle: 'none',
          scrollback: 2000,
          fontSize: 12,
          lineHeight: 1.25,
          fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
          screenReaderMode: true,
          drawBoldTextInBrightColors: false,
          logLevel: 'off',
          linkHandler: { activate() {} },
          theme: { background, foreground, cursor: background },
        });
        const fit = new FitAddon();
        terminal.loadAddon(fit);
        terminal.open(host);
        terminal.onScroll(() => {
          if (terminal)
            following = terminal.buffer.active.baseY - terminal.buffer.active.viewportY <= 1;
        });
        const fitVisible = () => {
          if (host.clientWidth && host.clientHeight) fit.fit();
        };
        fitVisible();
        resize = new ResizeObserver(fitVisible);
        resize.observe(host);
        ready = true;
        untrack(renderLatest);
      })
      .catch(() => {
        if (!disposed) failure = 'The terminal viewer could not load.';
      });
    return () => {
      disposed = true;
      resize?.disconnect();
      terminal?.dispose();
      terminal = undefined;
    };
  });
  function rawOutput() {
    return snapshot ? snapshot.chunks.map((chunk) => chunk.text).join('') : text;
  }
  function visibleOutput() {
    if (!terminal) return rawOutput();
    let result = '';
    for (let index = 0; index < terminal.buffer.active.length; index++) {
      const line = terminal.buffer.active.getLine(index);
      if (line) result += (index && !line.isWrapped ? '\n' : '') + line.translateToString(true);
    }
    return result.trimEnd();
  }
  async function copy() {
    try {
      await copyToClipboard(visibleOutput());
      toast.success('Copied terminal output');
    } catch {
      toast.error('Could not copy terminal output.');
    }
  }
  function download() {
    const url = URL.createObjectURL(new Blob([rawOutput()], { type: 'text/plain;charset=utf-8' }));
    const link = document.createElement('a');
    link.href = url;
    link.download = 'terminal.log';
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
</script>

<section aria-label={label} class="terminal-output min-w-0">
  <div class="flex min-h-11 items-center gap-1 text-[10px] text-muted-foreground">
    <span class="min-w-0 flex-1">Read-only terminal</span>
    {#if !following}<Button
        variant="ghost"
        size="icon"
        class="icon-button"
        aria-label="Follow terminal output"
        onclick={() => {
          terminal?.scrollToBottom();
          following = true;
        }}><ArrowDown class="size-3.5" /></Button
      >{/if}
    <Button
      variant="ghost"
      size="icon"
      class="icon-button"
      aria-label="Copy terminal output"
      onclick={() => void copy()}><Copy class="size-3.5" /></Button
    >
    <Button
      variant="ghost"
      size="icon"
      class="icon-button"
      aria-label="Download terminal log"
      onclick={download}><Download class="size-3.5" /></Button
    >
  </div>
  {#if !ready}<p class="text-xs text-muted-foreground" role="status">
      {failure || 'Loading terminal…'}
    </p>{/if}
  <div
    bind:this={host}
    class="terminal-host h-72 min-w-0 max-w-full overflow-hidden p-2"
    aria-label={label}
  ></div>
  {#if snapshot?.truncated || text.length > 524_288}<p
      class="mt-1 text-[10px] text-muted-foreground"
    >
      Earlier output omitted. The viewer keeps a bounded terminal history.
    </p>{/if}
</section>

<style>
  .terminal-host {
    background: var(--terminal-background);
    color: var(--terminal-foreground);
  }
  .terminal-host :global(.xterm) {
    height: 100%;
  }
  .terminal-host :global(.xterm-cursor) {
    visibility: hidden;
  }
  .terminal-host :global(.xterm-viewport) {
    overscroll-behavior: contain;
  }
</style>
