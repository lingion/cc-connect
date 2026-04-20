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
| 代码 Diff | 完整 split/unified diff viewer | Bridge 协议扩展支持（见下文） |
| 文件浏览 | 完整文件树 | Phase 2 |

### 1.3 目标

**Phase 1（MVP）：** 跨平台聊天客户端，通过 Bridge WebSocket 直连 cc-connect daemon，功能对齐 Paseo 的核心聊天体验。

**Phase 2：** 远程 Relay + E2EE、代码 Diff 查看、文件浏览。

**Phase 3：** 桌面 Electron 壳、语音输入等高级功能。

---

## 2. 技术选型

### 2.1 客户端框架：Expo + React Native

- **一套代码，三端运行**：iOS、Android、Web
- Paseo 同选 Expo，验证了该方案在 AI Agent 客户端场景的可行性
- cc-connect Web Admin 已使用 React，团队有经验
- Web 导出可嵌入 Electron 做桌面版（Phase 3）

### 2.2 UI 风格

**延续 Web Admin 的现有风格**（深色主题、Tailwind 色板、lucide 图标），同时参考 Paseo 的布局模式：

- Paseo：左侧 sidebar（host/项目/agent 列表）+ 中间 agent stream + 右侧 explorer（changes/files/PR）
- 我们：简化版 — 左侧会话/项目列表（手机上为底部 tab 或侧滑抽屉）+ 中间聊天流 + 按需展开 diff/文件面板

---

## 3. 数据存储策略：不用 SQLite

### 3.1 结论：AsyncStorage + 内存 Zustand，不引入 SQLite

**理由：**

Paseo 也不用 SQLite。它的方案是：
- **消息历史**：内存中（Zustand store），从 daemon 拉取 + 实时推送，不本地持久化
- **设置/布局/草稿**：AsyncStorage（轻量 key-value）
- **附件二进制**：IndexedDB（仅 Web 端）

我们的场景更简单——cc-connect daemon 已有完整的会话历史（`SessionStore` 的 `History` 字段），客户端只需做薄缓存。

### 3.2 各类数据的存储方式

| 数据类型 | 存储位置 | 说明 |
|---------|---------|------|
| **消息历史** | 内存 Zustand，数据源 = daemon | 连接时拉取最近 N 条，之后实时推送；断线重连时增量补齐 |
| **连接信息** | AsyncStorage `cc-hosts` | `{host, port, token, name}[]`，支持多 Host |
| **当前状态** | AsyncStorage `cc-active` | 上次活跃的 host/project/session，下次启动直接恢复 |
| **UI 偏好** | AsyncStorage `cc-prefs` | 主题、语言、字体大小、diff 显示模式 |
| **输入草稿** | AsyncStorage `cc-drafts` | 按 sessionKey 存未发送的输入框内容 |
| **附件缓存** | 临时目录 / IndexedDB | 图片/文件的发送缓存，发完即清 |

### 3.3 为什么不用 SQLite

| 考量 | SQLite | AsyncStorage + 内存 |
|------|--------|-------------------|
| 消息历史查询 | 可本地全文搜索 | daemon 侧搜索，客户端只做展示缓存 |
| 离线可用 | 可离线浏览历史 | 离线时无法与 Agent 交互，浏览历史意义不大 |
| 复杂度 | 需要 schema 迁移、ORM | 零额外依赖 |
| 包体积 | +1-2MB（expo-sqlite） | 0 |
| Paseo 验证 | Paseo 也不用 | 已验证方案可行 |

**例外：** 如果未来需要离线消息搜索或大量历史缓存，可以后期引入 SQLite 作为 L2 cache，不影响架构。

---

## 4. Bridge 协议分析：现有能力与缺口

### 4.1 现有协议已满足的能力

| 需求 | 现有消息类型 | 状态 |
|------|------------|------|
| 发送文本消息 | `message` (C→S) + `images[]` / `files[]` / `audio` | ✅ 完全满足 |
| 接收文本回复 | `reply` (S→C) | ✅ |
| 卡片（权限审批/模型选择等） | `card` (S→C) + `card_action` (C→S) | ✅ |
| 内联按钮 | `buttons` (S→C) | ✅ |
| 流式进度 | `preview_start` → `update_message` → `delete_message` | ✅ |
| 输入指示 | `typing_start` / `typing_stop` | ✅ |
| TTS 音频 | `audio` (S→C) | ✅ |
| 可用命令列表 | `capabilities_snapshot` (S→C) | ✅ |
| 心跳 | `ping` / `pong` | ✅ |
| REST 会话管理 | `GET/POST /bridge/sessions`, `POST /bridge/sessions/switch` | ✅ |
| Adapter 注册 | `register` + capabilities 声明 | ✅ |

### 4.2 需要扩展的协议（缺口）

#### 缺口 1：会话列表实时推送

**现状：** 只有 REST API（`GET /bridge/sessions?session_key=...`），客户端需要轮询。
**需要：** WebSocket 推送会话变更。

```
新增出站消息：session_list_update
{
  "type": "session_list_update",
  "session_key": "...",
  "sessions": [{ "id": "...", "name": "...", "history_count": 10 }],
  "active_id": "..."
}
```

**触发时机：** 新建/删除/切换会话时，向同 session_key 的所有 adapter 广播。

#### 缺口 2：Agent 运行状态推送

**现状：** 客户端无法知道 Agent 当前是空闲、运行中还是等待权限。
**需要：** 状态变更时推送。

```
新增出站消息：agent_status_update
{
  "type": "agent_status_update",
  "session_key": "...",
  "status": "running",       // idle | running | waiting_permission
  "agent_type": "claudecode",
  "project": "my-project"
}
```

**触发时机：** Agent session 创建/结束、权限请求/响应时。

#### 缺口 3：历史消息同步

**现状：** REST `GET /bridge/sessions/{id}?history_limit=50` 可以拉取，但无增量推送。
**需要：** 连接时 / 切换会话时自动推送历史，之后增量推送新消息。

```
新增出站消息：history_sync
{
  "type": "history_sync",
  "session_key": "...",
  "session_id": "...",
  "entries": [
    { "role": "user", "content": "...", "timestamp": 1713456789 },
    { "role": "assistant", "content": "...", "timestamp": 1713456800 }
  ],
  "has_older": true
}

新增入站消息：fetch_history
{
  "type": "fetch_history",
  "session_key": "...",
  "session_id": "...",
  "before_timestamp": 1713456789,
  "limit": 50
}
```

#### 缺口 4：项目列表

**现状：** 客户端不知道 daemon 上有哪些项目。
**需要：** 注册成功后推送项目列表。

```
capabilities_snapshot 已有 projects[] 字段，只需扩展内容：
{
  "type": "capabilities_snapshot",
  "projects": [
    {
      "project": "my-project",
      "agent_type": "claudecode",
      "work_dir": "/path/to/repo",
      "commands": [...],
      "status": "idle"
    }
  ]
}
```

#### 缺口 5：代码 Diff 订阅（Phase 2）

**现状：** Bridge 协议无 git diff 能力。
**需要：** 参考 Paseo 的 `subscribe_checkout_diff` 模式。

```
新增入站消息：subscribe_diff
{
  "type": "subscribe_diff",
  "session_key": "...",
  "project": "..."
}

新增出站消息：diff_update
{
  "type": "diff_update",
  "session_key": "...",
  "project": "...",
  "files": [
    {
      "path": "core/engine.go",
      "is_new": false,
      "is_deleted": false,
      "additions": 15,
      "deletions": 3,
      "status": "ok",
      "hunks": [
        {
          "old_start": 100, "old_count": 10,
          "new_start": 100, "new_count": 22,
          "lines": [
            { "type": "context", "content": "func foo() {" },
            { "type": "remove",  "content": "    old line" },
            { "type": "add",     "content": "    new line" }
          ]
        }
      ]
    }
  ]
}
```

**实现方式：** daemon 在 Agent 会话的 `work_dir` 上执行 `git diff HEAD`，解析为结构化数据，推送给订阅者。使用 debounce（~500ms）避免频繁推送。

### 4.3 协议扩展优先级

| 优先级 | 消息 | Phase |
|--------|------|-------|
| P0 | `history_sync` / `fetch_history` | Phase 1 |
| P0 | `agent_status_update` | Phase 1 |
| P0 | `session_list_update` | Phase 1 |
| P1 | `capabilities_snapshot` 扩展（project agent_type/work_dir） | Phase 1 |
| P2 | `subscribe_diff` / `diff_update` | Phase 2 |
| P2 | `file_explorer_request` / `file_explorer_response` | Phase 2 |
| P3 | `register_push_token` | Phase 2 |

---

## 5. UI 页面结构与导航

### 5.1 导航架构

**手机端：** 底部 Tab Bar + 侧滑抽屉

```
┌─────────────────────────────────┐
│  StatusBar (Agent 状态)          │
├─────────────────────────────────┤
│                                 │
│       主内容区域                  │
│  (聊天 / 项目列表 / 设置)         │
│                                 │
├─────────────────────────────────┤
│  ◉ 项目  │  💬 聊天  │  ⚙️ 设置  │  ← 底部 Tab Bar
└─────────────────────────────────┘
```

**平板/桌面 Web：** 左侧 Sidebar + 主内容

```
┌──────────┬──────────────────────────────────┐
│ Sidebar  │  主内容                            │
│          │                                    │
│ 项目列表  │  聊天消息流                         │
│ ────────│                                    │
│ 会话列表  │  [消息1]                           │
│  · chat1 │  [消息2]                           │
│  · chat2 │  [消息3 - 流式中...]                │
│  ────────│                                    │
│ 连接状态  │  ┌──────────────────────────┐      │
│          │  │ 输入框          📎 / ▶️   │      │
│ [+新会话] │  └──────────────────────────┘      │
└──────────┴──────────────────────────────────┘
```

### 5.2 底部 Tab Bar（手机端）

| Tab | 图标 | 页面 | 说明 |
|-----|------|------|------|
| **项目** | `FolderOpen` | 项目列表 | 展示所有 project，每个显示 Agent 类型 + 状态指示灯 |
| **聊天** | `MessageSquare` | 当前活跃聊天 | 主交互界面，顶部栏可切换会话 |
| **设置** | `Settings` | 设置页 | 连接管理、语言、主题、关于 |

### 5.3 各页面详细结构

#### 页面 1：连接/配对（首次使用）

```
┌────────────────────────┐
│     CC-Connect Logo    │
│                        │
│  ┌──────────────────┐  │
│  │ 输入连接地址      │  │
│  │ 192.168.1.100    │  │
│  │ ──────────────── │  │
│  │ 端口: 9810       │  │
│  │ ──────────────── │  │
│  │ Token: ••••••    │  │
│  └──────────────────┘  │
│                        │
│     [ 连接 ]           │
│                        │
│   ─── 或 ───           │
│                        │
│  [ 📷 扫码配对 ]       │
│                        │
│  已保存的连接:          │
│  · 🟢 家里 Mac         │
│  · 🔴 公司服务器        │
└────────────────────────┘
```

#### 页面 2：项目列表

```
┌────────────────────────┐
│ 项目                🔗 │  ← 🔗 = 连接状态指示
├────────────────────────┤
│ ┌────────────────────┐ │
│ │ 🟢 my-project      │ │  ← 绿点 = idle
│ │ Claude Code        │ │  ← Agent 类型
│ │ ~/code/my-repo     │ │  ← work_dir
│ │ 3 sessions         │ │
│ └────────────────────┘ │
│ ┌────────────────────┐ │
│ │ 🟡 data-analysis   │ │  ← 黄点 = running
│ │ Codex              │ │
│ │ ~/code/data        │ │
│ │ 1 session          │ │
│ └────────────────────┘ │
│ ┌────────────────────┐ │
│ │ 🔴 web-app         │ │  ← 红点 = waiting_permission
│ │ Gemini CLI         │ │
│ │ ~/code/web         │ │
│ │ 2 sessions         │ │
│ └────────────────────┘ │
└────────────────────────┘
```

点击进入项目 → 聊天页面。

#### 页面 3：聊天（核心页面）

```
┌───────────────────────────────┐
│ ◀ my-project    [会话▾] [···] │  ← 顶部导航
├───────────────────────────────┤  ← [会话▾] = 会话切换下拉
│                               │  ← [···] = 更多菜单
│  👤 帮我重构 UserService       │
│                               │
│  🤖 好的，我来分析一下...       │
│  ```go                        │
│  type UserService struct {    │
│      repo UserRepo            │
│  }                            │
│  ```                          │
│  ✅ 已修改 3 个文件             │
│                               │
│  ┌─────────────────────────┐  │
│  │ 🔐 权限请求               │  │  ← 权限审批卡片
│  │ Write: user_service.go   │  │
│  │                          │  │
│  │ [✅ Allow] [❌ Deny]      │  │
│  │ [✅ Allow All]            │  │
│  └─────────────────────────┘  │
│                               │
│  ⏳ Agent 运行中...            │  ← 状态指示
│                               │
├───────────────────────────────┤
│ /  │ 输入消息...       📎 ▶️  │  ← 输入框
└───────────────────────────────┘
    ↑ 斜杠触发命令面板
```

**会话切换下拉菜单：**

```
┌───────────────────┐
│ 会话列表           │
│ ─────────────── │
│ ● default         │  ← ● = 当前活跃
│   chat-2          │
│   refactor-task   │
│ ─────────────── │
│ [+ 新建会话]       │
└───────────────────┘
```

**更多菜单 [···]：**

```
┌───────────────────┐
│ /new 新建会话       │
│ /model 切换模型     │
│ /mode 权限模式      │
│ /dir 工作目录       │
│ /memory 记忆       │
│ ─────────────── │
│ 查看 Diff          │  ← Phase 2
│ 文件浏览           │  ← Phase 2
└───────────────────┘
```

**斜杠命令面板（输入 `/` 时弹出）：**

```
┌───────────────────────────┐
│ 🔍 搜索命令...              │
├───────────────────────────┤
│ /new      新建会话          │
│ /list     列出会话          │
│ /switch   切换会话          │
│ /model    切换模型          │
│ /mode     权限模式          │
│ /dir      工作目录          │
│ /cron     定时任务          │
│ /memory   Agent 记忆        │
│ /provider 切换 Provider    │
└───────────────────────────┘
```

命令列表从 `capabilities_snapshot.projects[].commands` 获取。

#### 页面 4：Diff 查看（Phase 2）

```
┌───────────────────────────────┐
│ ◀ 代码变更        unified ▾  │
├───────────────────────────────┤
│ core/engine.go  +15 -3        │
│ ┌─────────────────────────┐   │
│ │  100  100  func foo() { │   │
│ │  101     - old line      │   │  ← 红色背景
│ │       101 + new line     │   │  ← 绿色背景
│ │  102  102  }             │   │
│ └─────────────────────────┘   │
│                               │
│ core/i18n.go  +2 -0           │
│ ┌─────────────────────────┐   │
│ │  50   50   ...           │   │
│ │       51 + new key       │   │
│ │       52 + new value     │   │
│ └─────────────────────────┘   │
│                               │
│ 📊 2 files changed            │
│    +17 -3                     │
└───────────────────────────────┘
```

---

## 6. 代码 Diff 支持方案

### 6.1 Paseo 的做法

Paseo 通过 `CheckoutDiffManager` 在 daemon 侧 watch 文件变更，执行 `git diff`，解析成结构化数据（文件 → hunks → lines），推送给客户端。客户端用 `GitDiffPane` 组件渲染 split/unified 两种视图。

### 6.2 我们的实现方案

**服务端（Go）：** 在 Bridge 协议层新增 diff 订阅支持，复用 `work_dir` 下的 git 能力。

```go
// 执行 git diff 并解析为结构化数据
func computeWorkspaceDiff(workDir string) (*DiffResult, error) {
    // 1. exec: git diff HEAD --unified=3 --no-color
    // 2. 解析 unified diff 格式为 DiffFile → DiffHunk → DiffLine
    // 3. 标注 additions/deletions 统计
    // 4. 大文件标记为 "too_large"，二进制标记为 "binary"
}
```

**触发方式：**
- **被动拉取：** 客户端发送 `subscribe_diff`，daemon 立即计算并返回当前 diff
- **主动推送：** daemon 用 fsnotify watch `work_dir`，debounce 500ms 后推送 `diff_update`
- **Agent 完成时：** Agent session 结束后自动推送一次最新 diff

**客户端（React Native）：**
- 使用自定义 `<DiffView>` 组件，参考 Paseo 的 `GitDiffPane`
- 支持 unified / split 两种显示模式
- 代码高亮复用 highlight.js（Web Admin 已用）
- 文件折叠/展开、统计概览

### 6.3 对齐 Paseo 的能力

| Paseo Diff 功能 | 我们的实现 | Phase |
|----------------|-----------|-------|
| Working tree diff | `git diff HEAD` 推送 | Phase 2 |
| Structured hunks + lines | 服务端 Go 解析 | Phase 2 |
| Split / Unified 视图 | `<DiffView>` 组件 | Phase 2 |
| 实时文件监控 | fsnotify + debounce | Phase 2 |
| Syntax highlight tokens | highlight.js 客户端渲染 | Phase 2 |
| PR timeline | 暂不支持（非核心场景） | Phase 3+ |
| 文件浏览器 | `file_explorer` 协议 | Phase 2 |

---

## 7. 项目结构

```
cc-connect/
├── client/                       # 新增：客户端 monorepo
│   ├── package.json              # workspace root
│   ├── apps/
│   │   ├── mobile/               # Expo app (iOS/Android/Web)
│   │   │   ├── app.json
│   │   │   ├── src/
│   │   │   │   ├── app/          # expo-router 页面
│   │   │   │   │   ├── _layout.tsx
│   │   │   │   │   ├── index.tsx       # → 连接页或项目列表
│   │   │   │   │   ├── pair.tsx        # QR 扫描配对
│   │   │   │   │   ├── (tabs)/
│   │   │   │   │   │   ├── _layout.tsx # 底部 Tab Bar
│   │   │   │   │   │   ├── projects.tsx
│   │   │   │   │   │   ├── chat.tsx
│   │   │   │   │   │   └── settings.tsx
│   │   │   │   │   └── project/
│   │   │   │   │       ├── [name]/
│   │   │   │   │       │   ├── chat.tsx
│   │   │   │   │       │   ├── sessions.tsx
│   │   │   │   │       │   └── diff.tsx    # Phase 2
│   │   │   │   ├── components/
│   │   │   │   │   ├── ChatView.tsx        # 消息列表 + Markdown
│   │   │   │   │   ├── Composer.tsx        # 输入框 + 命令面板
│   │   │   │   │   ├── PermissionCard.tsx  # 权限审批卡片
│   │   │   │   │   ├── SessionSwitcher.tsx # 会话切换下拉
│   │   │   │   │   ├── CommandPalette.tsx  # 斜杠命令
│   │   │   │   │   ├── StatusBar.tsx       # Agent 状态
│   │   │   │   │   ├── DiffView.tsx        # 代码 Diff (Phase 2)
│   │   │   │   │   ├── ProjectCard.tsx     # 项目卡片
│   │   │   │   │   └── ConnectionSetup.tsx # 连接配置
│   │   │   │   ├── lib/
│   │   │   │   │   ├── bridge-client.ts    # WebSocket 客户端
│   │   │   │   │   ├── protocol.ts         # 消息类型 (对齐 Go)
│   │   │   │   │   └── markdown.ts         # Markdown 渲染配置
│   │   │   │   ├── store/
│   │   │   │   │   ├── connection.ts       # 连接状态
│   │   │   │   │   ├── projects.ts         # 项目列表
│   │   │   │   │   ├── sessions.ts         # 会话管理
│   │   │   │   │   ├── messages.ts         # 消息缓存
│   │   │   │   │   └── preferences.ts      # 用户偏好
│   │   │   │   ├── hooks/
│   │   │   │   │   ├── useBridge.ts        # Bridge 连接 hook
│   │   │   │   │   ├── useChat.ts          # 聊天消息 hook
│   │   │   │   │   └── useDiff.ts          # Diff 订阅 hook (Phase 2)
│   │   │   │   └── i18n/                   # 多语言 (5 语言)
│   │   │   └── package.json
│   │   └── desktop/                # Electron shell (Phase 3)
│   └── packages/                   # 未来可抽共享包
├── core/
│   └── bridge.go                   # 现有，需扩展
├── web/                            # 现有 Web Admin (保留)
└── ...
```

---

## 8. 服务端改动清单

### Phase 1 改动

| 文件 | 改动 | 工作量 |
|------|------|--------|
| `core/bridge.go` | 新增 `session_list_update` 出站消息 | 小 |
| `core/bridge.go` | 新增 `agent_status_update` 出站消息 | 小 |
| `core/bridge.go` | 新增 `history_sync` / `fetch_history` 消息对 | 中 |
| `core/bridge_capabilities.go` | 扩展 snapshot 加入 agent_type / work_dir / status | 小 |
| `core/engine.go` | 在会话变更时触发 `session_list_update` 广播 | 小 |
| `core/engine.go` | 在 Agent 状态变更时触发 `agent_status_update` | 中 |
| `core/management.go` | 新增 `GET /api/v1/pair/qrcode` 接口 | 小 |

### Phase 2 改动

| 文件 | 改动 | 工作量 |
|------|------|--------|
| `core/bridge.go` | 新增 `subscribe_diff` / `diff_update` 消息对 | 中 |
| `core/bridge_diff.go`（新） | diff 计算：`git diff` 执行 + unified diff 解析 | 大 |
| `core/bridge_diff.go`（新） | fsnotify watch + debounce 推送 | 中 |
| `core/bridge.go` | 新增 `file_explorer_request/response` | 中 |

---

## 9. 里程碑与排期

| 阶段 | 内容 | 预估 |
|------|------|------|
| **M0** | 方案 Review 确认 | 1 周 |
| **M1** | Expo 脚手架 + bridge-client + 连接/配对页 | 1 周 |
| **M2** | 项目列表页 + 服务端 capabilities_snapshot 扩展 | 1 周 |
| **M3** | 聊天核心：消息收发 + Markdown + 流式 + 状态推送 | 2 周 |
| **M4** | 权限审批卡片 + 斜杠命令面板 + 会话管理 | 1 周 |
| **M5** | 多语言 + 主题 + 多 Host + 打磨 | 1 周 |
| **M6** | TestFlight / 内测 APK | — |
| **P2-1** | Diff 服务端（git diff 解析 + fsnotify watch） | 1-2 周 |
| **P2-2** | Diff 客户端（DiffView 组件 + 订阅） | 1 周 |
| **P2-3** | Relay + E2EE + 推送通知 | 2-3 周 |
| **P3** | Electron + 语音 + 文件浏览 | 持续 |

---

## 10. 与 Paseo 的差异化优势

| 维度 | CC-Connect | Paseo |
|------|-----------|-------|
| **双模式** | IM 平台 + 原生客户端 | 仅原生客户端 |
| **Agent 数量** | 10+ Agent + ACP | 3 种 |
| **IM 平台** | 12 平台 | 无 |
| **定时任务** | /cron 自然语言 | UI 配置 |
| **多项目** | 一进程多项目 | 一 daemon 多 workspace |
| **开源协议** | MIT | AGPL-3.0 |
| **代码 Diff** | Phase 2 支持 | 已有 |
| **终端** | 不做（定位不同） | 有 |

---

## 11. 开放问题

1. ~~需不需要 SQLite？~~ → **不需要**，AsyncStorage + 内存 Zustand
2. **Web Admin 是否长期迁移到 Expo Web？** 还是保持两套（Admin 管配置，Client 管聊天）？
3. **Relay 自建 vs 第三方穿透？** 自建成本如何评估？
4. **推送通知超时策略？** 用户未响应时 Agent 行为？
5. **Diff 的 fsnotify 性能：** 大仓库 watch 是否有性能问题？是否需要 `.gitignore` 过滤？
6. **多 Host 管理的 UX：** 手机上如何优雅切换多台机器？

---

## 12. 参考

- [Paseo](https://paseo.sh) — 竞品
- [Expo 文档](https://docs.expo.dev)
- [CC-Connect Bridge 协议](../core/bridge.go)
- [CC-Connect Web Admin](../web/)
