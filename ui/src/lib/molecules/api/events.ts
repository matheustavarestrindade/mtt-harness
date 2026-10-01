import { HarnessApi, isHarnessEvent } from './client';
import type { HarnessEvent } from '../../atoms/types';

export type StreamState = 'connecting' | 'live' | 'retrying' | 'closed';

// Sequence cursors belong to a selected session. Reconnects resume that stream;
// selecting another session creates a fresh subscription to reconstruct permissions.
export function subscribeEvents(
  api: HarnessApi,
  sessionID: string,
  onEvent: (event: HarnessEvent) => void,
  onState: (state: StreamState) => void,
): () => void {
  let socket: WebSocket | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let cursor = 0;
  let retries = 0;
  let disposed = false;
  function connect() {
    if (disposed) return;
    onState(retries ? 'retrying' : 'connecting');
    socket = new WebSocket(api.eventsURL(sessionID, cursor));
    socket.onopen = () => {
      if (!disposed) {
        retries = 0;
        onState('live');
      }
    };
    socket.onmessage = (message) => {
      if (disposed) return;
      let event: unknown;
      try {
        event = JSON.parse(String(message.data));
      } catch {
        return;
      }
      if (!isHarnessEvent(event) || event.SessionID !== sessionID || event.Seq <= cursor) return;
      cursor = event.Seq;
      onEvent(event);
    };
    socket.onerror = () => socket?.close();
    socket.onclose = () => {
      if (disposed) return;
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
