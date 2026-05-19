# Fixture Collection Guide

## 什么是 Fixture，为什么要采集

`tests/blackbox/fixtures/` 中的 JSON 文件是从真实平台（飞书、企微、Telegram 等）
捕获的 **真实消息快照**（脱敏后）。它们用于：

1. **平台解析回归测试**：升级飞书/企微 SDK 后，把 fixture 重新跑一遍，
   立刻能发现解析行为有没有变化。
2. **离线开发**：不需要打开飞书 App 或配置 webhook，直接在本地回放真实消息格式。
3. **CI 集成**：`platform_sim` 测试不调用真实 API，所以 CI 中可以直接运行（不需要 API key）。

---

## 本地环境说明

| 服务 | 启动方式 | 配置文件 |
|------|---------|---------|
| cc-connect | `supervisor` 托管 | `~/.cc-connect/config.toml` |
| 状态检查 | `sudo supervisorctl status cc-connect` | |

已配置的平台：

| 平台 | 项目 | 说明 |
|------|------|------|
| 飞书 | pm / ceo / qa-release / dev-claude / growth-writer 等 | 内部办公平台，消息最丰富 |
| 企微 (wecom) | pm | WebSocket 模式 |
| Telegram | king-dev | 需要代理 `http://127.0.0.1:7890` |
| Discord | king-dev | 服务器 ID 1477201497425973391 |
| 微博 | pm | DM |

---

## 第一步：启用采集器

采集器通过环境变量激活，**不需要修改代码**。
只需在 supervisor 环境中加上 `CC_CONNECT_RECORD_FIXTURES`：

```bash
# 1. 创建采集目录
mkdir -p /tmp/cc-fixtures

# 2. 临时重启 cc-connect 并注入环境变量（两种方式选一）

# 方式 A：直接带 env 前缀启动（不影响 supervisor，适合临时采集）
CC_CONNECT_RECORD_FIXTURES=/tmp/cc-fixtures \
  /usr/local/bin/cc-connect start --config ~/.cc-connect/config.toml &

# 方式 B：通过 supervisor 环境注入（更稳定，需要 sudoer）
sudo bash -c 'cat >> /etc/supervisor/conf.d/cc-connect.conf << EOF
environment=CC_CONNECT_RECORD_FIXTURES="/tmp/cc-fixtures"
EOF'
sudo supervisorctl reread
sudo supervisorctl update
sudo supervisorctl restart cc-connect
```

确认服务启动后看到日志 `"collector: fixture saved"` 说明采集已就绪。

```bash
# 实时监控采集进度
tail -f /var/log/supervisor/cc-connect.log | grep collector
```

---

## 第二步：在各平台触发消息

### 飞书（Feishu）— 优先级最高

**建议使用项目：`pm`（飞书 app_id: `cli_a928e232e0399bd6`）**

打开飞书，找到该 Bot 的对话，按顺序发送以下消息：

| 操作 | 发送内容 | 采集类型 | 说明 |
|------|---------|---------|------|
| 1 | 纯文字：`你好，这是采集测试` | `text` | 私聊文本 |
| 2 | 纯文字：`tell me a joke` | `text` | 英文指令 |
| 3 | 发送一张截图（任意图片） | `image` | 图片消息 |
| 4 | 发送一个 `.txt` 或 `.pdf` 文件 | `file` | 文件消息 |
| 5 | 发一条语音消息 | `audio` | 语音（可选） |

**群聊采集（验证 @ 提及场景）：**

1. 找一个有该 Bot 的群聊
2. @Bot 发：`@Bot 帮我总结一下`
3. 发一张图片（不 @，只是群里发图）

> 每次发消息后等待 2~3 秒，让 cc-connect 处理完毕并将 fixture 写入磁盘。

**验证采集结果：**

```bash
ls -la /tmp/cc-fixtures/feishu/
# 应该看到类似：
# 20260519_143201_001_text_xxx.json
# 20260519_143210_002_text_xxx.json
# 20260519_143225_003_image_xxx.json
# 20260519_143240_004_file_xxx.json
```

---

### 企微（WeCom）— 项目 `pm`

Bot ID: `aibw29UAZ-2L8pvxwLdJxnwRcootp8Nblbk`，在企微找到此机器人：

| 操作 | 发送内容 | 采集类型 |
|------|---------|---------|
| 1 | 文字：`你好` | `text` |
| 2 | 发一张图片 | `image` |
| 3 | 发一个文件 | `file` |

```bash
ls /tmp/cc-fixtures/wecom/
```

---

### Telegram — 项目 `king-dev`

Bot Token: `8729686742:AAGRZSz5ui7h2Ku8DR2bAoKwyY-2Ikkeq6c`

在 Telegram 中找到该 Bot（搜索 bot username），只有 `allow_from=5780721216` 的用户才会被接受：

| 操作 | 发送内容 | 采集类型 |
|------|---------|---------|
| 1 | `/start` | `text` |
| 2 | `hello bot` | `text` |
| 3 | 发一张图片（带 caption） | `image` |
| 4 | 发一个文件 | `file` |

```bash
ls /tmp/cc-fixtures/telegram/
```

---

### Discord — 项目 `king-dev`

Guild ID: `1477201497425973391`

在 Discord 找到该服务器，向 Bot 发送 DM 或在允许的频道 @Bot：

| 操作 | 说明 |
|------|------|
| DM 文字 | `hello` |
| DM 图片 | 发一张图片 |
| 频道 @Bot | `@Bot what can you do?` |

```bash
ls /tmp/cc-fixtures/discord/
```

---

## 第三步：查看采集结果

每个采集到的消息都是一个 JSON 文件，格式如下：

```json
{
  "schema_version": 1,
  "captured_at": "2026-05-19T14:32:01.234Z",
  "platform": "feishu",
  "message_type": "text",
  "message": {
    "platform": "feishu",
    "session_key": "feishu:oc_abc123:ou_def456",
    "message_id": "om_xyz789",
    "chat_name": "测试群组",
    "user_id": "ou_d***",       ← 已脱敏（仅保留前4字符）
    "user_name": "Alice",        ← 仅保留名字
    "content": "你好，这是采集测试",
    "images_count": 0,
    "files_count": 0,
    "has_audio": false,
    "is_group_chat": true,
    "is_mentioned": false
  }
}
```

> **隐私说明**：`user_id` 和 `user_name` 已自动脱敏，`Images`/`Files` 的原始字节不保存（只保存元数据）。无需额外处理即可提交到代码仓库。

---

## 第四步：将 fixtures 复制到测试目录

```bash
# 停止采集（恢复服务正常运行，去掉 env var）
sudo supervisorctl restart cc-connect
# 或 kill 临时启动的进程

# 复制到测试目录（排除空目录）
for platform in feishu wecom telegram discord weibo; do
    src="/tmp/cc-fixtures/$platform"
    dst="/root/code/cc-connect/tests/blackbox/fixtures/$platform"
    if [ -d "$src" ] && [ "$(ls -A $src)" ]; then
        cp -v "$src"/*.json "$dst/"
    fi
done
```

检查最终文件：

```bash
find /root/code/cc-connect/tests/blackbox/fixtures -name "*.json" \
  | grep -v sample | sort
```

---

## 第五步：运行 platform_sim 测试

```bash
cd /root/code/cc-connect

# 仅运行飞书 fixture 测试（不需要 API key，本地离线可跑）
go test -tags blackbox ./tests/blackbox/platform_sim/... -timeout 300s -v

# 运行全部 platform_sim（包括未来新增的平台）
go test -tags blackbox ./tests/blackbox/platform_sim/... -timeout 600s -v -run ".*"
```

---

## 采集频率建议

| 场景 | 建议采集时机 |
|------|------------|
| 飞书 SDK 升级 | 每次升级后必须重新采集并跑 sim 测试 |
| 企微 SDK 升级 | 同上 |
| 新消息类型支持（如富文本 post） | 上线前采集并补充测试 |
| 发版前的冒烟 | 选一个平台（通常飞书）快速采集文本+图片 |

---

## 如何从服务端 API 获取 JSON 数据

如果你希望捕获 cc-connect **向平台发送** 的数据格式（回复 payload），
可以用 cc-connect 的 HTTP debug 接口（如果已开启 `--debug-port`）：

```bash
# 查看最近处理的消息事件（包括收到和发出）
curl http://localhost:8787/debug/messages | jq .

# 也可以直接看 slog 日志中的 JSON 输出（所有 Reply/SendCard 调用都有日志）
sudo journalctl -u cc-connect -f | grep '"level":"INFO"'
```

或者，在 `~/.cc-connect/config.toml` 中开启 webhook hook 来捕获完整事件：

```toml
[[hooks]]
event = "message.received"
type = "webhook"
url = "http://localhost:9999/capture"  # 你自己起一个 echo server
```

---

## 故障排查

| 问题 | 可能原因 | 解决 |
|------|---------|------|
| `/tmp/cc-fixtures/` 目录为空 | `CC_CONNECT_RECORD_FIXTURES` 没有生效 | 检查环境变量注入方式 |
| 只有部分平台有 fixture | 其他平台的消息没有触发 | 按上面步骤逐个平台操作 |
| fixture 内 `user_id` 是 `""`  | 平台未返回 user_id | 属于正常（部分平台私信不提供） |
| `image_count > 0` 但测试中没有图片 | fixture 只有元数据，无原始 bytes | 用 `InjectMessageWithAttachments` 补充 bytes |
| platform_sim 测试全部 skip | fixtures 目录中没有真实文件 | 按本文档采集后重试 |

---

## 文件结构

```
tests/blackbox/
├── collector/
│   └── recorder.go              ← Recorder 实现（Wrap 平台、写 JSON）
├── fixtures/
│   ├── feishu/
│   │   ├── sample_text.json     ← 示例（不用于测试）
│   │   ├── 20260519_*_text_*.json   ← 真实采集（gitignore 可选）
│   │   └── README.md
│   ├── wecom/
│   ├── telegram/
│   ├── discord/
│   └── weibo/
├── platform/
│   ├── mock_platform.go         ← 通用测试平台
│   └── fixture_platform.go      ← Fixture 回放平台
└── platform_sim/
    └── feishu_sim_test.go       ← 飞书 fixture 测试
```

---

## 如何在新项目的 cc-connect 服务上采集

1. 找到项目的 supervisor 配置文件路径（一般是 `/etc/supervisor/conf.d/<project>.conf`）
2. 在 `[program:xxx]` 块中加入 `environment=CC_CONNECT_RECORD_FIXTURES="/tmp/fixtures-<project>"`
3. `supervisorctl reread && supervisorctl update && supervisorctl restart <project>`
4. 触发消息，采集完毕后移除该环境变量并重启

整个过程不需要改代码，只是注入一个环境变量。
