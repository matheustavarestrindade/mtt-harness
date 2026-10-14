import { SvelteMap } from 'svelte/reactivity';
import { eventPayload } from '../atoms/agent-activity';
import { sessionChildren, sessionDescendants, topLevelAgent } from '../atoms/session-tree';
import type { HarnessEvent, Session } from '../atoms/types';
import type { HarnessApi } from '../molecules/api/client';
import type { NavigationHistory } from '../atoms/navigation-route';
import { AgentFeed } from './agent-feed.svelte';

interface AgentOrigin {
  parentID: string;
  messageID: string;
  task: string;
}

// Relationships come from Session.Parent. Ordered parent events place runs in
// their spawning response; no task-text or parallel-call matching is necessary.
export class AgentMonitor {
  feeds = new SvelteMap<string, AgentFeed>();
  origins = new SvelteMap<string, AgentOrigin>();
  focusedID = $state('');
  focusRevision = $state(0);
  error = $state('');
  sessions = $state<Session[]>([]);
  rootID = $state('');
  rootRunning = $state(false);
  rootStartedAt = $state('');
  private currentResponses = new Map<string, string>();
  private api: HarnessApi | null = null;

  constructor(
    private refreshSessions: () => void,
    private updateSession: (session: Session) => void,
    private onFocus?: (sessionID: string, history: NavigationHistory) => void,
  ) {}

  open(api: HarnessApi, root: Session, sessions: Session[]) {
    this.close();
    this.api = api;
    this.rootID = root.ID;
    this.sync(sessions, false);
  }

  sync(sessions: Session[], running: boolean) {
    this.sessions = sessions;
    this.rootRunning = running;
    if (!this.api || !this.rootID) return;
    const descendants = sessionDescendants(sessions, this.rootID);
    const included = new Set(descendants.map((session) => session.ID));
    for (const [identifier, feed] of this.feeds) {
      if (included.has(identifier)) continue;
      feed.close();
      this.feeds.delete(identifier);
      this.origins.delete(identifier);
    }
    for (const session of descendants) {
      let feed = this.feeds.get(session.ID);
      if (!feed) {
        feed = new AgentFeed(this.api, session, (event) => this.observe(event), this.updateSession);
        this.feeds.set(session.ID, feed);
      } else feed.updateSession(session);
      feed.parentRunning = running;
      feed.parentStartedAt = this.rootStartedAt;
    }
    if (this.focusedID && !included.has(this.focusedID)) this.clearFocus('replace');
  }

  observe(event: HarnessEvent) {
    if (!this.api || (event.SessionID !== this.rootID && !this.feeds.has(event.SessionID))) return;
    const payload = eventPayload(event);
    if (event.Name === 'model.call' && typeof payload.message_id === 'string')
      this.currentResponses.set(event.SessionID, payload.message_id);
    if (event.Name === 'turn.start' && event.SessionID === this.rootID) {
      this.rootStartedAt = event.Time;
      for (const feed of this.feeds.values()) feed.parentStartedAt = event.Time;
    }
    if (event.Name === 'agent.start' && typeof payload.session === 'string') {
      this.origins.set(payload.session, {
        parentID: event.SessionID,
        messageID: this.currentResponses.get(event.SessionID) ?? '',
        task: typeof payload.task === 'string' ? payload.task : '',
      });
      this.refreshSessions();
    }
    if (event.Name === 'agent.end') this.refreshSessions();
  }

  children(parentID: string, messageID?: string) {
    return sessionChildren(this.sessions, parentID).filter((session) => {
      if (messageID === undefined) return true;
      const origin = this.origins.get(session.ID);
      return origin?.parentID === parentID && origin.messageID === messageID;
    });
  }

  unplaced(messageIDs: Set<string>) {
    return this.children(this.rootID).filter((session) => {
      const origin = this.origins.get(session.ID);
      return !origin || origin.parentID !== this.rootID || !messageIDs.has(origin.messageID);
    });
  }

  task(sessionID: string) {
    return this.origins.get(sessionID)?.task || this.feeds.get(sessionID)?.task || '';
  }

  focus(sessionID: string) {
    const top = topLevelAgent(sessionID, this.rootID, this.sessions);
    if (!top) return null;
    this.focusedID = sessionID;
    this.focusRevision++;
    this.onFocus?.(sessionID, 'push');
    return top.ID;
  }

  clearFocus(history: NavigationHistory = 'push') {
    if (!this.focusedID) return;
    this.focusedID = '';
    this.onFocus?.('', history);
  }

  close() {
    for (const feed of this.feeds.values()) feed.close();
    this.feeds.clear();
    this.origins.clear();
    this.currentResponses.clear();
    this.rootID = '';
    this.focusedID = '';
    this.rootRunning = false;
    this.rootStartedAt = '';
    this.sessions = [];
    this.error = '';
    this.api = null;
  }
}
