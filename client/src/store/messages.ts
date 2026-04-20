import { create } from 'zustand';

export interface ChatMessage {
  id: string;
  role: 'user' | 'assistant' | 'system';
  content: string;
  format?: 'text' | 'markdown' | 'card' | 'buttons';
  card?: any;
  buttons?: { text: string; data: string }[][];
  streaming?: boolean;
  timestamp: number;
}

interface MessagesState {
  // Key: `${project}:${sessionId}`
  channels: Record<string, ChatMessage[]>;
  typing: Record<string, boolean>;

  addMessage: (channelKey: string, msg: ChatMessage) => void;
  updateStreamingMessage: (channelKey: string, content: string) => void;
  finishStreaming: (channelKey: string) => void;
  removeStreamingMessage: (channelKey: string) => void;
  setHistory: (channelKey: string, entries: { role: string; content: string; timestamp: number }[]) => void;
  setTyping: (channelKey: string, typing: boolean) => void;
  clearChannel: (channelKey: string) => void;
}

export const useMessagesStore = create<MessagesState>((set, get) => ({
  channels: {},
  typing: {},

  addMessage: (key, msg) => {
    set(state => {
      const existing = state.channels[key] || [];
      return { channels: { ...state.channels, [key]: [...existing, msg] } };
    });
  },

  updateStreamingMessage: (key, content) => {
    set(state => {
      const msgs = state.channels[key];
      if (!msgs || msgs.length === 0) return state;
      const last = msgs[msgs.length - 1];
      if (!last.streaming) return state;
      const updated = [...msgs.slice(0, -1), { ...last, content }];
      return { channels: { ...state.channels, [key]: updated } };
    });
  },

  finishStreaming: (key) => {
    set(state => {
      const msgs = state.channels[key];
      if (!msgs || msgs.length === 0) return state;
      const last = msgs[msgs.length - 1];
      if (!last.streaming) return state;
      const updated = [...msgs.slice(0, -1), { ...last, streaming: false }];
      return { channels: { ...state.channels, [key]: updated } };
    });
  },

  removeStreamingMessage: (key) => {
    set(state => {
      const msgs = state.channels[key];
      if (!msgs || msgs.length === 0) return state;
      const last = msgs[msgs.length - 1];
      if (!last.streaming) return state;
      return { channels: { ...state.channels, [key]: msgs.slice(0, -1) } };
    });
  },

  setHistory: (key, entries) => {
    const msgs: ChatMessage[] = entries.map((e, i) => ({
      id: `hist-${e.timestamp}-${i}`,
      role: e.role as 'user' | 'assistant',
      content: e.content,
      timestamp: e.timestamp,
    }));
    set(state => ({ channels: { ...state.channels, [key]: msgs } }));
  },

  setTyping: (key, typing) => {
    set(state => ({ typing: { ...state.typing, [key]: typing } }));
  },

  clearChannel: (key) => {
    set(state => {
      const { [key]: _, ...rest } = state.channels;
      return { channels: rest };
    });
  },
}));
