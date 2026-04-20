import { useEffect } from 'react';
import { Stack } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useColorScheme } from 'react-native';
import { useConnectionStore } from '@/store/connection';

export default function RootLayout() {
  const scheme = useColorScheme();
  const loadHosts = useConnectionStore(s => s.loadHosts);

  useEffect(() => { loadHosts(); }, []);

  return (
    <>
      <StatusBar style={scheme === 'light' ? 'dark' : 'light'} />
      <Stack screenOptions={{ headerShown: false }}>
        <Stack.Screen name="(tabs)" />
        <Stack.Screen name="connect" options={{ presentation: 'modal' }} />
      </Stack>
    </>
  );
}
