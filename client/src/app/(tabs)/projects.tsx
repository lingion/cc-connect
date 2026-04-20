import { useEffect, useRef, useCallback } from 'react';
import { View, Text, TouchableOpacity, FlatList, StyleSheet } from 'react-native';
import { useRouter } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useTheme } from '@/lib/theme';
import { useConnectionStore } from '@/store/connection';
import { useProjectsStore } from '@/store/projects';
import { useSessionsStore } from '@/store/sessions';
import { useMessagesStore } from '@/store/messages';
import { BridgeClient } from '@/lib/bridge-client';
import type { BridgeIncoming, CapabilitiesSnapshot, AgentStatusUpdate, SessionListUpdate, HistorySync } from '@/lib/protocol';

let globalClient: BridgeClient | null = null;
export function getGlobalClient() { return globalClient; }

export default function ProjectsScreen() {
  const t = useTheme();
  const router = useRouter();
  const activeHost = useConnectionStore(s => s.getActiveHost());
  const status = useConnectionStore(s => s.status);
  const setStatus = useConnectionStore(s => s.setStatus);
  const projects = useProjectsStore(s => s.projects);
  const setProjects = useProjectsStore(s => s.setProjects);
  const updateProjectStatus = useProjectsStore(s => s.updateProjectStatus);
  const setActiveProject = useProjectsStore(s => s.setActiveProject);
  const hostVersion = useProjectsStore(s => s.hostVersion);
  const setSessions = useSessionsStore(s => s.setSessions);
  const setHistory = useMessagesStore(s => s.setHistory);
  const clientRef = useRef<BridgeClient | null>(null);

  const handleMessage = useCallback((msg: BridgeIncoming) => {
    switch (msg.type) {
      case 'capabilities_snapshot': {
        const snap = msg as CapabilitiesSnapshot;
        setProjects(snap.projects, snap.host.cc_connect_version);
        break;
      }
      case 'agent_status_update': {
        const upd = msg as AgentStatusUpdate;
        updateProjectStatus(upd.project, upd.status, upd.agent_type);
        break;
      }
      case 'session_list_update': {
        const sl = msg as SessionListUpdate;
        setSessions(sl.project, sl.sessions, sl.active_id);
        break;
      }
      case 'history_sync': {
        const hs = msg as HistorySync;
        const key = `${hs.project}:${hs.session_id}`;
        setHistory(key, hs.entries);
        break;
      }
    }
  }, [setProjects, updateProjectStatus, setSessions, setHistory]);

  useEffect(() => {
    if (!activeHost) return;
    const client = new BridgeClient({
      host: activeHost,
      onMessage: handleMessage,
      onStatusChange: setStatus,
    });
    clientRef.current = client;
    globalClient = client;
    client.connect();
    return () => {
      client.disconnect();
      globalClient = null;
    };
  }, [activeHost?.name, activeHost?.host, activeHost?.port]);

  const handleProjectPress = (projectName: string) => {
    setActiveProject(projectName);
    router.push('/(tabs)/chat');
  };

  const statusColor = (s?: string) => {
    if (s === 'running') return t.accent;
    if (s === 'waiting_permission') return t.danger;
    return t.textQuaternary;
  };

  const statusLabel = (s?: string) => {
    if (s === 'running') return 'Running';
    if (s === 'waiting_permission') return 'Needs attention';
    return 'Idle';
  };

  const st = styles(t);
  return (
    <SafeAreaView style={[st.root, { backgroundColor: t.bg }]} edges={['top']}>
      <View style={[st.header, { borderBottomColor: t.border }]}>
        <Text style={[st.brand, { color: t.text }]}>
          CC<Text style={{ color: t.accent }}>-</Text>Connect
        </Text>
        <View style={st.headerRight}>
          <View style={[st.connDot, { backgroundColor: status === 'connected' ? t.accent : t.textQuaternary }]} />
          <Text style={[st.connLabel, { color: t.textTertiary }]}>
            {status === 'connected' ? activeHost?.name || 'Connected' : status}
          </Text>
        </View>
      </View>

      {hostVersion ? (
        <Text style={[st.versionBadge, { color: t.textQuaternary }]}>v{hostVersion}</Text>
      ) : null}

      <FlatList
        data={projects}
        keyExtractor={p => p.project}
        contentContainerStyle={st.list}
        renderItem={({ item: p }) => (
          <TouchableOpacity
            style={[st.card, { backgroundColor: t.bgCard, borderColor: t.border }]}
            onPress={() => handleProjectPress(p.project)}
            activeOpacity={0.7}
          >
            <View style={st.cardTop}>
              <View style={[st.statusDot, { backgroundColor: statusColor(p.status) }]} />
              <Text style={[st.projectName, { color: t.text }]}>{p.project}</Text>
            </View>
            <Text style={[st.agentType, { color: t.textSecondary }]}>{p.agent_type || 'Unknown'}</Text>
            {p.work_dir ? (
              <Text style={[st.workDir, { color: t.textTertiary }]} numberOfLines={1}>{p.work_dir}</Text>
            ) : null}
            <View style={st.cardFooter}>
              <Text style={[st.statusText, { color: statusColor(p.status) }]}>{statusLabel(p.status)}</Text>
              <Text style={{ color: t.textQuaternary, fontSize: 12 }}>{p.commands?.length || 0} commands</Text>
            </View>
          </TouchableOpacity>
        )}
        ListEmptyComponent={
          <View style={st.empty}>
            <Text style={[st.emptyText, { color: t.textTertiary }]}>
              {status === 'connected' ? 'No projects found' : 'Connecting...'}
            </Text>
          </View>
        }
      />
    </SafeAreaView>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    root: { flex: 1 },
    header: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', paddingHorizontal: 20, height: 52, borderBottomWidth: 1 },
    brand: { fontSize: 16, fontWeight: '700', letterSpacing: -0.3 },
    headerRight: { flexDirection: 'row', alignItems: 'center', gap: 6 },
    connDot: { width: 6, height: 6, borderRadius: 3 },
    connLabel: { fontSize: 12 },
    versionBadge: { fontSize: 10, textAlign: 'center', marginTop: 8 },
    list: { padding: 16, gap: 10 },
    card: { borderRadius: 12, borderWidth: 1, padding: 16 },
    cardTop: { flexDirection: 'row', alignItems: 'center', gap: 10, marginBottom: 4 },
    statusDot: { width: 8, height: 8, borderRadius: 4 },
    projectName: { fontSize: 15, fontWeight: '600' },
    agentType: { fontSize: 13, marginLeft: 18, marginBottom: 2 },
    workDir: { fontSize: 11, fontFamily: 'monospace', marginLeft: 18, marginBottom: 8 },
    cardFooter: { flexDirection: 'row', justifyContent: 'space-between', marginLeft: 18, marginTop: 4 },
    statusText: { fontSize: 12, fontWeight: '500' },
    empty: { alignItems: 'center', paddingTop: 80 },
    emptyText: { fontSize: 14 },
  });
