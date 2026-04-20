import { create } from 'zustand';
import type { ProjectCapabilities } from '@/lib/protocol';

interface ProjectsState {
  projects: ProjectCapabilities[];
  activeProject: string | null;
  hostVersion: string;

  setProjects: (projects: ProjectCapabilities[], version?: string) => void;
  setActiveProject: (name: string | null) => void;
  updateProjectStatus: (project: string, status: 'idle' | 'running' | 'waiting_permission', agentType?: string) => void;
}

export const useProjectsStore = create<ProjectsState>((set, get) => ({
  projects: [],
  activeProject: null,
  hostVersion: '',

  setProjects: (projects, version) => {
    const active = get().activeProject;
    const newActive = active && projects.some(p => p.project === active) ? active : (projects[0]?.project || null);
    set({ projects, activeProject: newActive, hostVersion: version || get().hostVersion });
  },

  setActiveProject: (name) => set({ activeProject: name }),

  updateProjectStatus: (project, status, agentType) => {
    set(state => ({
      projects: state.projects.map(p =>
        p.project === project
          ? { ...p, status, ...(agentType ? { agent_type: agentType } : {}) }
          : p
      ),
    }));
  },
}));
