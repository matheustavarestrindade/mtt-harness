import type { Session } from './types';

export function sessionRoot(session: Session, sessions: Session[]): Session | null {
  const indexed = new Map(sessions.map((entry) => [entry.ID, entry]));
  const visited = new Set<string>();
  let current = session;
  while (current.Parent) {
    if (visited.has(current.ID)) return null;
    visited.add(current.ID);
    const parent = indexed.get(current.Parent);
    if (!parent || parent.InstanceID !== session.InstanceID) return null;
    current = parent;
  }
  return current;
}

export function sessionChildren(sessions: Session[], parentID: string): Session[] {
  return sessions
    .filter((session) => session.Parent === parentID && session.ID !== parentID)
    .sort(
      (first, second) =>
        first.CreatedAt.localeCompare(second.CreatedAt) || first.ID.localeCompare(second.ID),
    );
}

export function sessionDescendants(sessions: Session[], rootID: string): Session[] {
  const root = sessions.find((session) => session.ID === rootID);
  if (!root) return [];
  const children = new Map<string, Session[]>();
  for (const session of sessions) {
    if (session.InstanceID !== root.InstanceID || !session.Parent || session.ID === session.Parent)
      continue;
    const siblings = children.get(session.Parent) ?? [];
    siblings.push(session);
    children.set(session.Parent, siblings);
  }
  const included = new Set([rootID]);
  const descendants: Session[] = [];
  for (let position = 0, parents = [rootID]; position < parents.length; position++) {
    for (const child of children.get(parents[position]) ?? []) {
      if (included.has(child.ID)) continue;
      included.add(child.ID);
      parents.push(child.ID);
      descendants.push(child);
    }
  }
  return descendants;
}

export function topLevelAgent(
  sessionID: string,
  rootID: string,
  sessions: Session[],
): Session | null {
  const indexed = new Map(sessions.map((session) => [session.ID, session]));
  const visited = new Set<string>();
  let current = indexed.get(sessionID);
  while (current?.Parent && current.Parent !== rootID) {
    if (visited.has(current.ID)) return null;
    visited.add(current.ID);
    current = indexed.get(current.Parent);
  }
  return current?.Parent === rootID ? current : null;
}
