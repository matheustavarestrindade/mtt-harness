import { HarnessApi, isHarnessEvent } from './client';
import type { HarnessEvent } from '../../atoms/types';
import type { SessionViewFrame, SessionViewState } from '../../atoms/session-view';

export type StreamState = 'connecting' | 'live' | 'retrying' | 'closed';

// Sequence cursors belong to a selected session. Reconnects resume that stream;
// selecting another session creates a fresh subscription to reconstruct permissions.
export function subscribeEvents(
  api: HarnessApi,
  sessionID: string,
  onEvent: (event: HarnessEvent) => void,
  onState: (state: StreamState) => void,
  options: {
    snapshot?: (state: SessionViewState, initial: boolean, covered: number) => void;
    recent?: (events: HarnessEvent[]) => void;
    deleted?: () => void;
    details?: boolean;
  } = {},
): () => void {
  let socket: WebSocket | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let cursor = 0;
  let retries = 0;
  let disposed = false;
  function connect() {
    if (disposed) return;
    onState(retries ? 'retrying' : 'connecting');
    const current = new WebSocket(
      api.eventsURL(sessionID, cursor, !!options.snapshot, options.details ?? true),
    );
    socket = current;
    socket.onopen = () => {
      if (!disposed && socket === current) {
        retries = 0;
        if (!options.snapshot) onState('live');
      }
    };
    socket.onmessage = (message) => {
      if (disposed || socket !== current) return;
      let event: unknown;
      try {
        event = JSON.parse(String(message.data));
      } catch {
        return;
      }
      if (event && typeof event === 'object' && 'type' in event) {
        const frame = event as SessionViewFrame;
        if (frame.type === 'state') {
          if (frame.session_id !== sessionID || frame.state?.Session?.ID !== sessionID) return;
          if (frame.initial) {
            cursor = frame.cursor;
            options.recent?.(frame.recent ?? []);
          }
          options.snapshot?.(frame.state, frame.initial, frame.covered_cursor ?? frame.cursor);
          onState('live');
          return;
        }
        if ((frame.type as string) === 'deleted' && frame.session_id === sessionID) {
          disposed = true;
          options.deleted?.();
          current.close();
          onState('closed');
          return;
        }
        if ((frame.type as string) === 'event' && 'event' in frame)
          event = (frame as unknown as { event: unknown }).event;
      }
      if (!isHarnessEvent(event) || event.SessionID !== sessionID || event.Seq <= cursor) return;
      cursor = event.Seq;
      onEvent(event);
    };
    socket.onerror = () => current.close();
    socket.onclose = () => {
      if (disposed || socket !== current) return;
      onState('retrying');
      timer = setTimeout(connect, Math.min(15_000, 1000 * 2 ** Math.min(retries++, 4)));
    };
  }
  connect();
  return () => {
    disposed = true;
    clearTimeout(timer);
    socket?.close();
    onState('closed');
  };
}
