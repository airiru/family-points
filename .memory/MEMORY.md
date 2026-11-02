# 家庭积分榜 · 项目记忆（提炼层）

> 本文件是**每次会话开头必读**的提炼层，不是归档。上限约 50 KB，超了先归档再蒸馏。
> 日志在 `.memory/YYYY-MM-DD.md`，**追加式**，不要覆盖。
> 记忆是**线索不是事实**——凡能从磁盘验证的结论，动手前先去验证。

## 一、技术栈与运行形态（以磁盘为准）

| 层 | 实现 |
|---|---|
| 前端 | Vue 3.5 + Vite 7，`<script setup>` 单文件组件，hash 路由（无 vue-router） |
| 后端 | Cloudflare Worker，`worker/index.js` 单文件，导出 `fetch` |
| 存储 | Cloudflare KV，**单键** `state`（一个 JSON 存全部数据） |
| 部署 | **Cloudflare Pages**（`wrangler pages deploy`），非 Workers |

## 二、架构约定（改动前必须遵守）

1. **单键全量状态**：`STATE_KEY = 'state'`，结构 `{ members, rules, items, records, secret }`。
   任何写操作 = 读出整个 state → 改内存对象 → `saveState()` 整体写回。
   ⇒ 新增字段必须在 `emptyState()` 里给默认值，否则老数据读取时是 `undefined`。
2. **接口协议统一**：所有响应为 `{ ok, state?, error? }`。前端 `store.js` 的 `request()` 收到响应后
   **无条件 `Object.assign(state, j.state)`** —— 后端返回的 `state` 就是前端唯一数据源，
   不要在 action 里另造增量补丁。
3. **鉴权是无状态 token**：`username.exp.hmac(username.exp)`，HMAC-SHA256 签在 `state.secret` 上，
   有效期 30 天（`TOKEN_DAYS`）。**角色以成员当前配置为准**，不从 token 里读。
4. **权限收敛在后端**：`fetch` 里统一拦截 —— 非 `admin` 只放行 `item/redeemSelf` 一条写接口，
   前端隐藏按钮只是体验，**不是安全边界**。
5. **记录上限 500 条**（`MAX_RECORDS`），`saveState()` 里 `slice(-MAX_RECORDS)` 丢弃最旧。
   加任何"按全量记录统计"的逻辑时，要意识到历史会被截断。
6. **补记语义**：记录有 `time`（事项发生时间）与 `addedAt`（实际写入时间）两个字段。
   前端 `addedText()` 判定「相差 > 12 小时 = 补记」。补记日期按**东八区当天中午**折算。
7. **连续打卡**：`rule.streaks` 为数组（每项含连续天数 N 与奖励分 M）。
   口径以 worker 的 `streakCount()` / `streakResetAt()` 为准，前端 `memberStreak()` 是**同口径的展示副本**
   —— 改一处必须改另一处，否则前后端显示不一致。
8. **0 分是合法记录**（如「违反重新计数」说明），显示为中性样式，不计正分。`ptsClass()` 三态：plus/minus/空。

## 三、构建与部署链路

```
npm run build  = vite build && node scripts/postbuild.mjs
npm run deploy = npm run build && wrangler pages deploy
```

`postbuild.mjs` 的作用：把 `worker/index.js` 复制为 `dist/worker-api.js`，并生成 **Pages 高级模式**入口
`dist/_worker.js`（`/api/*` 交给 Worker，其余走 `env.ASSETS.fetch`）。
⇒ **删掉 postbuild 这一步，Pages 上所有 `/api/*` 会 404**，这是最容易被误删的隐性依赖。

## 四、记忆协议自身的落点

- 权威源：`.memory/skills/memory-handoff/SKILL.md`（进仓库，可 review / 可回溯）
- 已存在的工具级副本（当前 3 份，**每次会话现场重算，不要引用本清单**）：
  `~/.workbuddy-ai/skills/`、`~/.workbuddy/skills/`、`~/.dsh/skills/`
- 同步后必须逐份 `diff` 核验为空，不要只信脚本跑过了。

## 五、已知待办 / 未修

- **README.md 与代码已漂移**（2026-11-02 核验）：
  - 文中写 `wrangler.jsonc`，实际文件是 `wrangler.toml`
  - 文中写「`npm run deploy` = vite build + wrangler deploy」，实际是 `vite build && wrangler pages deploy`
  - 文中通篇称「Cloudflare Workers 部署」，实际部署目标是 **Pages**
  - 未提及 `scripts/postbuild.mjs` 这一步（见上，删了会 404）
  ⇒ 未修。改动属文档性质，需用户确认口径后再动。
