# Fly.io 部署说明（P0）

## 1. 前置条件

- 已安装并登录 `flyctl`
- 已创建 Fly App（或使用 `fly launch --no-deploy` 初始化）
- 已创建 volume（SQLite 持久化）

## 2. 环境变量与 Secrets

使用 `fly secrets set` 设置：

```bash
fly secrets set SESSION_SECRET='replace-with-strong-secret'
fly secrets set DEEPSEEK_API_KEY='sk-...'
```

`fly.toml` 中已配置：

- `APP_ENV=production`
- `APP_ADDR=:8080`
- `DATABASE_PATH=/data/mini-atoms-gorm.db`
- `APP_BASE_URL=https://<your-app>.fly.dev`

部署前请把 `fly.toml` 中的以下字段替换为你的实际值：

- `app`
- `[env].APP_BASE_URL`
- `primary_region`（可选）

### GitHub Actions（CI/CD）需要的 Secrets

如果使用仓库内的 GitHub Actions 自动部署（`deploy-fly.yml`），需要在 GitHub 仓库的 `Settings -> Secrets and variables -> Actions` 中配置：

- `FLY_API_TOKEN`：用于 `flyctl deploy`

生成方式（本地执行）：

```bash
fly auth token
```

说明：

- `SESSION_SECRET`、`DEEPSEEK_API_KEY` 仍然建议保存在 Fly 平台 Secrets（`fly secrets set ...`），不需要放到 GitHub Secrets（除非你希望在 workflow 中同步维护）。

## 3. 创建 SQLite Volume（首次）

```bash
fly volumes create mini_atoms_data --region sjc --size 1
```

如果你修改了 `fly.toml` 的 `mounts.source`，请同步修改这里的 volume 名称。

## 4. 部署

```bash
make deploy
```

或直接：

```bash
flyctl deploy
```

### GitHub Actions 自动部署触发规则

- `CI`：在 `push` / `pull_request` / 手动触发时执行 `lint + test + build`
- `Deploy Fly.io`：当 `master` 分支上的 `CI` 成功后自动触发部署
- `Deploy Fly.io` 也支持在 GitHub Actions 页面手动触发（可选指定 `ref`）

## 5. 验证

```bash
flyctl status
flyctl logs
curl -fsSL https://<your-app>.fly.dev/healthz
curl -fsSL https://<your-app>.fly.dev/readyz
```

## 6. 说明

- P0 使用 SQLite + 单实例 + volume
- 分享页只读由服务端路由拦截保障
- 若未配置 `DEEPSEEK_API_KEY`，服务端会自动回退本地 Stub 生成器
