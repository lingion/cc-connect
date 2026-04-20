import { Redirect } from 'expo-router';
import { useConnectionStore } from '@/store/connection';

export default function Index() {
  const hosts = useConnectionStore(s => s.hosts);
  if (hosts.length === 0) return <Redirect href="/connect" />;
  return <Redirect href="/(tabs)/projects" />;
}
