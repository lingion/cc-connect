// Bridge WebSocket protocol types — mirrors core/bridge.go

export interface BridgeHost {
  name: string;
  host: string;
  port: number;
  token: string;
}

// --- Outbound (client → server) ---

export interface RegisterMsg {
  type: 'register';
  platform: string;
  capabilities: string[];
  metadata: Record<string, any>;
}

export interface SendMessageMsg {
  type: 'message';
  msg_id: string;
  session_key: string;
  user_id: string;
  user_name: string;
  content: string;
  reply_ctx: string;
  project?: string;
}

export interface CardActionMsg {
  type: 'card_action';
  session_key: string;
  action: string;
  reply_ctx: string;
  project?: string;
}

export interface FetchHistoryMsg {
  type: 'fetch_history';
  session_key: string;
  session_id: string;
  project?: string;
  before_timestamp?: number;
  limit?: number;
}

// --- Inbound (server → client) ---

export interface RegisterAck {
  type: 'register_ack';
  ok: boolean;
  error?: string;
}

export interface CapabilitiesSnapshot {
  type: 'capabilities_snapshot';
  v: number;
  host: {
    id: string;
    hostname?: string;
    cc_connect_version?: string;
    commit?: string;
    build_time?: string;
  };
  projects: ProjectCapabilities[];
}

export interface ProjectCapabilities {
  project: string;
  agent_type?: string;
  work_dir?: string;
  status?: 'idle' | 'running' | 'waiting_permission';
  commands: PublishedCommand[];
}

export interface PublishedCommand {
  name: string;
  description: string;
  source: string;
  requires_workspace: boolean;
  args_mode: string;
}

export interface ReplyMsg {
  type: 'reply';
  session_key: string;
  reply_ctx: string;
  content: string;
  format?: string;
}

export interface CardMsg {
  type: 'card';
  session_key: string;
  reply_ctx: string;
  card: BridgeCard;
}

export interface BridgeCard {
  header?: { title: string; color?: string };
  elements: BridgeCardElement[];
}

export type BridgeCardElement =
  | { type: 'markdown'; content: string }
  | { type: 'divider' }
  | { type: 'note'; text: string; tag?: string }
  | { type: 'actions'; buttons: { text: string; btn_type: string; value: string }[]; layout?: string }
  | { type: 'list_item'; text: string; btn_text: string; btn_type: string; btn_value: string }
  | { type: 'select'; placeholder: string; options: { text: string; value: string }[]; init_value?: string };

export interface ButtonsMsg {
  type: 'buttons';
  session_key: string;
  reply_ctx: string;
  content: string;
  buttons: { text: string; data: string }[][];
}

export interface PreviewStartMsg {
  type: 'preview_start';
  ref_id: string;
  session_key: string;
  reply_ctx: string;
  content: string;
}

export interface UpdateMessageMsg {
  type: 'update_message';
  session_key: string;
  preview_handle: string;
  content: string;
}

export interface DeleteMessageMsg {
  type: 'delete_message';
  session_key: string;
  preview_handle: string;
}

export interface TypingMsg {
  type: 'typing_start' | 'typing_stop';
  session_key: string;
}

export interface SessionListUpdate {
  type: 'session_list_update';
  project: string;
  session_key: string;
  sessions: { id: string; name: string; history_count: number }[];
  active_id: string;
}

export interface AgentStatusUpdate {
  type: 'agent_status_update';
  project: string;
  status: 'idle' | 'running' | 'waiting_permission';
  agent_type: string;
}

export interface HistorySync {
  type: 'history_sync';
  project: string;
  session_key: string;
  session_id: string;
  entries: { role: string; content: string; timestamp: number }[];
  has_older: boolean;
}

export type BridgeIncoming =
  | RegisterAck
  | CapabilitiesSnapshot
  | ReplyMsg
  | CardMsg
  | ButtonsMsg
  | PreviewStartMsg
  | UpdateMessageMsg
  | DeleteMessageMsg
  | TypingMsg
  | SessionListUpdate
  | AgentStatusUpdate
  | HistorySync
  | { type: 'pong'; ts: number }
  | { type: 'audio'; session_key: string; data: string; format: string }
  | { type: string; [key: string]: any };
