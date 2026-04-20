import type { BridgeHost, BridgeIncoming, RegisterMsg, SendMessageMsg, CardActionMsg, FetchHistoryMsg } from './protocol';

export type ConnectionStatus = 'disconnected' | 'connecting' | 'registering' | 'connected' | 'error';

export interface BridgeClientOptions {
  host: BridgeHost;
  platformName?: string;
  onMessage: (msg: BridgeIncoming) => void;
  onStatusChange: (status: ConnectionStatus) => void;
}

export class BridgeClient {
  private ws: WebSocket | null = null;
  private pingTimer: ReturnType<typeof setInterval> | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private alive = true;
  private opts: BridgeClientOptions;

  constructor(opts: BridgeClientOptions) {
    this.opts = opts;
  }

  connect() {
    this.alive = true;
    this.doConnect();
  }

  disconnect() {
    this.alive = false;
    this.cleanup();
    this.opts.onStatusChange('disconnected');
  }

  send(data: Record<string, any>) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data));
    }
  }

  sendMessage(sessionKey: string, content: string, project?: string) {
    const msg: SendMessageMsg = {
      type: 'message',
      msg_id: `client-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
      session_key: sessionKey,
      user_id: 'client-user',
      user_name: 'Client',
      content,
      reply_ctx: sessionKey,
      project: project || '',
    };
    this.send(msg);
  }

  sendCardAction(sessionKey: string, action: string, project?: string) {
    const msg: CardActionMsg = {
      type: 'card_action',
      session_key: sessionKey,
      action,
      reply_ctx: sessionKey,
      project: project || '',
    };
    this.send(msg);
  }

  fetchHistory(sessionKey: string, sessionId: string, project?: string, beforeTs?: number) {
    const msg: FetchHistoryMsg = {
      type: 'fetch_history',
      session_key: sessionKey,
      session_id: sessionId,
      project,
      before_timestamp: beforeTs,
      limit: 50,
    };
    this.send(msg);
  }

  private doConnect() {
    if (!this.alive) return;
    this.opts.onStatusChange('connecting');

    const { host } = this.opts;
    const proto = host.host.startsWith('https') ? 'wss:' : 'ws:';
    const hostname = host.host.replace(/^https?:\/\//, '');
    const url = `${proto}//${hostname}:${host.port}/bridge/ws?token=${encodeURIComponent(host.token)}`;

    const ws = new WebSocket(url);
    this.ws = ws;

    ws.onopen = () => {
      this.opts.onStatusChange('registering');
      const reg: RegisterMsg = {
        type: 'register',
        platform: this.opts.platformName || 'cc-client',
        capabilities: [
          'text', 'card', 'buttons', 'typing',
          'update_message', 'preview', 'delete_message',
          'reconstruct_reply',
        ],
        metadata: {
          version: '1.0.0',
          description: 'CC-Connect Client App',
          control_plane: ['capabilities_snapshot_v1'],
        },
      };
      this.send(reg);
    };

    ws.onmessage = (evt) => {
      try {
        const msg = JSON.parse(typeof evt.data === 'string' ? evt.data : '') as BridgeIncoming;
        if (msg.type === 'register_ack') {
          if ((msg as any).ok) {
            this.opts.onStatusChange('connected');
            this.pingTimer = setInterval(() => {
              this.send({ type: 'ping', ts: Date.now() });
            }, 25000);
          } else {
            this.opts.onStatusChange('error');
          }
        }
        this.opts.onMessage(msg);
      } catch { /* ignore parse errors */ }
    };

    ws.onclose = () => {
      this.cleanup();
      this.opts.onStatusChange('disconnected');
      if (this.alive) {
        this.reconnectTimer = setTimeout(() => this.doConnect(), 3000);
      }
    };

    ws.onerror = () => {
      this.opts.onStatusChange('error');
    };
  }

  private cleanup() {
    if (this.pingTimer) {
      clearInterval(this.pingTimer);
      this.pingTimer = null;
    }
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      this.ws.onclose = null;
      this.ws.close();
      this.ws = null;
    }
  }
}
