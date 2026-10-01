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
  CreatedAt: string;
  Completed: boolean;
}
export interface Content {
  Type: 'text' | 'image' | 'audio' | 'file';
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
  Role: 'system' | 'user' | 'assistant' | 'tool';
  Content: Content[] | null;
  ToolCalls: ToolCall[] | null;
  ToolCallID: string;
  Usage: Usage | null;
  CreatedAt: string;
}
export interface Cost {
  Currency: string;
  Value: number;
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
export interface Model {
  ID: string;
  Name?: string;
  Level: number;
  Input: string[] | null;
  Output: string[] | null;
  Tools: boolean;
  ToolSupportUnknown?: boolean;
  ContextMax: number;
  Prices: {
    Currency: string;
    Input: number;
    Output: number;
    CacheRead: number;
    CacheWrite: number;
  } | null;
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
