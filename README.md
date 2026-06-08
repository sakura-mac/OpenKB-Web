<div align="center">

# 📚 OKB Web

<p align="center"><i>多空间管理&nbsp; • &nbsp;实体关系图谱&nbsp; • &nbsp;多轮对话&nbsp; • &nbsp;幻灯片导出&nbsp; • &nbsp;国际化&nbsp; • &nbsp;单文件部署</i></p>

</div>

---

## 📑 什么是 OKB Web

**OKB Web** 是 [OpenKB](https://github.com/VectifyAI/OpenKB) 的 Web 管理界面——一个全栈应用，将 OpenKB 的 CLI 能力封装为可视化的多空间知识库管理平台。

Go (Gin) 后端 + Vue 3 (Vite + TypeScript) 前端，前端编译后通过 `go:embed` 嵌入二进制，产物为单个 ~13MB 可执行文件，零运行时依赖。首次启动时自动通过 `uv tool install` 拉取 OpenKB，无需手动安装 Python 环境。

### 核心特性

- **多空间隔离**：一个实例管理多个独立知识库，侧边栏一键切换
- **可视化浏览**：Markdown 渲染 + 实体关系图谱，知识结构一目了然
- **拖拽上传**：支持多文件拖拽、路径输入、服务器文件浏览器三种方式
- **流式对话**：SSE 实时推送 LLM 回答，多轮会话持久化
- **国际化**：中英双语，基于 IP/Accept-Language 自动检测，支持手动切换
- **URL 文档智能标题**：添加 URL 文档时自动抓取网页 `<title>` 作为显示名
- **Deck 幻灯片**：将知识库编译为 HTML 演示文稿，支持在线预览/下载
- **团队共享**：部署一次，团队通过浏览器即可使用

---

## 🚀 快速开始

### 方式一：下载预编译二进制（推荐）

到 **[Releases](https://github.com/sakura-mac/OpenKB-Web/releases/latest)** 下载对应平台压缩包：

| 平台 | 文件 |
|---|---|
| macOS Apple Silicon (M1/M2/M3) | `OpenKB-Web-vX.Y.Z-darwin-arm64.tar.gz` |
| macOS Intel | `OpenKB-Web-vX.Y.Z-darwin-amd64.tar.gz` |
| Windows x64 | `OpenKB-Web-vX.Y.Z-windows-amd64.zip` |
| Linux x64 | `OpenKB-Web-vX.Y.Z-linux-amd64.tar.gz` |
| Linux ARM64 | `OpenKB-Web-vX.Y.Z-linux-arm64.tar.gz` |

```bash
# macOS / Linux
tar -xzf OpenKB-Web-*-darwin-arm64.tar.gz
cd OpenKB-Web-*
./okb-web
# macOS 首次运行如被 Gatekeeper 拦截：xattr -d com.apple.quarantine ./okb-web
```

```powershell
# Windows
Expand-Archive OpenKB-Web-*-windows-amd64.zip
cd OpenKB-Web-*
.\okb-web.exe
```

启动后浏览器打开 http://localhost:8901，首次会自动下载 uv 并安装 OpenKB（约 1-2 分钟）。在 Web 设置页填入 LLM API Key 即可使用。

校验下载完整性：

```bash
sha256sum -c SHA256SUMS.txt
```

### 方式二：从源码编译

#### 前置要求

| 依赖 | 版本 | 说明 |
|------|------|------|
| [Go](https://go.dev) | ≥ 1.23 | 编译后端 |
| [Node.js](https://nodejs.org) | ≥ 18 | 编译前端 |
| LLM API Key | — | DeepSeek / OpenAI / Claude 等 LiteLLM 兼容 API |

> **注意**：无需手动安装 uv / OpenKB / Python。首次启动时自动从 GitHub 拉取 uv standalone 二进制并 `uv tool install` 拉取 OpenKB 及其所有依赖到 `~/.config/OKB/runtime/`，与系统 Python 完全隔离。

### 安装 & 启动

```bash
git clone <repo-url> && cd okb-web

# 1. 配置环境变量
cp .env.example .env
# 编辑 .env，填入你的 LLM_API_KEY

# 2. 一键启动（自动停旧进程 + 编译前后端 + 启动 + 健康检查）
bash start.sh

# 3. 打开浏览器
open http://localhost:8901
```

### start.sh 用法

```bash
bash start.sh          # 完整构建（前端 + 后端）并启动
bash start.sh skip-web # 跳过前端编译（仅改了 Go 代码时）
```

脚本特性：
- 可靠停止旧进程（按端口查找 + 进程名校验，避免误杀）
- 实时显示编译和启动日志
- 15 秒超时保护 + 进程存活检测
- 失败时自动清理并输出诊断信息

### 配置 LLM

通过 [LiteLLM](https://docs.litellm.ai/docs/providers) 支持多 LLM 提供商。创建 `.env`：

```bash
LLM_API_KEY=your-api-key-here
LLM_BASE_URL=https://api.deepseek.com
LLM_MODEL=deepseek/deepseek-chat
LLM_LANGUAGE=zh
```

模型名使用 `provider/model` LiteLLM 格式（如 `anthropic/claude-sonnet-4-6`）。OpenAI 模型可省略前缀。

创建空间时，全局 `.env` 中的 LLM 配置会自动分发到各空间独立 `.env`，每个空间可独立修改。

---

## 🧩 架构

```
浏览器 (Vue 3 SPA + vue-router + vue-i18n)
 │
 ├── GET / ──────────────────▶ go:embed 嵌入的 web/dist/*
 │
 ├── /api/spaces/* ──────────▶ 空间 CRUD（直接文件系统操作）
 │
 ├── /api/upload/:space ─────▶ 保存文件到 raw/ + spawn openkb add
 │
 ├── /api/chat/stream ───────▶ SSE 流式推送（spawn chat_helper.py）
 │
 ├── /api/deck ──────────────▶ 异步生成 HTML 幻灯片
 │
 ├── /api/locale ────────────▶ IP/Accept-Language 自动语言检测
 │
 └── /api/{add,remove,query} ▶ spawn 子进程
                                    │
                                    ▼
                  uv tool 安装的 openkb <cmd>
                                    │
                                    └──▶ LLM API (DeepSeek/OpenAI/Claude/Gemini)
```

### 前端四大视图

| 视图 | 功能 | 技术实现 |
|------|------|----------|
| 🧠 知识 | 三栏切换浏览 Summaries / Concepts / Entities | Markdown 渲染 (marked) + wikilink 跳转 |
| 📄 文档 | 路径添加 / 拖拽上传 / 文件浏览器 / 删除 | 异步 task 轮询 + URL 标题自动抓取 |
| 🕸️ 图谱 | 实体关系可视化，类型过滤、搜索高亮 | Cytoscape.js 力导向布局 |
| 💬 查询 | 多轮对话，会话持久化，SSE 流式输出 | OpenKB Agent SDK 桥接 |

### 后端模块

| 模块 | 文件 | 职责 |
|------|------|------|
| 配置 | `internal/config/config.go` | .env 加载、uv 路径解析 |
| 核心 Handler | `internal/handler/handler.go` | 空间/文档/Wiki/图谱/浏览/历史 |
| 对话 | `internal/handler/chat.go` | 多轮对话（SSE 流式 + 异步 task） |
| 幻灯片 | `internal/handler/deck.go` | Deck 生成/列表/预览/删除 |
| 国际化 | `internal/handler/locale.go` | IP 地理位置 + Accept-Language 检测 |
| URL 标题 | `internal/handler/urltitle.go` | 网页 title 抓取 + 持久化缓存 |
| OpenKB 封装 | `internal/okb/okb.go` | CLI 调用封装（spawn 子进程） |
| 自动安装 | `internal/okb/bootstrap.go` | 首次启动自动安装 OpenKB |
| 异步任务 | `internal/okb/task.go` | 内存 map + 定时清理 |
| Git 提交 | `internal/okb/git.go` | 空间变更自动 commit |
| 资源嵌入 | `internal/assets/assets.go` | chat_helper.py + skills 释放 |

---

## 🗑️ 卸载

每个 release 包内自带 `uninstall.sh`（macOS/Linux）或 `uninstall.bat`（Windows）。运行后会交互式询问卸载方式：

| 模式 | 删除 | 保留 |
|---|---|---|
| **温柔卸载（推荐）** | runtime + cache（约 200MB，可重新下载） | spaces（笔记） + config（API key） |
| **彻底卸载** | 全部 OKB 数据 | — |

### macOS / Linux

```bash
cd OpenKB-Web-v0.1.0-darwin-arm64
./uninstall.sh

# 或非交互
./uninstall.sh --keep --yes    # 温柔
./uninstall.sh --purge --yes   # 彻底（含笔记，慎用）
```

### Windows

```cmd
cd OpenKB-Web-v0.1.0-windows-amd64
uninstall.bat

REM 或非交互
uninstall.bat /keep /yes
uninstall.bat /purge /yes
```

### OKB Web 数据存哪？

| 平台 | 路径 |
|---|---|
| macOS | `~/Library/Application Support/OKB/` |
| Linux | `~/.config/OKB/` |
| Windows | `%AppData%\OKB\` |

目录结构：

```
OKB/
├── config.json         # LLM 配置（API key 在这）
├── spaces/             # 你的所有知识库
├── runtime/            # uv + OpenKB Python 环境（约 200MB，可重装）
└── cache/              # chat_helper.py + skills（可重装）
```

OKB Web **不污染系统**：不写注册表、不改 PATH、不进 `/usr/local/`、不开机自启、不碰系统 Python。卸载脚本删完之后零残留。

---

## ❓ 常见问题

**Q: macOS 双击/运行报「无法打开，因为它来自身份不明的开发者」**

```bash
# 解 Gatekeeper 隔离（一次即可）
xattr -d com.apple.quarantine ./okb-web
```

或：系统设置 → 隐私与安全 → 滑到底「已阻止 okb-web」→ 仍要打开。

**Q: 进度遮罩卡在「下载运行时」很久**

国内网络拉 GitHub Releases 慢。设置代理后重启：

```bash
export HTTPS_PROXY=http://127.0.0.1:7890   # 改成你的代理端口
./okb-web
```

**Q: 端口 8901 被占用**

```bash
PORT=9000 ./okb-web      # 换端口
# 或
lsof -ti:8901 | xargs kill   # 杀占用进程
```

**Q: 想后台跑**

```bash
nohup ./okb-web > okb.log 2>&1 &
```

**Q: 想用自定义数据目录**

```bash
OKB_HOME=/path/to/custom ./okb-web
```

---

## 📡 API 参考

### 空间

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/spaces` | 列出所有空间（名称、文档数、概念数） |
| GET | `/api/space/:name` | 空间详情（文档/概念/实体列表） |
| GET | `/api/space-status/:name` | 空间初始化状态（initializing/ready/error） |
| POST | `/api/spaces/create` | 创建空间 `{"name", "path?"}` |
| POST | `/api/spaces/delete` | 删除空间 `{"name"}` |

### 文档

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/add` | 添加文档（路径/URL）`{"space", "path"}` |
| POST | `/api/upload/:space` | 上传文件 `multipart/form-data` field: `files` |
| POST | `/api/remove` | 删除文档 `{"space", "doc"}` |
| POST | `/api/browse` | 目录浏览 `{"path"}` 返回文件/目录列表 |

### Wiki 与知识

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/wiki/:space/:category/:page` | 读取 wiki 页面 Markdown |
| GET | `/api/graph/:space` | 实体关系图谱数据（节点 + 边） |
| GET | `/api/status/:space` | 知识库统计信息 |
| GET | `/api/lint/:space` | 结构 + 知识健康检查 |
| POST | `/api/recompile/:space` | 重新编译知识库 |
| GET | `/api/history/:space` | 变更历史 |
| POST | `/api/revert` | 回滚到历史版本 |

### 对话

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/chat/sessions/:space` | 列出所有会话 |
| GET | `/api/chat/session/:space/:sid` | 加载会话完整消息 |
| DELETE | `/api/chat/session/:space/:sid` | 删除会话 |
| POST | `/api/chat/send` | 发送消息（异步 task） |
| POST | `/api/chat/stream` | 发送消息（SSE 流式） |

### Deck 幻灯片

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/deck` | 生成幻灯片 `{"space", "name", "intent", "critique?"}` |
| GET | `/api/decks/:space` | 列出已生成的 deck |
| GET | `/api/deck/:space/:name` | 在线预览（`?download=1` 下载） |
| DELETE | `/api/deck/:space/:name` | 删除 deck |

### 其他

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/task/:id` | 查询异步任务状态（running/done/error） |
| POST | `/api/query` | 单次查询（非会话模式） |
| GET | `/api/locale` | 自动语言检测（返回 locale + 判断依据） |

---

## ⚙️ 配置

### 环境变量

| 变量 | 必填 | 默认值 | 说明 |
|------|:----:|--------|------|
| `LLM_API_KEY` | ✅ | — | LLM API 密钥 |
| `LLM_BASE_URL` | | `https://api.deepseek.com` | API 基础 URL |
| `LLM_MODEL` | | `deepseek/deepseek-chat` | 模型名（LiteLLM 格式） |
| `LLM_LANGUAGE` | | `zh` | Wiki 输出语言 |
| `PORT` | | `8901` | 服务监听端口 |
| `SPACES_ROOT` | | `okb-spaces` | 空间存储根目录 |
| `OKB_SPEC` | | `git+https://github.com/VectifyAI/OpenKB` | OpenKB 安装源 |
| `UV_BIN` | | 自动查找 | uv 可执行文件路径 |

### LLM 提供商示例

| 提供商 | 模型格式 |
|--------|----------|
| DeepSeek | `deepseek/deepseek-chat` |
| OpenAI | `gpt-5.4` |
| Anthropic | `anthropic/claude-sonnet-4-6` |
| Gemini | `gemini/gemini-3.1-pro-preview` |

### 运行时数据结构

```
okb-spaces/
└── <space-name>/
    ├── .env                     # 该空间 LLM 配置
    ├── .openkb/
    │   ├── config.yaml          # 空间级配置
    │   ├── hashes.json          # 文档去重注册表
    │   ├── url-titles.json      # URL 文档标题缓存
    │   └── chats/               # 对话会话持久化
    ├── raw/                     # 原始文档
    ├── wiki/
    │   ├── index.md             # 知识库概览
    │   ├── AGENTS.md            # Wiki 结构定义
    │   ├── sources/             # 全文转换
    │   ├── summaries/           # 文档摘要
    │   ├── concepts/            # 跨文档主题概念
    │   └── entities/            # 命名实体
    └── output/
        └── decks/               # 生成的幻灯片
```

---

## 🏗️ 项目结构

```
okb-web/
├── main.go                      # 入口：路由注册 + embed 前端 + 定时清理
├── start.sh                     # 一键停旧 + 编译 + 启动 + 健康检查
├── Makefile                     # dev / build / clean
├── .env.example                 # 环境变量模板
├── internal/
│   ├── config/config.go         # 配置加载（.env → Config 结构体）
│   ├── handler/
│   │   ├── handler.go           # 空间/文档/Wiki/图谱/浏览/历史 handler
│   │   ├── chat.go              # 多轮对话（SSE 流式 + 异步 task）
│   │   ├── deck.go              # Deck 幻灯片生成/列表/预览/删除
│   │   ├── locale.go            # IP/Accept-Language 语言自动检测
│   │   └── urltitle.go          # URL 网页标题抓取 + 持久化
│   ├── model/model.go           # 请求/响应结构体
│   ├── okb/
│   │   ├── okb.go               # OpenKB CLI 调用封装
│   │   ├── bootstrap.go         # 首次启动自动安装 OpenKB
│   │   ├── task.go              # 异步任务管理器
│   │   └── git.go               # 空间 Git 自动提交
│   └── assets/
│       ├── assets.go            # embed chat_helper.py + skills
│       ├── chat_helper.py       # Chat Agent SDK 桥接
│       └── skills/              # OpenKB deck/critic skills
├── scripts/
│   └── chat_helper.py           # Chat Agent SDK 源文件
└── web/                         # Vue 3 前端
    ├── src/
    │   ├── App.vue              # 主布局（侧边栏 + 路由视图）
    │   ├── api.ts               # 统一 API 封装
    │   ├── types.ts             # TypeScript 类型定义
    │   ├── i18n.ts              # 国际化配置（中/英）
    │   ├── main.ts              # 入口（挂载 router + i18n）
    │   ├── composables/
    │   │   ├── useUpload.ts     # 文件上传组合式函数
    │   │   └── useChatState.ts  # 对话状态管理
    │   └── views/
    │       ├── WikiView.vue     # 知识浏览（摘要/概念/实体）
    │       ├── DocsView.vue     # 文档管理（添加/上传/删除）
    │       ├── GraphView.vue    # 实体关系图谱（Cytoscape.js）
    │       └── QueryView.vue    # 多轮对话（SSE 流式）
    ├── package.json
    └── vite.config.ts           # 开发代理 /api → :8901
```

---

## 🛠️ 开发

### 开发模式

```bash
# 终端 1：后端热重载（需安装 air）
make dev

# 终端 2：前端 HMR
make dev-frontend
# → http://localhost:5173，API 自动代理到 :8901
```

### Makefile 命令

| 命令 | 说明 |
|------|------|
| `make dev` | 后端热重载（air） |
| `make dev-frontend` | 前端 HMR 开发服务（:5173） |
| `make dev-backend` | 后端 `go run`（无热重载） |
| `make frontend` | 编译前端到 `web/dist/` |
| `make build` | 完整构建（前端 + Go 二进制，CGO_ENABLED=0） |
| `make clean` | 清理产物 |

### 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go 1.22+ · [Gin](https://gin-gonic.com/) · go:embed |
| 前端 | Vue 3.5 · TypeScript 6 · [Vite](https://vite.dev/) 8 |
| 路由 | [vue-router](https://router.vuejs.org/) 5 |
| 国际化 | [vue-i18n](https://vue-i18n.intlify.dev/) 10 |
| 图谱 | [Cytoscape.js](https://js.cytoscape.org/) 3.34 |
| Markdown | [marked](https://marked.js.org/) 18 |
| 知识引擎 | [OpenKB](https://github.com/VectifyAI/OpenKB)（via `uv tool`） |
| LLM 网关 | [LiteLLM](https://github.com/BerriAI/litellm)（多提供商） |
| 文档转换 | [markitdown](https://github.com/microsoft/markitdown) · [pymupdf](https://pymupdf.readthedocs.io/) |
| 长文档索引 | [PageIndex](https://github.com/VectifyAI/PageIndex)（无向量检索） |

---

## 🧭 了解更多

### 与 OpenKB CLI 对比

| | OpenKB CLI | OKB Web |
|---|---|---|
| 交互方式 | 终端命令行 | 浏览器 GUI |
| 多空间 | 手动 cd 切换目录 | 侧边栏一键切换 |
| 文档添加 | `openkb add <path>` | 拖拽上传 + 路径浏览器 + URL |
| 知识浏览 | 打开 Markdown 文件 | 内置渲染 + 图谱可视化 |
| 对话 | TTY REPL | Web Chat（SSE 流式） |
| 幻灯片 | `openkb deck new` | 一键生成 + 在线预览 |
| 国际化 | 英文 | 中英双语自动检测 |
| 部署 | 本地 Python 环境 | 单二进制，零依赖 |
| 团队使用 | 每人各装一套 | 部署一次，浏览器访问 |

### Bootstrap 自动安装

首次启动时自动：

1. 检测 `uv` 可执行文件（`UV_BIN` > PATH > `~/.local/bin/uv` > 系统路径）
2. 执行 `uv tool install --prerelease=allow --from <OKB_SPEC> openkb`
3. 解析安装后的 `openkb` 和 `python` 绝对路径
4. 释放 `chat_helper.py` 和 skills 到 `~/.cache/okb-web/`
5. 将 SHA256(spec) 写入 marker 文件，下次启动秒过

可通过 `OKB_SPEC` 环境变量切换安装源：
- 默认：`git+https://github.com/VectifyAI/OpenKB`（最新 main，含 deck）
- 稳定版：`openkb==0.3.0`（PyPI，不含 deck）

### 与 Obsidian 配合

OKB Web 生成的 wiki 是标准 Markdown + `[[wikilink]]` 格式，可直接用 Obsidian 打开 `okb-spaces/<name>/wiki/` 作为 vault。

---

## 📝 License

MIT
