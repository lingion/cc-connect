import { View, Text, TouchableOpacity, ScrollView, StyleSheet, Alert } from 'react-native';
import { useRouter } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useTheme } from '@/lib/theme';
import { useConnectionStore } from '@/store/connection';
import { useProjectsStore } from '@/store/projects';

export default function SettingsScreen() {
  const t = useTheme();
  const router = useRouter();
  const { hosts, activeHostName, removeHost } = useConnectionStore();
  const hostVersion = useProjectsStore(s => s.hostVersion);
  const status = useConnectionStore(s => s.status);

  const handleRemoveHost = (name: string) => {
    Alert.alert('Remove Connection', `Remove "${name}"?`, [
      { text: 'Cancel', style: 'cancel' },
      { text: 'Remove', style: 'destructive', onPress: () => removeHost(name) },
    ]);
  };

  const st = styles(t);
  return (
    <SafeAreaView style={[st.root, { backgroundColor: t.bg }]} edges={['top']}>
      <View style={[st.header, { borderBottomColor: t.border }]}>
        <Text style={[st.title, { color: t.text }]}>Settings</Text>
      </View>

      <ScrollView contentContainerStyle={st.content}>
        <Text style={[st.section, { color: t.textTertiary }]}>Connections</Text>
        {hosts.map(h => (
          <View key={h.name} style={[st.hostRow, { backgroundColor: t.bgCard, borderColor: t.border }]}>
            <View style={{ flex: 1 }}>
              <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
                <View style={[st.dot, { backgroundColor: h.name === activeHostName && status === 'connected' ? t.accent : t.textQuaternary }]} />
                <Text style={[st.hostName, { color: t.text }]}>{h.name}</Text>
              </View>
              <Text style={[st.hostAddr, { color: t.textTertiary }]}>{h.host}:{h.port}</Text>
            </View>
            <TouchableOpacity onPress={() => handleRemoveHost(h.name)}>
              <Text style={{ color: t.danger, fontSize: 13 }}>Remove</Text>
            </TouchableOpacity>
          </View>
        ))}
        <TouchableOpacity
          style={[st.addBtn, { borderColor: t.border }]}
          onPress={() => router.push('/connect')}
        >
          <Text style={{ color: t.accent, fontSize: 13, fontWeight: '500' }}>+ Add Connection</Text>
        </TouchableOpacity>

        <Text style={[st.section, { color: t.textTertiary, marginTop: 32 }]}>About</Text>
        <View style={[st.infoRow, { borderBottomColor: t.border }]}>
          <Text style={{ color: t.textSecondary, fontSize: 13 }}>Client Version</Text>
          <Text style={{ color: t.textTertiary, fontSize: 13 }}>1.0.0</Text>
        </View>
        {hostVersion ? (
          <View style={[st.infoRow, { borderBottomColor: t.border }]}>
            <Text style={{ color: t.textSecondary, fontSize: 13 }}>Server Version</Text>
            <Text style={{ color: t.textTertiary, fontSize: 13 }}>{hostVersion}</Text>
          </View>
        ) : null}
        <View style={[st.infoRow, { borderBottomColor: t.border }]}>
          <Text style={{ color: t.textSecondary, fontSize: 13 }}>Connection</Text>
          <Text style={{ color: status === 'connected' ? t.accent : t.textTertiary, fontSize: 13 }}>{status}</Text>
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    root: { flex: 1 },
    header: { paddingHorizontal: 20, height: 52, justifyContent: 'center', borderBottomWidth: 1 },
    title: { fontSize: 16, fontWeight: '700' },
    content: { padding: 20 },
    section: { fontSize: 11, fontWeight: '600', textTransform: 'uppercase', letterSpacing: 0.8, marginBottom: 12 },
    hostRow: { flexDirection: 'row', alignItems: 'center', padding: 14, borderRadius: 12, borderWidth: 1, marginBottom: 8 },
    dot: { width: 7, height: 7, borderRadius: 4 },
    hostName: { fontSize: 14, fontWeight: '600' },
    hostAddr: { fontSize: 12, marginLeft: 15, marginTop: 2 },
    addBtn: { borderWidth: 1, borderStyle: 'dashed', borderRadius: 12, padding: 14, alignItems: 'center', marginTop: 4 },
    infoRow: { flexDirection: 'row', justifyContent: 'space-between', paddingVertical: 14, borderBottomWidth: 1 },
  });
