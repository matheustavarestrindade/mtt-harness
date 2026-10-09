// These keys intentionally match the Go JSON wire format. See root routes.md.
export interface Instance {
  ID: string;
  Workspace: string;
  Models: string[] | null;
  DefaultModel: string;
  ProcessLimit: number;
  AgentDepthLimit: number;
  CreatedAt: string;
  Stopped: boolean;
}
export interface Session {
  ID: string;
  InstanceID: string;
  Parent: string;
  Depth: number;
  Model: string;
  ReasoningEffort?: string;
  CreatedAt: string;
  Completed: boolean;
}
export type TaskStatus = 'pending' | 'in_progress' | 'done' | 'cancelled';
export interface TaskItem {
  ID: string;
  Title: string;
  Status: TaskStatus;
}
export interface DoingState {
  Title: string;
  Description: string;
}
export interface TaskState {
  SessionID: string;
  Todo: TaskItem[];
  Doing: DoingState | null;
  Revision: number;
  UpdatedAt: string;
}
export interface Content {
  Type: 'text' | 'image' | 'audio' | 'video' | 'file';
  Text: string;
  Data: string | null;
  MIME: string;
  URL: string;
  Filename: string;
  AudioID: string;
}
export interface ToolCall {
  ID: string;
  Name: string;
  Input: unknown;
}
export interface Message {
  ID: string;
  SessionID: string;
  Seq: number;
  Role: 'system' | 'user' | 'assistant' | 'tool' | 'runtime';
  Content: Content[] | null;
  Reasoning?: string;
  ToolCalls: ToolCall[] | null;
  ToolCallID: string;
  Usage: Usage | null;
  CreatedAt: string;
}
export interface Cost {
  Currency: string;
  Value: number;
  Estimated?: boolean;
}
export interface Usage {
  Input: number;
  CacheRead: number;
  CacheWrite: number;
  Output: number;
  Reasoning: number;
  Cost: Cost | null;
}
export interface Statistics extends Usage {
  Calls: number;
  Costs: Cost[] | null;
  CacheHitRate: number;
  CacheHitPercentage: number;
}
export interface SessionDeletion {
  status: 'deleted';
  session_ids: string[];
}
export interface Model {
  ID: string;
  APIModel?: string;
  ServiceTier?: string;
  Name?: string;
  Level: number;
  Input: string[] | null;
  Output: string[] | null;
  Tools: boolean;
  ToolSupportUnknown?: boolean;
  ContextMax: number;
  Reasoning?: boolean;
  ReasoningEfforts?: string[];
  DefaultReasoningEffort?: string;
  ReasoningSummary?: string;
  Billing?: 'tokens' | 'subscription' | '';
  Prices: {
    Currency: string;
    Input: number;
    Output: number;
    CacheRead: number;
    CacheWrite: number;
    CacheReadUnknown?: boolean;
    CacheWriteUnknown?: boolean;
    Reasoning?: number;
    Source?: string;
    Tiers?: PriceTier[];
  } | null;
}
export interface PriceTier {
  AboveInputTokens: number;
  Input: number;
  Output: number;
  CacheRead: number;
  CacheWrite: number;
  CacheReadUnknown?: boolean;
  CacheWriteUnknown?: boolean;
  Reasoning?: number;
}
export interface Provider {
  Protocol?: string;
  Authentication?: 'api_key' | 'chatgpt' | 'none' | '';
  Connected?: boolean;
  ModelCount?: number;
  Name: string;
  APIURL: string;
  ModelListURL: string;
  ModelListFormat?: 'openai' | 'codex';
  PriceTableURL: string;
  Interval: number;
  MetadataURL?: string;
  MetadataFormat?: string;
  MetadataProvider?: string;
  Billing?: 'tokens' | 'subscription' | '';
}

export interface DeviceLogin {
  ID: string;
  Provider: string;
  VerificationURL: string;
  UserCode: string;
  ExpiresAt: string;
  Status: 'pending' | 'connected' | 'cancelled' | 'expired' | 'error';
  Error: string;
}
export interface QueueStatus {
  running: boolean;
  queued: number;
  messages: string[] | null;
  error: string;
}
export interface AcceptedMessage {
  status: 'queued';
  position: number;
  message: Message;
}
export interface HarnessEvent {
  Seq: number;
  InstanceID: string;
  SessionID: string;
  Name: string;
  Payload: unknown;
  Time: string;
}
export interface PermissionRequest {
  ID: string;
  InstanceID: string;
  SessionID: string;
  Target: string;
  Why: string;
}
export interface InstanceInput {
  workspace: string;
  default_model: string;
  models?: string[];
  process_limit?: number;
  agent_depth_limit?: number;
}
