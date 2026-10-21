import type { ConversationEntry } from './transcript';

export interface PluginActivityNote {
  Text: string;
  References: string[] | null;
}
export interface PluginActivityEntry {
  Outcome?: string;
  Decision?: string;
  ContinueChat?: boolean;
  Trigger?: string;
  DefinitionID?: string;
  WorkerSessionID?: string;
  DurationMS?: number;
  Error?: string;
  InputPreview?: string;
  InputTruncated?: boolean;
  ID: string;
  SessionID: string;
  AnchorID: string;
  AnchorSequence: number;
  Title: string;
  Notes: PluginActivityNote[];
  CreatedAt: string;
  InputTokens?: number;
  Estimated: boolean;
}
export interface PluginActivityPage {
  Name: string;
  SessionID: string;
  Revision: number;
  HistoryRevision: number;
  Entries: PluginActivityEntry[] | null;
  NextCursor?: string;
}

export function comparePluginActivity(first: PluginActivityEntry, second: PluginActivityEntry) {
  return (
    Date.parse(first.CreatedAt) - Date.parse(second.CreatedAt) || first.ID.localeCompare(second.ID)
  );
}

// Tool results and process notices can render inside another message. Resolve
// the actual saved anchor IDs, never infer placement from task text or time.
export function placePluginActivity(entries: ConversationEntry[], activity: PluginActivityEntry[]) {
  const owners = new Map<string, string>();
  for (const entry of entries) {
    const key = entry.message.ID;
    const scope = entry.message.SessionID;
    owners.set(`${scope}:${key}`, key);
    for (const identifier of entry.relatedMessageIDs ?? [])
      owners.set(`${scope}:${identifier}`, key);
    for (const tool of entry.tools) {
      if (tool.result) owners.set(`${scope}:${tool.result.ID}`, key);
      for (const identifier of tool.relatedMessageIDs ?? [])
        owners.set(`${scope}:${identifier}`, key);
    }
  }
  const anchored = new Map<string, PluginActivityEntry[]>();
  const unplaced: PluginActivityEntry[] = [];
  for (const item of [...activity].sort(comparePluginActivity)) {
    const owner = owners.get(`${item.SessionID}:${item.AnchorID}`);
    if (!owner) {
      unplaced.push(item);
      continue;
    }
    const group = anchored.get(owner) ?? [];
    group.push(item);
    anchored.set(owner, group);
  }
  return { anchored, unplaced };
}
