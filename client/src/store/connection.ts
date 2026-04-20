import { create } from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';
import type { BridgeHost } from '@/lib/protocol';
import type { ConnectionStatus } from '@/lib/bridge-client';

const HOSTS_KEY = 'cc-hosts';
const ACTIVE_KEY = 'cc-active';

interface ConnectionState {
  hosts: BridgeHost[];
  activeHostName: string | null;
  status: ConnectionStatus;

  loadHosts: () => Promise<void>;
  addHost: (host: BridgeHost) => Promise<void>;
  removeHost: (name: string) => Promise<void>;
  setActiveHost: (name: string | null) => Promise<void>;
  setStatus: (s: ConnectionStatus) => void;
  getActiveHost: () => BridgeHost | null;
}

export const useConnectionStore = create<ConnectionState>((set, get) => ({
  hosts: [],
  activeHostName: null,
  status: 'disconnected',

  loadHosts: async () => {
    try {
      const raw = await AsyncStorage.getItem(HOSTS_KEY);
      const hosts: BridgeHost[] = raw ? JSON.parse(raw) : [];
      const active = await AsyncStorage.getItem(ACTIVE_KEY);
      set({ hosts, activeHostName: active || (hosts.length > 0 ? hosts[0].name : null) });
    } catch { /* ignore */ }
  },

  addHost: async (host) => {
    const hosts = [...get().hosts.filter(h => h.name !== host.name), host];
    await AsyncStorage.setItem(HOSTS_KEY, JSON.stringify(hosts));
    set({ hosts, activeHostName: host.name });
    await AsyncStorage.setItem(ACTIVE_KEY, host.name);
  },

  removeHost: async (name) => {
    const hosts = get().hosts.filter(h => h.name !== name);
    await AsyncStorage.setItem(HOSTS_KEY, JSON.stringify(hosts));
    const active = get().activeHostName === name ? (hosts[0]?.name || null) : get().activeHostName;
    set({ hosts, activeHostName: active });
    if (active) await AsyncStorage.setItem(ACTIVE_KEY, active);
  },

  setActiveHost: async (name) => {
    set({ activeHostName: name });
    if (name) await AsyncStorage.setItem(ACTIVE_KEY, name);
  },

  setStatus: (status) => set({ status }),

  getActiveHost: () => {
    const { hosts, activeHostName } = get();
    return hosts.find(h => h.name === activeHostName) || null;
  },
}));
