import { useState } from 'react';
import { View, Text, TextInput, TouchableOpacity, ScrollView, StyleSheet, KeyboardAvoidingView, Platform } from 'react-native';
import { useRouter } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useTheme } from '@/lib/theme';
import { useConnectionStore } from '@/store/connection';
import type { BridgeHost } from '@/lib/protocol';

export default function ConnectScreen() {
  const t = useTheme();
  const router = useRouter();
  const { hosts, addHost, setActiveHost } = useConnectionStore();

  const [name, setName] = useState('');
  const [host, setHost] = useState('');
  const [port, setPort] = useState('9810');
  const [token, setToken] = useState('');

  const canConnect = name.trim() && host.trim() && port.trim();

  const handleConnect = async () => {
    if (!canConnect) return;
    const h: BridgeHost = {
      name: name.trim(),
      host: host.trim(),
      port: parseInt(port, 10) || 9810,
      token: token.trim(),
    };
    await addHost(h);
    router.replace('/(tabs)/projects');
  };

  const handleSelectHost = async (h: BridgeHost) => {
    await setActiveHost(h.name);
    router.replace('/(tabs)/projects');
  };

  const s = styles(t);
  return (
    <SafeAreaView style={[s.root, { backgroundColor: t.bg }]}>
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={{ flex: 1 }}>
        <ScrollView contentContainerStyle={s.scroll} keyboardShouldPersistTaps="handled">
          <Text style={[s.brand, { color: t.text }]}>
            CC<Text style={{ color: t.accent }}>-</Text>Connect
          </Text>
          <Text style={[s.subtitle, { color: t.textSecondary }]}>Connect to your daemon</Text>

          <View style={s.form}>
            <View style={s.field}>
              <Text style={[s.label, { color: t.textSecondary }]}>Name</Text>
              <TextInput
                style={[s.input, { backgroundColor: t.bgInput, borderColor: t.border, color: t.text }]}
                value={name}
                onChangeText={setName}
                placeholder="My Mac"
                placeholderTextColor={t.textQuaternary}
                autoCapitalize="none"
              />
            </View>
            <View style={s.field}>
              <Text style={[s.label, { color: t.textSecondary }]}>Host</Text>
              <TextInput
                style={[s.input, { backgroundColor: t.bgInput, borderColor: t.border, color: t.text }]}
                value={host}
                onChangeText={setHost}
                placeholder="192.168.1.100"
                placeholderTextColor={t.textQuaternary}
                autoCapitalize="none"
                keyboardType="url"
              />
            </View>
            <View style={s.row}>
              <View style={[s.field, { flex: 1 }]}>
                <Text style={[s.label, { color: t.textSecondary }]}>Port</Text>
                <TextInput
                  style={[s.input, { backgroundColor: t.bgInput, borderColor: t.border, color: t.text }]}
                  value={port}
                  onChangeText={setPort}
                  placeholder="9810"
                  placeholderTextColor={t.textQuaternary}
                  keyboardType="numeric"
                />
              </View>
              <View style={[s.field, { flex: 2, marginLeft: 12 }]}>
                <Text style={[s.label, { color: t.textSecondary }]}>Token</Text>
                <TextInput
                  style={[s.input, { backgroundColor: t.bgInput, borderColor: t.border, color: t.text }]}
                  value={token}
                  onChangeText={setToken}
                  placeholder="Optional"
                  placeholderTextColor={t.textQuaternary}
                  autoCapitalize="none"
                  secureTextEntry
                />
              </View>
            </View>

            <TouchableOpacity
              style={[s.btn, { backgroundColor: t.accent, opacity: canConnect ? 1 : 0.4 }]}
              onPress={handleConnect}
              disabled={!canConnect}
              activeOpacity={0.7}
            >
              <Text style={s.btnText}>Connect</Text>
            </TouchableOpacity>
          </View>

          {hosts.length > 0 && (
            <View style={s.saved}>
              <Text style={[s.savedTitle, { color: t.textTertiary }]}>Saved Connections</Text>
              {hosts.map(h => (
                <TouchableOpacity
                  key={h.name}
                  style={[s.hostItem, { backgroundColor: t.bgCard, borderColor: t.border }]}
                  onPress={() => handleSelectHost(h)}
                  activeOpacity={0.7}
                >
                  <View style={[s.dot, { backgroundColor: t.accent }]} />
                  <View style={{ flex: 1 }}>
                    <Text style={[s.hostName, { color: t.text }]}>{h.name}</Text>
                    <Text style={[s.hostAddr, { color: t.textTertiary }]}>{h.host}:{h.port}</Text>
                  </View>
                  <Text style={{ color: t.textQuaternary, fontSize: 18 }}>›</Text>
                </TouchableOpacity>
              ))}
            </View>
          )}
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    root: { flex: 1 },
    scroll: { padding: 24, paddingTop: 60 },
    brand: { fontSize: 28, fontWeight: '700', letterSpacing: -0.5, textAlign: 'center' },
    subtitle: { fontSize: 14, textAlign: 'center', marginTop: 6, marginBottom: 36 },
    form: { gap: 16 },
    field: { gap: 6 },
    label: { fontSize: 12, fontWeight: '600' },
    input: { height: 44, borderRadius: 10, borderWidth: 1, paddingHorizontal: 14, fontSize: 14 },
    row: { flexDirection: 'row' },
    btn: { height: 46, borderRadius: 10, alignItems: 'center', justifyContent: 'center', marginTop: 8 },
    btnText: { color: '#000', fontSize: 14, fontWeight: '600' },
    saved: { marginTop: 40 },
    savedTitle: { fontSize: 11, fontWeight: '600', textTransform: 'uppercase', letterSpacing: 0.8, marginBottom: 12 },
    hostItem: { flexDirection: 'row', alignItems: 'center', gap: 12, padding: 14, borderRadius: 12, borderWidth: 1, marginBottom: 8 },
    dot: { width: 8, height: 8, borderRadius: 4 },
    hostName: { fontSize: 14, fontWeight: '600' },
    hostAddr: { fontSize: 12, marginTop: 1 },
  });
