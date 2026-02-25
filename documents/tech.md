# mini-atoms - 技术方案（MVP 收敛版）

版本：v1.1（收敛版）
目标：优先完成可在线演示的核心闭环（自然语言 -> 草稿生成 -> 多轮迭代 -> 发布 -> 分享），并降低实现复杂度与返工风险。

---

# 1. 设计目标与原则

mini-atoms 是一个 AI 驱动的轻量应用生成系统。本版技术方案强调：

1. 单一 Go 服务架构（不使用多进程或沙箱）
2. 不执行 LLM 生成代码（仅生成结构化 Spec）
3. 通用数据模型（避免动态建表）
4. 草稿 / 已发布双态（替代完整版本管理）
5. 多轮对话保留，但每轮重新生成完整草稿
6. AI 可视化做简化版（固定阶段）
7. 结构清晰，便于后续扩展为完整版本系统

---

# 2. 总体架构

## 2.1 架构模式

- 单一 Go 服务
- SQLite 单数据库
- 多租户逻辑隔离（`user_id` + `project_id`）
- Go Gin + Go Template + HTMX + AlpineJS（轻前端）
- DeepSeek API（或兼容 API）负责 LLM 生成

**核心技术选型**
| 类别 | 技术选型 | 理由 |
| :--- | :--- | :--- |
| **后端语言** | **Go** | 性能卓越，编译为单二进制文件，部署极简，完美契合您的技能背景。 |
| **后端框架** | **Gin** | 轻量、高效、路由功能强大，社区成熟，上手快。 |
| **前端方案** | **HTMX + Alpine.js + pnpm** | **[效率核心]** 无需学习庞大的前端框架，让后端直接驱动交互，极大提升开发效率。 |
| **UI 样式** | **Tailwind CSS** | **[美观核心]** Utility-first 框架，无需成为设计专家也能快速构建精美、专业的界面。 |
| **数据库** | **SQLite** | **[成本核心]** 零成本、零配置、文件即数据库。性能对个人项目绰绰有余，极大降低运维负担。 |
| **ORM** | **GORM** | Go 生态中最成熟的 ORM，与 SQLite 配合无间，简化数据库操作。 |
| **部署方案** | **Fly.io ** | **[成本核心]** 提供慷慨的免费额度，能完美运行 Go 应用和挂载 SQLite 数据库文件，实现零成本上线。 |
| **AI 集成** | **Deepseek API** | 直接在 Go 后端通过标准 HTTP 请求调用，简单直接。 |


## 2.2 核心模块

### 前端层

- Go Template 渲染 HTML
- HTMX 负责局部刷新（聊天、预览、列表等）
- AlpineJS 负责轻交互（计时器、状态切换 UI 等）

### 后端模块

1. Auth 模块（简单账号密码注册登录）
2. Session 模块（Cookie 会话）
3. Project 模块（项目管理 + 草稿/发布状态）
4. Chat 模块（多轮对话记录）
5. Generation 模块（LLM 调用、校验、修复）
6. Spec Engine（Spec 校验与约束）
7. Primitive Renderer（原语渲染器）
8. CRUD API（通用数据读写）
9. Publish 模块（发布到 `/p/{slug}`）
10. Share 模块（分享页 `/share/{slug}`，只读）
11. Showcase 占位（仅保留字段与入口，不做内容）

---

# 3. 范围收敛（与 PRD 对齐）

## 3.1 P0 保留

- 简单认证（本地账号密码）
- 多轮对话（每轮重生成草稿）
- 简化 AI 可视化
- 草稿/发布双态
- 分享页（公开只读）

## 3.2 P0 不做

- 完整版本管理（`project_versions` 历史表、版本切换、回滚）
- Showcase 页面与预置内容
- 第三方认证
- 外部 API 集成
- 沙箱执行

---

# 4. 数据库设计（SQLite）

说明：

- 采用单库。
- 使用通用 `collections + records` 模型承载业务数据。
- 不使用 `project_versions`，改为项目表内维护 `draft_spec_json` 与 `published_spec_json`。

## 4.1 平台层表

### users

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | INTEGER | 主键 |
| username | TEXT | 唯一账号（本地登录） |
| password_hash | TEXT | bcrypt 哈希 |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |

约束：

- `UNIQUE(username)`

### user_sessions（简单会话）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | INTEGER | 主键 |
| user_id | INTEGER | 用户 ID |
| session_token | TEXT | 随机 token（写入 Cookie） |
| expires_at | DATETIME | 过期时间 |
| created_at | DATETIME | 创建时间 |

约束：

- `UNIQUE(session_token)`

说明：

- P0 使用服务端会话表 + HttpOnly Cookie，方便实现登出与失效。

### projects

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | INTEGER | 主键 |
| user_id | INTEGER | 所属用户 |
| name | TEXT | 项目名称 |
| goal_prompt | TEXT | 初始需求 |
| share_slug | TEXT | 分享路径标识 |
| published_slug | TEXT | 发布路径标识 |
| draft_spec_json | TEXT | 当前草稿 Spec |
| published_spec_json | TEXT | 当前已发布 Spec |
| is_showcase | BOOLEAN | Showcase 预留标识（P0 不使用） |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |
| published_at | DATETIME | 最近发布时间（可空） |

约束：

- `UNIQUE(share_slug)`（允许空）
- `UNIQUE(published_slug)`（允许空）

说明：

- `draft_spec_json` 为当前编辑态。
- `published_spec_json` 为当前线上态。
- 再次发布会覆盖 `published_spec_json`，不保留历史版本。

### chat_messages

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | INTEGER | 主键 |
| project_id | INTEGER | 所属项目 |
| round_no | INTEGER | 对话轮次（从 1 开始） |
| role | TEXT | `user` / `assistant` / `system` |
| content | TEXT | 消息内容 |
| created_at | DATETIME | 创建时间 |

说明：

- 每轮至少包含一条用户消息。
- `assistant` 消息保存面向用户的 AI 回复内容（可由服务端根据本轮结果生成摘要），用于聊天和分享展示。
- 生成失败时仍可写入 `system` 消息，便于排错与分享展示。
- AI 可视化状态不落库，由前端请求生命周期驱动。

## 4.2 通用数据模型

### collections

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | INTEGER | 主键 |
| project_id | INTEGER | 所属项目 |
| name | TEXT | 逻辑表名 |
| schema_json | TEXT | 字段结构定义 |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |

约束：

- `UNIQUE(project_id, name)`

### records

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | INTEGER | 主键 |
| project_id | INTEGER | 所属项目 |
| collection_id | INTEGER | 所属逻辑表 |
| data_json | TEXT | 实际数据 |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |

索引建议：

- `(project_id, collection_id)`
- `(collection_id, updated_at)`

---

# 5. Spec 设计（最小可实现定义）

mini-atoms 不生成代码，而是生成结构化 Spec，通过原语渲染页面。

## 5.1 顶层结构（MVP）

```json
{
  "app_name": "Todo App",
  "theme": "light",
  "collections": [
    {
      "name": "todos",
      "fields": [
        { "name": "title", "type": "text", "required": true },
        { "name": "done", "type": "bool", "required": true }
      ]
    }
  ],
  "pages": [
    {
      "id": "home",
      "title": "Home",
      "blocks": [
        { "type": "nav", "items": [{ "label": "Home", "page_id": "home" }] },
        { "type": "form", "collection": "todos" },
        { "type": "list", "collection": "todos" },
        { "type": "toggle", "collection": "todos", "field": "done" },
        { "type": "stats", "collection": "todos", "metric": "count" }
      ]
    }
  ]
}
```

## 5.2 允许的原语（Primitives）

1. `nav`
2. `list`
3. `form`
4. `toggle`
5. `stats`
6. `timer`

## 5.3 允许字段类型

- `text`
- `int`
- `real`
- `bool`
- `date`
- `datetime`
- `enum`

## 5.4 结构限制（Validator 强约束）

- 单 project collections <= 5
- 单 collection 字段 <= 10
- 禁止嵌套对象
- 禁止数组字段（`enum` 的选项列表除外）
- `pages[].blocks[]` 的 `type` 必须在白名单内
- `toggle.field` 必须引用 `bool` 字段
- `stats.metric` 仅允许 `count` / `sum`
- `stats.metric = sum` 时必须指定数值字段（`int` / `real`）
- `enum` 字段必须包含非空 `options`

## 5.5 Block 最小字段约定（MVP）

### `nav`

```json
{ "type": "nav", "items": [{ "label": "Home", "page_id": "home" }] }
```

### `list`

```json
{ "type": "list", "collection": "todos", "fields": ["title", "done"] }
```

说明：

- `fields` 可选，缺省则展示集合全部字段。

### `form`

```json
{ "type": "form", "collection": "todos", "fields": ["title"] }
```

说明：

- `fields` 可选，缺省则使用可编辑字段。

### `toggle`

```json
{ "type": "toggle", "collection": "todos", "field": "done" }
```

### `stats`

```json
{ "type": "stats", "collection": "todos", "metric": "count", "label": "未完成数" }
```

或：

```json
{ "type": "stats", "collection": "focus_sessions", "metric": "sum", "field": "minutes" }
```

### `timer`（简化）

```json
{
  "type": "timer",
  "session_collection": "focus_sessions",
  "work_minutes": 25,
  "break_minutes": 5
}
```

说明：

- P0 可以先做前端计时 + 手动记录提交。
- 不要求高精度后台任务调度。

---

# 6. LLM 生成与校验流程

## 6.1 每轮生成输入

每次调用包含：

1. System Prompt（强约束）
2. 最近 N 条对话历史（建议截断）
3. 当前草稿 Spec（若存在）
4. 当前用户输入

## 6.2 关键策略（P0）

- 每轮重新生成完整草稿 Spec
- 不做 AST diff / patch 合并
- 失败时可进行一次修复重试（提供校验错误）

## 6.3 System Prompt 约束重点

- 只允许指定原语与字段类型
- 限制数量与结构复杂度
- LLM 只输出 JSON（结构化 Spec）
- 超出能力时主动简化（由服务端在 `assistant` 消息中向用户说明）

## 6.4 校验与修复流程

1. 调用模型
2. 解析 JSON
3. 执行 Spec Validator
4. 失败则构造修复 Prompt（包含错误原因）
5. 再次解析与校验
6. 成功则保存为 `projects.draft_spec_json`

失败处理：

- 写入 `chat_messages`（`role=system`）错误信息
- 草稿保持上一个成功版本不变

---

# 7. 多轮对话与 AI 可视化（前端临时效果）

## 7.1 多轮对话策略

- 每轮用户输入都写入 `chat_messages`
- 每轮 AI 回复也写入 `chat_messages`
- 系统基于历史对话 + 当前草稿重生成
- 不保留版本树，仅保留对话记录 + 当前草稿/已发布状态

## 7.2 AI 可视化数据来源

P0 不持久化 AI 可视化状态，前端基于请求生命周期展示临时动画：

- 提交后立即显示 loading / typing 动画
- 请求处理中显示“AI 正在生成/校验/渲染”文案（可轮播）
- 请求完成后收起动画并刷新预览/聊天区
- 请求失败后显示错误提示（错误信息来自接口响应或 `system` 消息）

推荐前端展示文案（示例）：

1. `正在理解你的需求...`
2. `正在生成应用结构...`
3. `正在校验并渲染预览...`
4. `已完成` / `生成失败`

说明：

- 这些状态仅用于交互反馈，不要求可追溯回放。
- Share 页面不展示真实阶段回放，仅展示聊天记录与当前结果。

---

# 8. 渲染与 CRUD 实现

## 8.1 渲染入口

- 项目编辑页（登录态）：渲染 `draft_spec_json`
- 发布页 `/p/{slug}`：渲染 `published_spec_json`
- 分享页 `/share/{slug}`：展示聊天记录与只读预览

建议实现统一渲染函数：

- `RenderApp(spec, mode)`
- `mode`: `draft` / `published` / `share_readonly`

## 8.2 Spec 渲染流程

1. 读取目标 Spec（草稿或发布）
2. 校验 Spec（保险校验）
3. 同步 collections（仅兼容变更）
4. 遍历 pages
5. 遍历 blocks
6. 根据 `block.type` 调用对应 renderer
7. 输出 HTML（HTMX 局部更新）

## 8.3 Collection 同步策略（P0 很关键）

为降低数据一致性风险，P0 只允许自动执行兼容变更：

- 新增 collection：允许
- 新增字段：允许（旧 records 缺失字段按空值处理）
- 调整页面布局：允许（不影响数据）

不兼容变更（默认拒绝）：

- 删除 collection
- 删除字段
- 修改字段类型（如 `text -> int`）
- 将非枚举字段改为枚举且不兼容现有数据

处理方式：

- 生成失败或草稿同步失败
- 返回明确错误提示，引导用户重命名字段/集合而不是修改类型

## 8.4 CRUD API 规则

- 所有写入都必须校验字段白名单与类型
- `data_json` 写入前做类型归一化（如 `int`/`real`/`bool`）
- Share 模式下禁止一切写操作（服务端返回 403）
- 发布页是否可写：P0 默认允许（即发布应用可被使用）

---

# 9. 认证与会话（简单版）

## 9.1 登录注册范围

- 本地账号密码注册
- 本地账号密码登录
- 退出登录

不做：

- OAuth
- 忘记密码
- 邮箱验证
- 多因素认证

## 9.2 密码与会话安全

- 密码使用 bcrypt 存储
- Session token 使用随机高熵字符串
- Cookie 设置 `HttpOnly`
- 建议开启 `SameSite=Lax`
- 生产环境启用 `Secure`

---

# 10. 发布与分享机制（双态）

## 10.1 发布（Publish）

发布动作逻辑：

1. 检查 `draft_spec_json` 存在且合法
2. 若 `published_slug` 为空则生成 slug
3. 复制 `draft_spec_json -> published_spec_json`
4. 更新 `published_at`

说明：

- 每个项目只有一个当前发布态
- 再次发布覆盖原发布内容（不保留历史）

## 10.2 分享（Share）

分享动作逻辑：

1. 若 `share_slug` 为空则生成 slug
2. 返回 `/share/{slug}`

分享页展示（P0）：

- 对话记录（`chat_messages`，含用户与 AI 回复）
- 当前草稿预览（只读）
- 当前发布预览/状态（若存在）

## 10.3 只读保护（必须后端落实）

分享页的“只读”必须由后端路由/接口保障：

- `/share/*` 页面中的表单提交接口全部禁用
- CRUD 写接口校验请求上下文模式，`share_readonly` 直接拒绝
- 前端禁用按钮仅作为体验增强，不作为安全措施

---

# 11. Showcase 预留（P0 不实现）

保留：

- `projects.is_showcase` 字段
- 首页入口占位（可选）

不做：

- 预置项目 seed 数据
- Showcase 列表页
- Remix 流程

说明：

- 后续开发完成后可手动更新 `is_showcase` 标识。

---

# 12. 部署方案（Fly.io + SQLite）

## 12.1 部署形态

- Go 单二进制
- SQLite 文件持久化
- Fly.io 单实例部署（P0）
- 挂载 volume 持久化 SQLite 文件

## 12.2 环境变量

- `DEEPSEEK_API_KEY`（或兼容 LLM Key）
- `DATABASE_PATH`
- `SESSION_SECRET`
- `APP_BASE_URL`

## 12.3 运行约束说明

- SQLite 方案适合单实例演示环境
- P0 不做多实例写入协调
- 后续若扩展需迁移到 Postgres 或引入单写架构

---

# 13. 实施建议（工程落地）

## 13.1 开发优先级

1. 认证 + 项目骨架
2. Spec 生成与校验
3. 基础渲染（List/Form/Toggle/Stats）
4. 多轮对话 + AI 阶段展示
5. 发布与分享
6. Timer（若时间允许可简化）

## 13.2 工程质量要求（与仓库规范对齐）

- Go 代码遵循 `gofmt`、`go vet`
- 单元测试优先（至少覆盖 Validator、生成流程、发布/分享权限）
- 提供 `Makefile`（`build/test/lint/dev`）
- 完成前执行 build/test/lint
- Web 流程建议补 Playwright e2e（生成、发布、分享只读）

---

# 14. 风险与缓解

## 14.1 LLM 输出不稳定

风险：

- 非法 JSON
- 超出字段/原语限制

缓解：

- 强约束 Prompt
- JSON 解析 + Validator
- 修复重试一次
- 失败时保留旧草稿

## 14.2 Schema 演进导致数据不兼容

风险：

- 多轮迭代后字段类型变化导致旧数据无法渲染/统计

缓解：

- P0 仅允许兼容变更自动同步
- 不兼容变更直接拒绝并提示重命名方案

## 14.3 分享页误写数据

风险：

- 仅前端禁用造成后端仍可写

缓解：

- 写接口统一鉴权 + 模式检查
- `share_readonly` 直接返回 403

---

# 15. 后续扩展方向（P1+）

- 完整版本管理（历史版本、切换、回滚、Diff）
- Showcase 页面与 Remix
- 模板市场
- 更丰富原语（图表、过滤器、关系视图）
- 更强权限模型
- 动态代码生成 / 沙箱执行（若未来需要）
