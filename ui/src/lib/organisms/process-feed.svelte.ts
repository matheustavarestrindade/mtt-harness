import {
  processOutputEvent,
  type ProcessExit,
  type ProcessRecord,
  type TerminalChunk,
  type TerminalSnapshot,
  terminalChunkSize,
} from '../atoms/processes';
import { HarnessApi } from '../molecules/api/client';

const retainedTextLimit = 524_288;

// One open terminal owns a bounded output connection. Reconnects begin a new
// snapshot; byte offsets detect dropped output without duplicating old lines.
export class ProcessFeed {
  snapshot = $state<TerminalSnapshot>({ generation: 0, chunks: [], truncated: false });
  state = $state<'connecting' | 'live' | 'retrying' | 'stopped' | 'unavailable'>('connecting');
  exit = $state<ProcessExit | null>(null);
  error = $state('');
  inputError = $state('');
  terminal = $state(false);
  inputEnabled = $state(false);
  private size = { columns: 120, rows: 30 };
  private resizeTimer: ReturnType<typeof setTimeout> | undefined;
  private socket: WebSocket | undefined;
  private retryTimer: ReturnType<typeof setTimeout> | undefined;
  private flushTimer: ReturnType<typeof setTimeout> | undefined;
  private pending: TerminalChunk[] = [];
  private pendingLength = 0;
  private nextID = 0;
  private retries = 0;
  private closed = false;
  private generation = 0;
  private offsets = { stdout: 0, stderr: 0 };
  private decoders = { stdout: new TextDecoder(), stderr: new TextDecoder() };

  constructor(
    private api: HarnessApi,
    readonly record: ProcessRecord,
    private onRecord?: (record: ProcessRecord) => void,
  ) {
    this.connect();
  }
  private connect() {
    if (this.closed) return;
    this.inputEnabled = false;
    this.state = this.retries ? 'retrying' : 'connecting';
    const socket = new WebSocket(this.api.processOutputURL(this.record.ID));
    this.socket = socket;
    socket.onopen = () => {
      if (!this.closed && this.socket === socket) this.state = 'live';
    };
    socket.onmessage = (message) => {
      if (this.closed || this.socket !== socket) return;
      let value: unknown;
      try {
        value = JSON.parse(String(message.data));
      } catch {
        this.error = 'Invalid process event.';
        socket.close();
        return;
      }
      const event = processOutputEvent(value, this.record.ID);
      if (!event) {
        this.error = 'Process stream metadata is unavailable.';
        socket.close();
        return;
      }
      this.retries = 0;
      if (event.stream === 'control') {
        this.inputError = event.error || '';
        return;
      }
      if (event.stream === 'start') {
        clearTimeout(this.flushTimer);
        this.flushTimer = undefined;
        this.pending = [];
        this.pendingLength = 0;
        this.offsets = { stdout: 0, stderr: 0 };
        this.decoders = { stdout: new TextDecoder(), stderr: new TextDecoder() };
        this.snapshot = { generation: ++this.generation, chunks: [], truncated: false };
        this.error = '';
        this.terminal = event.terminal ?? this.record.Spec.Terminal ?? false;
        this.inputEnabled = event.input === true;
        this.inputError = '';
        this.sendSize();
        return;
      }
      if (event.stream === 'exit') {
        if (!this.terminal)
          this.append(this.decoders.stdout.decode() + this.decoders.stderr.decode());
        this.flush();
        this.exit = event.exit ?? this.record.Exit;
        this.onRecord?.({
          ...this.record,
          Status: 'stopped',
          Exit: this.exit,
          EndedAt: event.at ?? this.record.EndedAt,
        });
        this.error = event.error || this.exit?.Error || '';
        this.state = 'stopped';
        this.inputEnabled = false;
        socket.close();
        return;
      }
      let bytes: Uint8Array;
      try {
        if (typeof event.data_base64 === 'string')
          bytes = Uint8Array.from(atob(event.data_base64), (character) => character.charCodeAt(0));
        else bytes = new TextEncoder().encode(event.data ?? '');
      } catch {
        this.error = 'Invalid process output bytes.';
        socket.close();
        return;
      }
      const stream = event.stream;
      if (event.snapshot) {
        this.offsets[stream] = event.offset;
        if (event.offset > 0) this.snapshot = { ...this.snapshot, truncated: true };
      }
      const expected = this.offsets[stream];
      if (event.offset > expected) {
        this.error = 'Recovering an output gap…';
        socket.close();
        return;
      }
      const overlap = expected - event.offset;
      if (overlap >= bytes.length) return;
      const fresh = bytes.subarray(overlap);
      this.offsets[stream] += fresh.length;
      if (this.terminal) this.append('', fresh.slice());
      else this.append(this.decoders[stream].decode(fresh, { stream: true }));
    };
    socket.onerror = () => socket.close();
    socket.onclose = () => {
      if (this.closed || this.socket !== socket || this.state === 'stopped') return;
      this.inputEnabled = false;
      if (
        (this.record.Status === 'stopped' && !this.snapshot.chunks.length) ||
        ++this.retries > 3
      ) {
        this.state = 'unavailable';
        this.error ||= 'Process output is not retained or the connection is unavailable.';
        return;
      }
      this.state = 'retrying';
      this.retryTimer = setTimeout(
        () => this.connect(),
        Math.min(5000, 500 * 2 ** (this.retries - 1)),
      );
    };
  }
  private append(text: string, bytes?: Uint8Array) {
    if (!text && !bytes?.length) return;
    this.pending.push({ id: ++this.nextID, text, bytes });
    this.pendingLength += bytes?.length ?? text.length;
    while (
      (this.pendingLength > retainedTextLimit || this.pending.length > 512) &&
      this.pending.length > 1
    ) {
      this.pendingLength -= terminalChunkSize(this.pending.shift()!);
      this.snapshot = { ...this.snapshot, truncated: true };
    }
    if (!this.flushTimer) this.flushTimer = setTimeout(() => this.flush(), 100);
  }
  private flush() {
    clearTimeout(this.flushTimer);
    this.flushTimer = undefined;
    if (!this.pending.length) return;
    const chunks = [...this.snapshot.chunks, ...this.pending];
    this.pending = [];
    this.pendingLength = 0;
    let length = chunks.reduce((total, chunk) => total + terminalChunkSize(chunk), 0);
    let truncated = this.snapshot.truncated;
    while ((length > retainedTextLimit || chunks.length > 512) && chunks.length > 1) {
      length -= terminalChunkSize(chunks.shift()!);
      truncated = true;
    }
    if (chunks[0] && terminalChunkSize(chunks[0]) > retainedTextLimit) {
      chunks[0] = {
        ...chunks[0],
        text: chunks[0].text.slice(-retainedTextLimit),
        bytes: chunks[0].bytes?.slice(-retainedTextLimit),
      };
      truncated = true;
    }
    this.snapshot = { generation: this.generation, chunks, truncated };
  }
  get canInput() {
    return !this.closed && this.state === 'live' && this.inputEnabled;
  }
  sendInput(value: string | Uint8Array) {
    if (!this.canInput) return;
    if (!this.terminal && value === '\x03') {
      this.sendControl({ type: 'signal', signal: 'interrupt' });
      return;
    }
    const bytes = typeof value === 'string' ? new TextEncoder().encode(value) : value;
    if (
      bytes.length > 65_536 ||
      (this.socket?.bufferedAmount ?? 0) + bytes.length * 1.5 > 131_072
    ) {
      this.inputError = 'Terminal input buffer is full.';
      return;
    }
    for (let offset = 0; offset < bytes.length; offset += 4096) {
      const part = bytes.subarray(offset, offset + 4096);
      this.sendControl({ type: 'input', data_base64: btoa(String.fromCharCode(...part)) });
    }
  }
  interrupt() {
    if (this.terminal) this.sendInput('\x03');
    else if (this.canInput) this.sendControl({ type: 'signal', signal: 'interrupt' });
  }
  resize(columns: number, rows: number) {
    this.size = {
      columns: Math.min(500, Math.max(2, columns)),
      rows: Math.min(200, Math.max(1, rows)),
    };
    clearTimeout(this.resizeTimer);
    this.resizeTimer = setTimeout(() => this.sendSize(), 50);
  }
  private sendSize() {
    if (this.canInput && this.terminal) this.sendControl({ type: 'resize', ...this.size });
  }
  private sendControl(value: unknown) {
    if (this.socket?.readyState !== WebSocket.OPEN) return;
    try {
      this.socket.send(JSON.stringify(value));
      this.inputError = '';
    } catch {
      this.inputError = 'Terminal input could not be sent.';
    }
  }
  reconnect() {
    clearTimeout(this.retryTimer);
    this.socket?.close();
    this.retries = 0;
    this.connect();
  }
  close() {
    this.closed = true;
    clearTimeout(this.retryTimer);
    clearTimeout(this.flushTimer);
    clearTimeout(this.resizeTimer);
    this.socket?.close();
    this.socket = undefined;
    this.pending = [];
    this.pendingLength = 0;
    this.inputEnabled = false;
    this.snapshot = { generation: ++this.generation, chunks: [], truncated: false };
  }
}
