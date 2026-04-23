# Multi-Model Router

AI 模型智能路由代理 — 统一管理 OpenAI / Anthropic / Google / Ollama 等多个 AI 模型，自动根据请求复杂度选择最优模型。

## 功能

- **多模型管理** — 添加、配置、测试多个 AI 模型，按能力评分
- **智能路由** — 自动/手动/竞争三种模式，根据请求复杂度分配最优模型
- **OpenAI 兼容代理** — 启动本地代理，现有工具零改造接入
- **可视化仪表盘** — 请求统计、模型使用量、延迟分析
- **桌面 GUI + CLI** — 图形界面和命令行双模式运行
- **TUI 管理界面** — 终端内交互式管理（Bubble Tea）
- **系统托盘** — 关闭窗口后最小化到托盘，后台运行
- **开机自启动** — 可选 Windows 开机自动运行
- **中英双语** — 支持中文和英文界面

## 快速开始

### GUI 模式（默认）

双击 `MultiModelRouter.exe` 或不带参数运行：

```bash
MultiModelRouter.exe
```

首次运行后：
1. 点击侧栏 **模型** → 添加 AI 模型（填写名称、供应商、API Key、模型 ID）
2. 点击侧栏 **设置** → 配置代理端口 → 启动代理
3. 将你的工具（如 ChatGPT 客户端、代码编辑器）的 API 地址指向 `http://localhost:9680`

### CLI 模式

```bash
# 查看帮助
MultiModelRouter.exe --help

# 查看版本
MultiModelRouter.exe version

# Headless 代理模式（无 GUI，适合服务器）
MultiModelRouter.exe serve --port 9680 --mode auto --api-key sk-your-key

# TUI 交互模式（终端管理界面）
MultiModelRouter.exe tui
```

#### CLI 子命令

| 命令 | 说明 | 参数 |
|------|------|------|
| `serve` | 启动无头代理服务器 | `-p, --port` 代理端口（默认 9680）<br>`-m, --mode` 路由模式：auto/manual/race<br>`-k, --api-key` 代理 API 密钥 |
| `tui` | 启动终端管理界面 | `-p, --port` 默认代理端口 |
| `version` | 显示版本号 | — |

#### TUI 快捷键

| 按键 | 功能 |
|------|------|
| `1` / `2` / `3` | 切换标签页（状态/模型/统计） |
| `s` | 启动/停止代理 |
| `r` | 重新加载模型 |
| `↑` / `↓` | 列表导航 |
| `q` / `Ctrl+C` | 退出 |

## 代理使用

启动代理后，将任何 OpenAI 兼容客户端的 Base URL 改为：

```
http://localhost:9680/v1
```

### API 密钥认证

在设置页面配置代理 API 密钥后，请求必须携带密钥：

```bash
# 方式一：Authorization Bearer
curl http://localhost:9680/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_PROXY_KEY" \
  -d '{"model":"auto","messages":[{"role":"user","content":"Hello"}]}'

# 方式二：x-api-key 头
curl http://localhost:9680/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "x-api-key: YOUR_PROXY_KEY" \
  -d '{"model":"auto","messages":[{"role":"user","content":"Hello"}]}'
```

不配置密钥则不校验（适合本地开发）。

### 模型路由

通过 `model` 字段控制路由行为：

| model 值 | 行为 |
|----------|------|
| `"auto"` 或空 | 使用默认路由模式（设置页配置） |
| `"race"` | 强制竞速模式 |
| 具体模型名（如 `"Kimi-K2.6"`） | 手动模式，路由到指定模型 |

```bash
# 自动路由
curl ... -d '{"model":"auto","messages":[...]}'

# 指定模型
curl ... -d '{"model":"Kimi-K2.6","messages":[...]}'

# 竞速模式
curl ... -d '{"model":"race","messages":[...]}'
```

路由模式：
- `auto` — 自动根据复杂度选择模型
- `manual` — 需指定模型名称
- `race` — 多模型竞争，最快响应胜出

## 开发

### 环境要求

- Go 1.25+
- Node.js 18+
- [Wails CLI](https://wails.io/docs/gettingstarted/installation) v2

### 开发模式

```bash
wails dev
```

### 构建

```bash
# 标准构建
wails build

# 优化构建（更小体积）
wails build -clean -ldflags "-s -w"
```

构建产物位于 `build/bin/MultiModelRouter.exe`。

## 项目结构

```
├── app.go                    # Wails GUI 适配层
├── main.go                   # 入口：GUI / CLI 分发
├── frontend/                 # Vue 3 前端
│   └── src/
│       ├── views/            # 页面（聊天、模型、仪表盘、设置）
│       ├── components/       # 组件（标题栏、模型卡片、编辑器）
│       └── stores/           # Pinia 状态管理
├── internal/
│   ├── core/                 # 核心业务逻辑（独立于 GUI）
│   ├── cli/                  # CLI 子命令（cobra）
│   ├── tui/                  # 终端管理界面（Bubble Tea）
│   ├── config/               # 配置管理
│   ├── db/                   # SQLite 数据库
│   ├── router/               # 路由引擎
│   ├── provider/             # AI 模型供应商适配
│   ├── proxy/                # HTTP 代理服务器
│   ├── stats/                # 统计收集
│   └── crypto/               # API Key 加密存储
└── build/                    # 构建产物
```

## 技术栈

- **后端**: Go, Wails v2, Cobra, Bubble Tea, SQLite
- **前端**: Vue 3, TypeScript, Pinia, Vue Router, vue-i18n
- **TUI**: Bubble Tea, Lip Gloss

## License

MIT
