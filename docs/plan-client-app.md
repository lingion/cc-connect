# CC-Connect Client App — 设计方案

> 状态：**RFC（征求意见稿）**  
> 作者：cc-connect team  
> 日期：2026-04-18

---

## 1. 背景与目标

### 1.1 现状

CC-Connect 目前通过两种方式与用户交互：

1. **消息平台适配器**（飞书、Telegram、Discord 等）— 适合已有 IM 工具的团队
2. **Web Admin UI**（`cc-connect web`）— 适合配置管理和简单聊天

缺少一个**原生客户端**让用户能在手机、平板或桌面上直连本地 Agent，无需依赖第三方 IM。

### 1.2 竞品参考：Paseo

| 维度 | Paseo | CC-Connect（目标） |
|------|-------|-------------------|
| 核心定位 | 本地 Agent 统一 UI | IM 桥接 + 原生客户端双模式 |
| 客户端 | Expo (iOS/Android/Web) + Electron (桌面) | Expo (iOS/Android/Web) + Electron（后期） |
| 通信协议 | WebSocket `/ws` + JSON session 协议 | 复用现有 Bridge WebSocket 协议 |
| 远程访问 | 自建 Relay + E2EE (TweetNaCl) | Relay + E2EE（Phase 2） |
| 终端 | node-pty + xterm.js | 不做终端（定位不同） |
| 语音 | OpenAI/Sherpa STT + TTS | 复用现有 `[speech]` 配置 |
| Agent 支持 | Claude Code / Codex / OpenCode | 10+ Agent（已有优势） |
| IM 平台 | 无 | 12 平台（独有优势） |

### 1.3 目标

**Phase 1（MVP）：** 跨平台聊天客户端，通过 Bridge WebSocket 直连 cc-connect daemon，功能对齐 Paseo 的核心聊天体验。

**Phase 2：** 远程 Relay 中继 + E2EE，支持不在同一局域网时安全访问。

**Phase 3：** 桌面 Electron 壳、语音输入、文件浏览等高级功能。

---

## 2. 技术选型

### 2.1 客户端框架：Expo + React Native

**选择理由：**

- **一套代码，三端运行**：iOS、Android、Web（`expo export --platform web`）
- Paseo 同样选择了 Expo，验证了该方案在 AI Agent 客户端场景的可行性
- cc-connect Web Admin 已使用 React + Vite + Tailwind，团队有 React 经验
- Expo SDK 54+ 成熟稳定，OTA 更新、推送通知等开箱即用
- Web 导出可嵌入 Electron 做桌面版（Phase 3）

**替代方案对比：**

| 方案 | 优点 | 缺点 | 结论 |
|------|------|------|------|
| **Expo (React Native)** | 跨平台、生态丰富、与现有 Web 技术栈一致 | 性能略逊原生 | ✅ 选用 |
| Flutter | 性能好、UI 一致性强 | Dart 生态、团队无经验、与现有 React 不复用 | ❌ |
| SwiftUI + Kotlin | 原生性能最优 | 双端开发、维护成本翻倍 | ❌ |
| PWA (纯 Web) | 零安装 | 推送受限、无原生体验、不进应用商店 | ❌ 但 Web 版作为副产出 |

### 2.2 通信协议：复用 Bridge WebSocket

**核心思路：** 客户端作为一个 Bridge adapter 接入 cc-connect，复用现有 `core/bridge.go` 的全部能力。

```
┌──────────────────┐     WebSocket      ┌──────────────────┐
│   Client App     │ ◄──────────────── │   cc-connect     │
│  (Expo/RN)       │   /bridge/ws      │   daemon         │
│                  │   JSON 协议        │                  │
│  bridge adapter  │ ──────────────► │  BridgeServer    │
│  (built-in)      │                    │  → Engine → Agent│
└──────────────────┘                    └──────────────────┘
```

**已有协议能力（无需新增）：**

| 消息类型 | 方向 | 说明 |
|---------|------|------|
| `register` | C→S | 注册 adapter，声明 capabilities |
| `message` | C→S | 发送文本/图片/文件/音频消息 |
| `card_action` | C→S | 权限审批、按钮点击 |
| `reply` | S→C | 文本回复 |
| `card` | S→C | 卡片消息（Markdown、按钮、分割线等） |
| `buttons` | S→C | 内联按钮 |
| `preview_start` / `update_message` / `delete_message` | S→C | 流式进度（preview → update → finalize） |
| `typing_start` / `typing_stop` | S→C | 输入指示 |
| `audio` | S→C | TTS 音频 |
| `capabilities_snapshot` | S→C | 可用命令列表 |
| `ping` / `pong` | 双向 | 心跳 |

**需要扩展的协议：**

| 新增消息 | 方向 | 说明 |
|---------|------|------|
| `session_list` | S→C | 推送会话列表（替代 REST 轮询） |
| `session_switched` | S→C | 会话切换通知 |
| `history_sync` | S→C | 历史消息同步（首次连接或切换会话后） |
| `project_list` | S→C | 推送项目列表 |
| `status_update` | S→C | Agent 状态变更（idle/running/waiting_permission） |
| `connection_offer` | 双向 | Relay 配对握手（Phase 2） |

### 2.3 认证方式

**Phase 1（局域网）：**
- 复用 Bridge Server 的 `token` 认证（Bearer token / X-Bridge-Token / query param）
- 新增 QR 配对流程：daemon 展示 QR 码 → 客户端扫描获取 `{host, port, token}`
- `cc-connect web` 页面增加 "连接客户端" 入口，展示 QR 码和连接信息

**Phase 2（远程）：**
- Relay 中继服务器（可自建或使用官方托管）
- E2EE 端到端加密（NaCl/X25519 密钥交换 + XSalsa20-Poly1305）
- 连接 Offer 格式：`ccconnect://pair#offer=<base64url(json)>`

### 2.4 项目结构

```
cc-connect/
├── client/                      # 新增：客户端 monorepo
│   ├── package.json             # workspace root
│   ├── apps/
│   │   ├── mobile/              # Expo app (iOS/Android/Web)
│   │   │   ├── app.json
│   │   │   ├── src/
│   │   │   │   ├── app/         # expo-router 页面
│   │   │   │   ├── components/  # UI 组件
│   │   │   │   ├── hooks/       # 自定义 hooks
│   │   │   │   ├── lib/         # bridge client, store
│   │   │   │   └── i18n/        # 多语言
│   │   │   └── package.json
│   │   └── desktop/             # Electron shell (Phase 3)
│   │       └── package.json
│   └── packages/
│       ├── bridge-client/       # 共享 Bridge WebSocket 客户端
│       │   ├── src/
│       │   │   ├── client.ts    # DaemonClient 类
│       │   │   ├── protocol.ts  # 消息类型定义 (与 Go 对齐)
│       │   │   └── store.ts     # 状态管理 (Zustand)
│       │   └── package.json
│       └── ui/                  # 共享 UI 组件 (Phase 3)
│           └── package.json
├── core/
│   └── bridge.go                # 现有，需少量扩展
├── web/                         # 现有 Web Admin
└── ...
```

---

## 3. 功能规划

### Phase 1 — MVP 聊天客户端（4-6 周）

**核心页面：**

| 页面 | 功能 |
|------|------|
| **连接/配对** | 输入地址 + token，或扫描 QR 码连接 daemon |
| **项目列表** | 展示所有 project，显示 Agent 类型和状态 |
| **聊天** | 主交互界面，发送文本、接收 Markdown 回复 |
| **会话管理** | 新建/切换/删除会话 |
| **权限审批** | 卡片式权限请求，一键 Allow / Deny / Allow All |
| **设置** | 连接管理、语言切换、主题切换 |

**UI 交互：**

- Markdown 渲染（代码高亮、表格、列表）
- 流式输出动画（打字机效果 + 进度指示）
- 卡片渲染（权限请求、模型选择、会话列表等）
- 斜杠命令快捷输入（`/new`、`/model`、`/mode` 等，从 capabilities_snapshot 获取）
- 深色/浅色主题
- 多语言（复用 cc-connect 的 5 语言体系）

**对齐 Paseo 的关键体验：**

| Paseo 功能 | CC-Connect 实现方式 |
|-----------|-------------------|
| 多 Agent 聊天 | 多项目选择，每个项目绑定不同 Agent |
| 流式回复 | Bridge `preview_start` + `update_message` |
| 权限审批 | Bridge `card_action` + `perm:allow/deny` |
| 代码高亮 | react-native-markdown + highlight.js |
| 多设备同步 | 同一 Bridge token 多端连接，会话共享 |
| 图片发送 | Bridge `images[]` base64 |
| 语音输入 | Bridge `audio` + `[speech]` 配置 |

### Phase 2 — 远程访问与安全（2-3 周）

- Relay 中继服务器（Go 实现，可自建或托管）
- E2EE 端到端加密（X25519 密钥交换）
- 连接 Offer QR 码（含 relay endpoint + public key）
- 推送通知（Expo Push + 权限审批/任务完成提醒）
- 离线消息缓存

### Phase 3 — 高级功能（持续迭代）

- Electron 桌面壳（加载 Expo Web 导出）
- 语音对话模式（实时 STT → Agent → TTS）
- 文件浏览器（远程查看 work_dir 文件）
- 多 Host 管理（同时连接多台机器上的 cc-connect）
- Widget / 快捷方式（iOS Widget、Android 快捷方式）
- App Store / Google Play 发布

---

## 4. 服务端改动

### 4.1 Bridge 协议扩展

需要在 `core/bridge.go` 中新增以下出站消息类型：

```go
// 新增出站消息
type bridgeSessionList struct {
    Type       string              `json:"type"` // "session_list"
    SessionKey string              `json:"session_key"`
    Sessions   []bridgeSessionInfo `json:"sessions"`
    ActiveID   string              `json:"active_id"`
}

type bridgeSessionInfo struct {
    ID           string `json:"id"`
    Name         string `json:"name"`
    HistoryCount int    `json:"history_count"`
}

type bridgeStatusUpdate struct {
    Type       string `json:"type"` // "status_update"
    SessionKey string `json:"session_key"`
    Status     string `json:"status"` // "idle", "running", "waiting_permission"
    AgentType  string `json:"agent_type"`
}

type bridgeHistorySync struct {
    Type       string              `json:"type"` // "history_sync"
    SessionKey string              `json:"session_key"`
    SessionID  string              `json:"session_id"`
    History    []bridgeHistoryItem `json:"history"`
}

type bridgeHistoryItem struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}
```

### 4.2 QR 配对

在 Management API（Web Admin 后端）新增：

```
GET /api/v1/pair/qrcode   → 返回 { url, token, host, port } 供 QR 渲染
```

客户端扫描 QR 后自动填入连接信息。

### 4.3 推送通知支持（Phase 2）

新增 `register_push_token` 入站消息，daemon 存储 token，在权限请求/任务完成时发送推送。

---

## 5. 客户端核心模块设计

### 5.1 BridgeClient（packages/bridge-client）

```typescript
class BridgeClient {
  // 连接管理
  connect(url: string, token: string): Promise<void>
  disconnect(): void
  get connected(): boolean

  // 注册（连接后自动调用）
  private register(): void
  // capabilities: ["card", "buttons", "preview", "update_message",
  //                "delete_message", "typing", "audio", "reconstruct_reply"]

  // 发送
  sendMessage(sessionKey: string, content: string, images?: ImageData[], files?: FileData[]): void
  sendCardAction(sessionKey: string, action: string, replyCtx: string): void

  // 事件
  on(event: 'reply', handler: (msg: ReplyMessage) => void): void
  on(event: 'card', handler: (msg: CardMessage) => void): void
  on(event: 'preview_start', handler: (msg: PreviewMessage) => void): void
  on(event: 'update_message', handler: (msg: UpdateMessage) => void): void
  on(event: 'typing', handler: (typing: boolean) => void): void
  on(event: 'status', handler: (status: StatusUpdate) => void): void
  on(event: 'session_list', handler: (list: SessionList) => void): void
  on(event: 'capabilities', handler: (snapshot: CapabilitiesSnapshot) => void): void
  on(event: 'disconnect', handler: () => void): void

  // 心跳 & 重连
  private startPingLoop(): void
  private reconnect(): void
}
```

### 5.2 状态管理（Zustand）

```typescript
interface AppStore {
  // 连接
  connection: { host: string; token: string; status: 'disconnected' | 'connecting' | 'connected' }

  // 项目
  projects: Project[]
  activeProject: string | null

  // 会话
  sessions: Record<string, Session[]>     // sessionKey → sessions
  activeSessionId: Record<string, string> // sessionKey → active session id

  // 消息（按 session 分组）
  messages: Record<string, ChatMessage[]>

  // Agent 状态
  agentStatus: Record<string, 'idle' | 'running' | 'waiting_permission'>

  // 命令列表
  commands: Record<string, PublishedCommand[]> // project → commands
}
```

### 5.3 页面路由（expo-router）

```
/                           → 连接页（无已保存连接）或项目列表
/pair                       → QR 扫描配对
/settings                   → 连接管理、语言、主题
/project/[name]             → 项目主页（会话列表）
/project/[name]/chat        → 聊天界面
/project/[name]/sessions    → 会话管理
```

---

## 6. UI 设计原则

### 6.1 设计风格

- **深色优先**（AI/开发工具的标准视觉），支持浅色主题
- **紧凑信息密度**（代码/日志场景友好）
- 底部导航（项目列表 / 聊天 / 设置）
- 卡片式权限审批（醒目的 Allow / Deny 按钮）
- Mono 字体用于代码块、命令输出

### 6.2 核心 UI 组件

| 组件 | 说明 |
|------|------|
| `<ChatView>` | 消息列表 + Markdown 渲染 + 流式动画 |
| `<Composer>` | 输入框 + 斜杠命令选择器 + 附件按钮 |
| `<PermissionCard>` | 工具调用权限审批卡片 |
| `<SessionSwitcher>` | 会话列表抽屉 |
| `<ConnectionSetup>` | 连接配置 / QR 扫描 |
| `<StatusBar>` | Agent 状态指示（运行中/等待/空闲） |
| `<CommandPalette>` | 斜杠命令快速选择 |

---

## 7. 与 Paseo 的差异化优势

| 优势维度 | CC-Connect 客户端 | 说明 |
|---------|-------------------|------|
| **双模式接入** | IM + 原生客户端 | Paseo 只有原生客户端；我们两者兼得 |
| **Agent 数量** | 10+ Agent + ACP | Paseo 仅 3 种（Claude Code/Codex/OpenCode） |
| **零改造成本** | 复用 Bridge 协议 | 已有 bridge adapter 生态无需适配 |
| **IM 平台** | 12 平台 | Paseo 无 IM 平台支持 |
| **定时任务** | 聊天内 /cron | Paseo 的 schedules 需要 UI 配置 |
| **多项目** | 一个进程多项目 | 架构天然支持 |
| **开源协议** | MIT | Paseo 是 AGPL-3.0 |

---

## 8. 里程碑与排期

| 阶段 | 内容 | 预估时间 |
|------|------|---------|
| **M0** | 本文档 Review & 确认技术方案 | 1 周 |
| **M1** | bridge-client 包 + Expo 项目脚手架 | 1 周 |
| **M2** | 连接/配对页 + 项目列表页 | 1 周 |
| **M3** | 聊天核心：消息收发 + Markdown 渲染 + 流式 | 2 周 |
| **M4** | 权限审批 + 卡片渲染 + 斜杠命令 | 1 周 |
| **M5** | 会话管理 + 多语言 + 主题 + 打磨 | 1 周 |
| **M6** | TestFlight / 内测 APK 发布 | — |
| **P2** | Relay + E2EE + 推送通知 | 2-3 周 |
| **P3** | Electron + 语音 + 文件浏览 | 持续 |

---

## 9. 开放问题

1. **Relay 是否自建？** 还是先用第三方 TURN/STUN 做穿透？自建 Relay 成本如何？
2. **Web Admin 是否迁移到 Expo Web？** 长期看，Expo Web 导出是否能替代现有 Vite Web Admin？还是保持两套？
3. **推送通知的权限审批超时策略？** 用户未响应推送时，Agent 是自动 deny 还是保持等待？
4. **客户端应用商店名称？** "CC-Connect" 还是另取名？
5. **是否支持多 Host？** 手机上同时连接家里和公司的 cc-connect 实例？

---

## 10. 参考资料

- [Paseo 项目](https://paseo.sh) — 竞品参考
- [Expo 文档](https://docs.expo.dev) — 客户端框架
- [CC-Connect Bridge 协议](../core/bridge.go) — 现有 WebSocket 协议
- [CC-Connect Web Admin](../web/) — 现有 Web 前端
- [TweetNaCl](https://tweetnacl.js.org) — E2EE 加密库参考
