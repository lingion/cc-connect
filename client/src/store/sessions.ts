import { create } from 'zustand';

export interface SessionInfo {
  id: string;
  name: string;
  history_count: number;
}

interface SessionsState {
  // Key: project name
  sessions: Record<string, SessionInfo[]>;
  activeIds: Record<string, string>;

  setSessions: (project: string, list: SessionInfo[], activeId: string) => void;
}

export const useSessionsStore = create<SessionsState>((set) => ({
  sessions: {},
  activeIds: {},

  setSessions: (project, list, activeId) => {
    set(state => ({
      sessions: { ...state.sessions, [project]: list },
      activeIds: { ...state.activeIds, [project]: activeId },
    }));
  },
}));
