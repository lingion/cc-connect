import { useEffect, useRef, useCallback, useState } from 'react';
import { View, Text, TextInput, TouchableOpacity, FlatList, StyleSheet, KeyboardAvoidingView, Platform } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useTheme } from '@/lib/theme';
import { useProjectsStore } from '@/store/projects';
import { useSessionsStore } from '@/store/sessions';
import { useMessagesStore, type ChatMessage } from '@/store/messages';
import { useConnectionStore } from '@/store/connection';
import { getGlobalClient } from './projects';
import type { BridgeIncoming, ReplyMsg, CardMsg, ButtonsMsg, PreviewStartMsg, UpdateMessageMsg, DeleteMessageMsg, HistorySync } from '@/lib/protocol';

export default function ChatScreen() {
  const t = useTheme();
  const activeProject = useProjectsStore(s => s.activeProject);
  const project = useProjectsStore(s => s.projects.find(p => p.project === s.activeProject));
  const sessions = useSessionsStore(s => s.sessions[activeProject || ''] || []);
  const activeSessionId = useSessionsStore(s => s.activeIds[activeProject || '']);
  const channelKey = activeProject && activeSessionId ? `${activeProject}:${activeSessionId}` : '';
  const messages = useMessagesStore(s => channelKey ? (s.channels[channelKey] || []) : []);
  const typing = useMessagesStore(s => channelKey ? s.typing[channelKey] : false);
  const addMessage = useMessagesStore(s => s.addMessage);
  const updateStreaming = useMessagesStore(s => s.updateStreamingMessage);
  const finishStreaming = useMessagesStore(s => s.finishStreaming);
  const removeStreaming = useMessagesStore(s => s.removeStreamingMessage);
  const setHistory = useMessagesStore(s => s.setHistory);
  const setTyping = useMessagesStore(s => s.setTyping);
  const status = useConnectionStore(s => s.status);

  const [input, setInput] = useState('');
  const [showCommands, setShowCommands] = useState(false);
  const flatListRef = useRef<FlatList>(null);
  const previewRef = useRef<string | null>(null);

  const sessionKey = activeProject ? `cc-client:client-user:${activeProject}` : '';

  // Handle incoming bridge messages for this chat
  useEffect(() => {
    const client = getGlobalClient();
    if (!client) return;

    const originalOnMessage = (client as any).opts?.onMessage;
    const wrappedHandler = (msg: BridgeIncoming) => {
      originalOnMessage?.(msg);
      if (!channelKey) return;

      switch (msg.type) {
        case 'reply': {
          const r = msg as ReplyMsg;
          finishStreaming(channelKey);
          addMessage(channelKey, {
            id: `reply-${Date.now()}`,
            role: 'assistant',
            content: r.content,
            timestamp: Date.now() / 1000,
          });
          break;
        }
        case 'card': {
          const c = msg as CardMsg;
          addMessage(channelKey, {
            id: `card-${Date.now()}`,
            role: 'assistant',
            content: c.card.header?.title || '',
            format: 'card',
            card: c.card,
            timestamp: Date.now() / 1000,
          });
          break;
        }
        case 'buttons': {
          const b = msg as ButtonsMsg;
          addMessage(channelKey, {
            id: `btns-${Date.now()}`,
            role: 'assistant',
            content: b.content,
            format: 'buttons',
            buttons: b.buttons,
            timestamp: Date.now() / 1000,
          });
          break;
        }
        case 'preview_start': {
          const ps = msg as PreviewStartMsg;
          previewRef.current = ps.ref_id;
          addMessage(channelKey, {
            id: `stream-${Date.now()}`,
            role: 'assistant',
            content: ps.content,
            streaming: true,
            timestamp: Date.now() / 1000,
          });
          client.send({ type: 'preview_ack', ref_id: ps.ref_id, preview_handle: ps.reply_ctx || ps.ref_id });
          break;
        }
        case 'update_message': {
          const um = msg as UpdateMessageMsg;
          updateStreaming(channelKey, um.content);
          break;
        }
        case 'delete_message': {
          removeStreaming(channelKey);
          break;
        }
        case 'typing_start':
          setTyping(channelKey, true);
          break;
        case 'typing_stop':
          setTyping(channelKey, false);
          break;
        case 'history_sync': {
          const hs = msg as HistorySync;
          const hk = `${hs.project}:${hs.session_id}`;
          if (hk === channelKey) {
            setHistory(hk, hs.entries);
          }
          break;
        }
      }
    };

    if ((client as any).opts) {
      (client as any).opts.onMessage = wrappedHandler;
    }

    return () => {
      if ((client as any).opts) {
        (client as any).opts.onMessage = originalOnMessage;
      }
    };
  }, [channelKey]);

  // Auto-scroll
  useEffect(() => {
    if (messages.length > 0) {
      setTimeout(() => flatListRef.current?.scrollToEnd({ animated: true }), 100);
    }
  }, [messages.length]);

  const handleSend = () => {
    const text = input.trim();
    if (!text || !sessionKey) return;
    setInput('');
    setShowCommands(false);

    addMessage(channelKey, {
      id: `user-${Date.now()}`,
      role: 'user',
      content: text,
      timestamp: Date.now() / 1000,
    });

    const client = getGlobalClient();
    client?.sendMessage(sessionKey, text, activeProject || undefined);
  };

  const handleCardAction = (action: string) => {
    const client = getGlobalClient();
    client?.sendCardAction(sessionKey, action, activeProject || undefined);
  };

  const handleInputChange = (text: string) => {
    setInput(text);
    setShowCommands(text === '/');
  };

  const handleCommandSelect = (cmd: string) => {
    setInput(cmd + ' ');
    setShowCommands(false);
  };

  const st = styles(t);

  const renderMessage = ({ item }: { item: ChatMessage }) => {
    const isUser = item.role === 'user';
    return (
      <View style={st.msgRow}>
        <View style={[st.avatar, isUser ? { backgroundColor: t.bgBadge } : { backgroundColor: `${t.accent}18` }]}>
          <Text style={[st.avatarText, !isUser && { color: t.accent }]}>
            {isUser ? 'Y' : 'A'}
          </Text>
        </View>
        <View style={st.msgBody}>
          <View style={st.msgHead}>
            <Text style={[st.msgName, { color: t.text }]}>{isUser ? 'You' : (project?.agent_type || 'Agent')}</Text>
          </View>

          {item.format === 'card' && item.card ? (
            <CardBlock card={item.card} theme={t} onAction={handleCardAction} />
          ) : item.format === 'buttons' && item.buttons ? (
            <View>
              <Text style={[st.msgText, { color: t.text }]}>{item.content}</Text>
              <View style={st.buttonsRow}>
                {item.buttons.flat().map((b, i) => (
                  <TouchableOpacity
                    key={i}
                    style={[st.actionBtn, { backgroundColor: t.bgBadge, borderColor: t.border }]}
                    onPress={() => handleCardAction(b.data)}
                  >
                    <Text style={[st.actionBtnText, { color: t.textSecondary }]}>{b.text}</Text>
                  </TouchableOpacity>
                ))}
              </View>
            </View>
          ) : (
            <Text style={[st.msgText, { color: t.text }]}>
              {item.content}
              {item.streaming && <Text style={{ color: t.accent }}> ▌</Text>}
            </Text>
          )}
        </View>
      </View>
    );
  };

  return (
    <SafeAreaView style={[st.root, { backgroundColor: t.bg }]} edges={['top']}>
      {/* Top bar */}
      <View style={[st.topbar, { borderBottomColor: t.border }]}>
        <View style={st.topLeft}>
          <Text style={[st.topTitle, { color: t.text }]}>{activeProject || 'No Project'}</Text>
          {project?.agent_type && (
            <View style={[st.agentBadge, { backgroundColor: `${t.accent}18`, borderColor: `${t.accent}33` }]}>
              <Text style={[st.agentBadgeText, { color: t.accent }]}>{project.agent_type}</Text>
            </View>
          )}
        </View>
        {project?.status === 'running' && (
          <View style={st.statusRow}>
            <View style={[st.pulseDot, { backgroundColor: t.accent }]} />
            <Text style={{ color: t.textTertiary, fontSize: 11 }}>Running</Text>
          </View>
        )}
      </View>

      {/* Messages */}
      <FlatList
        ref={flatListRef}
        data={messages}
        keyExtractor={m => m.id}
        renderItem={renderMessage}
        contentContainerStyle={st.messages}
        ListEmptyComponent={
          <View style={st.emptyChat}>
            <Text style={{ color: t.textQuaternary, fontSize: 13 }}>
              {status === 'connected' ? 'Send a message to start' : 'Connecting...'}
            </Text>
          </View>
        }
      />

      {typing && (
        <View style={st.typingBar}>
          <Text style={{ color: t.textTertiary, fontSize: 12 }}>Agent is typing...</Text>
        </View>
      )}

      {/* Command palette */}
      {showCommands && project?.commands && (
        <View style={[st.cmdPalette, { backgroundColor: t.bgCard, borderColor: t.border }]}>
          {project.commands.slice(0, 8).map(cmd => (
            <TouchableOpacity key={cmd.name} style={st.cmdItem} onPress={() => handleCommandSelect(`/${cmd.name}`)}>
              <Text style={[st.cmdName, { color: t.accent }]}>/{cmd.name}</Text>
              <Text style={[st.cmdDesc, { color: t.textTertiary }]} numberOfLines={1}>{cmd.description}</Text>
            </TouchableOpacity>
          ))}
        </View>
      )}

      {/* Composer */}
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <View style={[st.composer, { borderTopColor: t.border }]}>
          <View style={[st.composerBox, { backgroundColor: t.bgInput, borderColor: t.border }]}>
            <TextInput
              style={[st.composerInput, { color: t.text }]}
              value={input}
              onChangeText={handleInputChange}
              placeholder="Send a message..."
              placeholderTextColor={t.textQuaternary}
              multiline
              returnKeyType="send"
              blurOnSubmit
              onSubmitEditing={handleSend}
            />
            <TouchableOpacity
              style={[st.sendBtn, { opacity: input.trim() ? 1 : 0.3 }]}
              onPress={handleSend}
              disabled={!input.trim()}
            >
              <Text style={{ color: t.accent, fontSize: 18 }}>▶</Text>
            </TouchableOpacity>
          </View>
        </View>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

function CardBlock({ card, theme: t, onAction }: { card: any; theme: ReturnType<typeof useTheme>; onAction: (v: string) => void }) {
  return (
    <View style={{ backgroundColor: t.bgCard, borderRadius: 10, borderWidth: 1, borderColor: t.border, padding: 14, gap: 10 }}>
      {card.header?.title && (
        <Text style={{ fontSize: 13, fontWeight: '600', color: t.text }}>{card.header.title}</Text>
      )}
      {card.elements?.map((el: any, i: number) => {
        if (el.type === 'markdown') return <Text key={i} style={{ fontSize: 13, color: t.text, lineHeight: 20 }}>{el.content}</Text>;
        if (el.type === 'divider') return <View key={i} style={{ height: 1, backgroundColor: t.border }} />;
        if (el.type === 'note') return <Text key={i} style={{ fontSize: 11, color: t.textTertiary }}>{el.text}</Text>;
        if (el.type === 'actions') {
          return (
            <View key={i} style={{ flexDirection: 'row', flexWrap: 'wrap', gap: 6 }}>
              {el.buttons?.map((btn: any, j: number) => (
                <TouchableOpacity
                  key={j}
                  style={{
                    paddingHorizontal: 12, paddingVertical: 6, borderRadius: 8,
                    backgroundColor: btn.btn_type === 'primary' ? t.accent : t.bgBadge,
                    borderWidth: 1, borderColor: btn.btn_type === 'primary' ? t.accent : t.border,
                  }}
                  onPress={() => onAction(btn.value)}
                >
                  <Text style={{
                    fontSize: 12, fontWeight: '500',
                    color: btn.btn_type === 'primary' ? '#000' : t.textSecondary,
                  }}>{btn.text}</Text>
                </TouchableOpacity>
              ))}
            </View>
          );
        }
        return null;
      })}
    </View>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    root: { flex: 1 },
    topbar: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', paddingHorizontal: 20, height: 48, borderBottomWidth: 1 },
    topLeft: { flexDirection: 'row', alignItems: 'center', gap: 8 },
    topTitle: { fontSize: 14, fontWeight: '600' },
    agentBadge: { paddingHorizontal: 7, paddingVertical: 2, borderRadius: 6, borderWidth: 1 },
    agentBadgeText: { fontSize: 10, fontWeight: '500' },
    statusRow: { flexDirection: 'row', alignItems: 'center', gap: 5 },
    pulseDot: { width: 5, height: 5, borderRadius: 3 },
    messages: { padding: 16, paddingBottom: 8 },
    emptyChat: { alignItems: 'center', paddingTop: 100 },
    msgRow: { flexDirection: 'row', gap: 10, marginBottom: 18 },
    avatar: { width: 26, height: 26, borderRadius: 8, alignItems: 'center', justifyContent: 'center', marginTop: 1 },
    avatarText: { fontSize: 11, fontWeight: '600', color: t.textSecondary },
    msgBody: { flex: 1 },
    msgHead: { flexDirection: 'row', alignItems: 'baseline', gap: 6, marginBottom: 3 },
    msgName: { fontSize: 12, fontWeight: '600' },
    msgText: { fontSize: 13.5, lineHeight: 21 },
    buttonsRow: { flexDirection: 'row', flexWrap: 'wrap', gap: 6, marginTop: 8 },
    actionBtn: { paddingHorizontal: 12, paddingVertical: 6, borderRadius: 8, borderWidth: 1 },
    actionBtnText: { fontSize: 12, fontWeight: '500' },
    typingBar: { paddingHorizontal: 20, paddingVertical: 4 },
    cmdPalette: { marginHorizontal: 16, marginBottom: 4, borderRadius: 10, borderWidth: 1, overflow: 'hidden' },
    cmdItem: { flexDirection: 'row', alignItems: 'center', gap: 10, paddingHorizontal: 14, paddingVertical: 9, borderBottomWidth: 1, borderBottomColor: t.borderSubtle },
    cmdName: { fontSize: 12, fontWeight: '600', fontFamily: 'monospace', width: 80 },
    cmdDesc: { fontSize: 12, flex: 1 },
    composer: { paddingHorizontal: 16, paddingVertical: 10, borderTopWidth: 1 },
    composerBox: { flexDirection: 'row', alignItems: 'flex-end', borderRadius: 12, borderWidth: 1, paddingHorizontal: 12, paddingVertical: 8 },
    composerInput: { flex: 1, fontSize: 13, maxHeight: 100, lineHeight: 20 },
    sendBtn: { width: 30, height: 30, alignItems: 'center', justifyContent: 'center' },
  });
