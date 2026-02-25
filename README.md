# mini-atoms

一个 AI 驱动的轻量应用生成平台（MVP）。

用户用自然语言描述需求，系统生成结构化 `Spec`，并基于 `Spec` 渲染可运行应用（不是代码生成），支持多轮迭代、发布与分享。

详细介绍：https://my.feishu.cn/wiki/EwsgwvMeLiTrLVkdR3WcOUGZnjf

## 功能特性

- 本地账号注册 / 登录（Cookie Session）
- 创建项目并输入自然语言需求
- AI 生成草稿 `Spec`（支持多轮对话迭代）
- 基于 `Spec` 的通用应用渲染与 CRUD 交互
- 草稿 / 已发布双态管理
- 发布页（`/p/{slug}`）
- 分享页（`/share/{slug}`，只读）
- SQLite 数据持久化
- 健康检查与就绪检查（`/healthz`、`/readyz`）

## 技术栈

- 后端：Go 1.25、Gin、GORM
- 数据库：SQLite
- 前端：Go Templates + HTMX + Alpine.js + Tailwind CSS
- AI：DeepSeek（未配置时自动回退 Stub）
- 部署：Docker / Fly.io（仓库内已提供 `Dockerfile`、`fly.toml`）

## 快速开始

### 1. 前置环境

- Go `1.25+`
- Node.js（建议 `18+`）
- `pnpm`（仓库使用 `pnpm@10.24.0`）

### 2. 安装前端依赖

```bash
make ui-install
```

### 3. 启动开发环境

```bash
make dev
```

默认启动地址：`http://localhost:8080`

默认本地数据库文件：`./data/mini-atoms-gorm.db`

### 4. 使用方式（本地）

- 打开 `http://localhost:8080`
- 注册账号并登录
- 或使用页面内 Demo 登录（默认演示账号）

说明：

- 如果未配置 `DEEPSEEK_API_KEY`，系统会自动使用 `Stub` 生成器，方便本地联调与演示。

## 环境变量

| 变量名 | 默认值 | 说明 |
| --- | --- | --- |
| `APP_ENV` | `development` | 运行环境（生产环境会校验 `SESSION_SECRET`） |
| `APP_ADDR` | `:8080` | HTTP 监听地址 |
| `DATABASE_PATH` | `./data/mini-atoms-gorm.db` | SQLite 文件路径 |
| `SESSION_SECRET` | `dev-session-secret-change-me` | Session 签名密钥（生产环境必须显式设置） |
| `DEEPSEEK_API_KEY` | 空 | DeepSeek API Key；为空时使用 Stub |
| `APP_BASE_URL` | `http://localhost:8080` | 应用对外访问地址（用于生成链接等） |
| `DEBUG_RESET_ALL_DATA_TOKEN` | 空 | 调试清库开关；配置后启用 `POST /debug/reset-db` |

示例（本地接入 DeepSeek）：

```bash
export DEEPSEEK_API_KEY="your-key"
export SESSION_SECRET="replace-me"
make dev
```

## 常用命令（Makefile）

```bash
make ui-install   # 安装前端依赖（pnpm）
make ui-build     # 构建 CSS（Tailwind）
make ui-dev       # Tailwind watch
make dev          # 启动开发服务器（含 CSS 构建）
make run          # 等价于 make dev
make build        # 构建 CSS + go mod tidy + go build ./...
make test         # go test ./...
make lint         # go vet ./...
make deploy       # flyctl deploy
```

## 核心页面与路由

- `GET /`：重定向到 `/projects`
- `GET /projects`：项目列表 / 创建项目（需登录）
- `GET /projects/:slug`：项目工作台（聊天、生成、预览、发布、分享）
- `GET /p/:slug`：已发布页面（公开访问）
- `GET /share/:slug`：分享页面（公开访问，只读）
- `GET /healthz`：健康检查
- `GET /readyz`：就绪检查（含数据库可用性）

## 本地构建与测试

```bash
make build
make test
make lint
```

CI 也会执行 `make build`（见 `.github/workflows/ci.yml`）。

## Docker 运行

### 构建镜像

```bash
docker build -t mini-atoms:local .
```

### 启动容器

```bash
docker run --rm \
  -p 8080:8080 \
  -e APP_ENV=production \
  -e SESSION_SECRET='replace-with-strong-secret' \
  -e APP_BASE_URL='http://localhost:8080' \
  -v "$(pwd)/data:/data" \
  mini-atoms:local
```

说明：

- 容器默认数据库路径为 `/data/mini-atoms-gorm.db`
- 生产环境请显式配置 `SESSION_SECRET`

## Fly.io 部署（仓库已内置配置）

仓库包含 `fly.toml`，可直接使用：

```bash
flyctl secrets set SESSION_SECRET='replace-with-strong-secret'
flyctl secrets set DEEPSEEK_API_KEY='your-key' # 可选
make deploy
```

## 项目结构

```text
cmd/server/              # 程序入口
internal/app/            # 应用启动与生命周期
internal/httpapp/        # HTTP 路由与页面/接口处理
internal/store/          # SQLite/GORM 数据访问与迁移
internal/generation/     # AI 生成客户端（DeepSeek / Stub）
internal/spec/           # Spec 定义与校验
internal/apprender/      # Spec -> 页面渲染
internal/auth/           # 注册/登录/会话
web/templates/           # Go HTML 模板
web/assets/              # Tailwind 源样式
web/static/              # 构建产物（CSS/静态文件）
documents/               # PRD、技术方案、演示脚本等文档
```

## 相关文档

- `documents/prd.md`：产品需求（MVP 收敛版）
- `documents/tech.md`：技术方案
- `documents/roadmap.md`：路线图
- `documents/demo-script.md`：演示脚本
- `documents/deploy-fly.md`：Fly.io 部署说明
