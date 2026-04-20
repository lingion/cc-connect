import { Tabs } from 'expo-router';
import { useTheme } from '@/lib/theme';
import { Text } from 'react-native';

function TabIcon({ label, focused, color }: { label: string; focused: boolean; color: string }) {
  const icons: Record<string, string> = { Projects: '📂', Chat: '💬', Settings: '⚙' };
  return <Text style={{ fontSize: 18, opacity: focused ? 1 : 0.5 }}>{icons[label] || '•'}</Text>;
}

export default function TabLayout() {
  const t = useTheme();

  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarStyle: {
          backgroundColor: t.bgCard,
          borderTopColor: t.border,
          borderTopWidth: 1,
        },
        tabBarActiveTintColor: t.accent,
        tabBarInactiveTintColor: t.textTertiary,
        tabBarLabelStyle: { fontSize: 11, fontWeight: '500' },
      }}
    >
      <Tabs.Screen
        name="projects"
        options={{
          title: 'Projects',
          tabBarIcon: ({ focused, color }) => <TabIcon label="Projects" focused={focused} color={color} />,
        }}
      />
      <Tabs.Screen
        name="chat"
        options={{
          title: 'Chat',
          tabBarIcon: ({ focused, color }) => <TabIcon label="Chat" focused={focused} color={color} />,
        }}
      />
      <Tabs.Screen
        name="settings"
        options={{
          title: 'Settings',
          tabBarIcon: ({ focused, color }) => <TabIcon label="Settings" focused={focused} color={color} />,
        }}
      />
    </Tabs>
  );
}
