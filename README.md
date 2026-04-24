# Multi-Model Router

<!-- Banner -->
<p align="center">
  <img src="build/icon.svg" alt="Multi-Model Router" width="128" style="border-radius: 24px; box-shadow: 0 8px 32px rgba(0,0,0,0.3);"/>
</p>

<p align="center">
  <strong>AI 模型智能路由代理</strong> — 自动根据请求复杂度选择最优模型，支持 OpenAI / Anthropic / Google / Ollama 等多平台
</p>

<p align="center">

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Wails](https://img.shields.io/badge/Wails-v2-purple?style=flat&logo=wails)](https://wails.io/)
[![Platform](https://img.shields.io/badge/Platform-Windows-0078D4?style=flat&logo=windows)](https://github.com/ethanfly/multi_model_router)

</p>

---

## 功能特点

### 核心能力

| 功能 | 说明 |
|------|------|
| **多模型管理** | 统一管理多个 AI 模型，按推理/编程/创意/速度/性价比五维评分 |
| **智能路由** | 自动 / 手动 / 竞速三种模式，零配置自动选择最优模型 |
| **OpenAI 兼容代理** | 本地代理端口，现有工具零改造接入 |
| **请求统计** | 实时统计 Token 消耗、延迟、模型使用量、复杂度分布 |
| **导入 / 导出** | 模型配置加密导出，跨设备迁移 |

### 多端支持

| 模式 | 说明 |
|------|------|
| **桌面 GUI** | Wails + Vue 3，可视化模型管理和仪表盘 |
| **终端 TUI** | Bubble Tea 交互式管理，无需图形界面 |
| **Headless CLI** | 服务器静默运行，仅提供代理服务 |
| **系统托盘** | 关闭窗口最小化到托盘，后台常驻 |

### 其他

- 中英双语界面
- Windows 开机自启动
- API Key AES-256-GCM 加密存储
- 模型配置加密导入导出（PBKDF2 + AES-256-GCM）

---

## 快速开始

### 下载安装

从 [Releases](https://github.com/ethanfly/multi_model_router/releases) 下载最新版本 `MultiModelRouter-v*.exe`，双击运行。

### 首次配置

```
侧边栏 模型 → 添加模型（名称 / 供应商 / API Key / 模型 ID）
侧边栏 设置 → 配置端口 → 启动代理
```

### 接入使用

将任意 OpenAI 兼容客户端的 API 地址改为：

```
http://localhost:9680/v1
```

---

## 代理 API

### 认证方式

```bash
# Authorization Bearer
curl http://localhost:9680/v1/chat/completions \
  -H "Authorization: Bearer YOUR_PROXY_KEY" \
  -d '{"model":"auto","messages":[{"role":"user","content":"Hello"}]}'

# x-api-key 头
curl http://localhost:9680/v1/chat/completions \
  -H "x-api-key: YOUR_PROXY_KEY" \
  -d '{"model":"auto","messages":[{"role":"user","content":"Hello"}]}'
```

不配置密钥则跳过认证（适合本地开发）。

### 路由控制

| `model` 值 | 行为 |
|-------------|------|
| `"auto"` 或留空 | 使用默认路由模式 |
| `"race"` | 强制竞速模式，所有模型竞争，最快响应胜出 |
| `"gpt-4o"` 等具体名称 | 手动模式，强制路由到指定模型 |

### 路由模式

| 模式 | 说明 |
|------|------|
| `auto` | 根据请求复杂度（简单/中等/复杂）自动选择最合适的模型 |
| `manual` | 需在请求中指定 `model` 为具体模型名称 |
| `race` | 所有启用模型同时请求，最快响应被采用 |

---

## 命令行

```bash
# 查看帮助
MultiModelRouter.exe --help

# 查看版本
MultiModelRouter.exe version

# Headless 代理模式（无 GUI，适合服务器）
MultiModelRouter.exe serve --port 9680 --mode auto

# TUI 交互模式（终端管理界面）
MultiModelRouter.exe tui
```

### CLI 子命令

| 命令 | 说明 | 参数 |
|------|------|------|
| `serve` | 启动无头代理服务器 | `-p, --port` 端口（默认 9680）<br>`-m, --mode` 路由模式<br>`-k, --api-key` 代理密钥 |
| `tui` | 终端交互界面 | `-p, --port` 默认端口 |
| `version` | 显示版本号 | — |

### TUI 快捷键

| 按键 | 功能 |
|------|------|
| `1` / `2` / `3` | 切换标签页（状态 / 模型 / 统计） |
| `s` | 启动 / 停止代理 |
| `r` | 重新加载模型 |
| `↑` / `↓` | 列表导航 |
| `q` / `Ctrl+C` | 退出 |

---

## 界面预览

<details>
<summary>点击展开截图</summary>

> GUI 界面截图待添加

</details>

---

## 项目结构

```
multi_model_router/
├── main.go                    # 入口：GUI / CLI 分发
├── app.go                    # Wails GUI 绑定层
├── trayicon.go               # 程序托盘图标生成
│
├── frontend/                 # Vue 3 前端
│   └── src/
│       ├── views/           # 页面：聊天 / 模型 / 仪表盘 / 设置
│       ├── components/       # 组件：标题栏 / 模型卡片 / 编辑器
│       ├── stores/          # Pinia 状态管理
│       └── i18n/           # 中英双语
│
├── internal/
│   ├── core/               # 核心业务逻辑（独立于 GUI）
│   ├── cli/                # CLI 命令（cobra）
│   ├── tui/               # 终端 UI（Bubble Tea）
│   ├── config/            # 配置管理
│   ├── db/                # SQLite 数据库层
│   ├── router/            # 路由引擎 & 分类器
│   ├── provider/          # AI 供应商适配（OpenAI / Anthropic）
│   ├── proxy/             # HTTP 代理服务器
│   ├── stats/            # 请求统计收集
│   └── crypto/            # API Key 加密（AES-256-GCM）
│
├── scripts/
│   └── generate-icons.mjs # SVG → PNG / ICO 图标生成
│
└── build/                  # 构建产物 & 图标资源
```

---

## 开发

### 环境要求

| 依赖 | 版本 |
|------|------|
| Go | 1.25+ |
| Node.js | 18+ |
| Wails CLI | v2 |

```bash
# 安装 Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### 开发模式

```bash
wails dev
```

### 构建发布

```bash
# Windows 构建
build.bat

# 或直接使用 Wails
wails build -clean -ldflags "-s -w"
```

构建产物位于 `build/bin/MultiModelRouter.exe`。

---

## 技术栈

| 层级 | 技术 |
|------|------|
| 桌面框架 | [Wails v2](https://wails.io/) |
| 后端语言 | Go 1.25 |
| CLI 框架 | [Cobra](https://github.com/spf13/cobra) |
| TUI 框架 | [Bubble Tea](https://github.com/charmbracelet/bubbletea) |
| 数据库 | SQLite（modernc.org/sqlite，无 CGo） |
| 前端框架 | Vue 3 + TypeScript |
| 状态管理 | Pinia |
| 路由 | Vue Router 4 |
| 国际化 | vue-i18n |

---

## License

MIT License — 详见 [LICENSE](LICENSE)
