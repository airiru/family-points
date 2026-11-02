# 家庭积分榜 · 项目记忆（提炼层）

> 本文件是**每次会话开头必读**的提炼层，不是归档。上限约 50 KB，超了先归档再蒸馏。
> 日志在 `.memory/YYYY-MM-DD.md`，**追加式**，不要覆盖。
> 记忆是**线索不是事实**——凡能从磁盘验证的结论，动手前先去验证。

## 一、技术栈与运行形态（**当前代码实况 = v1**）

> ⚠️ 本节描述的是**磁盘上正在运行的代码**。v2 目标架构见第二节，**尚未实现**。
> 不要把 v2 的设计当成既有事实——动手前先确认代码里到底是不是 D1。

| 层 | 实现 |
|---|---|
| 前端 | Vue 3.5 + Vite 7，`<script setup>` 单文件组件，hash 路由（无 vue-router） |
| 后端 | Cloudflare Worker，`worker/index.js` 单文件，导出 `fetch` |
| 存储 | Cloudflare KV，**单键** `state`（一个 JSON 存全部数据） |
| 部署 | **Cloudflare Pages**（`wrangler pages deploy`），非 Workers |

## 一之二、v3 目标架构（2026-11-02 用户拍板，**未实现**）

> 架构已被变更三次：v1 KV → v2 D1 → **v3 自建 MySQL/Go**。**v2 方案已作废**，不要参考。

| 维度 | v1（代码现状） | v3（目标） |
|---|---|---|
| 存储 | KV 单键 `state` | **MySQL 8**（InnoDB / utf8mb4） |
| 后端 | Cloudflare Worker (JS) | **Go + Gin + sqlx**，自建服务器 |
| Web | 前端应用（双角色） | **管理后台**（仅 `admins`，无成员视图） |
| Android | 无 | **Kotlin + Compose 原生**（成员端，12 个界面，1 个仅组长可见） |
| 部署 | Cloudflare Pages | **自建服务器**：Nginx + Docker Compose |

**已定决策**（用户明确选择，不要重新讨论）：
1. 数据库 **MySQL**；后端 **Go**；Android **Kotlin 原生**；后台 **Web**。
2. **后台管理功能只在 Web**，Android 不承载后台管理界面。
3. 家庭组 = **多租户**，一个用户只能加入一个家庭组（`members.username` 全局唯一保证）。
4. 屏幕时间计分**双向**（达标加分、超标减分），迟到数据**补算但不回滚**。
5. **后台与前台是两套独立账号系统**：`admins`（Web 管理员，不参与积分）与
   `members`（Android 组长/成员，参与积分），互不通用、token 类型隔离。
6. **前台没有管理员，只有组长 `leader`**：可发布/审核任务、记一笔；
   不能改成员/规则/设备、不能看审计（那是后台专属）。

**v3 相对 v1 的关键变化**：取消 500 条记录截断；密码 SHA-256 → **bcrypt**（需登录时惰性迁移）；
Token 改 **Access 2h + Refresh 30d**；并发兑换需事务 + `FOR UPDATE` 防超兑；
**数据安全责任转移到自建服务器**（备份是必做项，不是可选项）。

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

### v3 强制约定（实现时必须遵守，否则会出安全或数据事故）

9. **多租户隔离是最高优先级**：所有 SQL **必须带 `family_id = ?`**，且 `family_id` **只能从 JWT claims 取**，
   **禁止从请求体、query、路径参数取**。跨组访问返回 **404 而非 403**（避免泄露资源存在性）。
10. **并发写必须走事务**：兑换扣分、屏幕时间结算都要 `SELECT ... FOR UPDATE`，
   否则 MySQL 下会出现负余额（v1 的 KV 单键没有这个问题，迁移后是新风险）。
11. **屏幕时间用「后报覆盖」，不是首次锁定**（19.3 已于 2026-11-02 拍板）：
    `UNIQUE(member_id, report_date)` 保证一天一行，新上报**覆盖**旧值并**重算积分**
    （回滚旧积分 → 删旧记录 → 按新值结算 → `revision+1`）。
    **上报值是「当天累计总时长」，不是增量**——客户端传增量、服务端再累加，会导致重复计分。
    数值未变时直接返回，不要无条件 `revision+1`（设备重试会刷大 revision）。
12. **`/api/state` 不得返回全量 records**：必须服务端分页（v1 是前端分页全量下发）。
13. **记录名快照**：`records.member_name` 冗余存储，避免成员改名污染历史（v1 的缺陷）。
14. **密码用 bcrypt**，但旧 SHA-256 哈希无法换算 ⇒ 登录时校验通过后**惰性改写**，保留一段兼容分支。
15. **积分赠送是「转移」不是「计分」**：家庭总积分守恒，不参与连续打卡计数，也不触发违反重置。
    实现必须**按 `members.id` 升序加锁**（否则 A↔B 互赠并发会死锁），余额校验必须在事务内。
    `amount` 恒为正整数，方向由 `from/to` 表达，不接受负数。
    **唯一约束是转出方余额**——不设单笔/每日上限；管理员代赠同样不能凭空生成积分。
16. **任务审核通过必须复用 `score/add` 的逻辑**，不要另写计分代码——否则连续奖励、每日上限、
    违反重置会出现两套口径。审核幂等靠**条件更新**（`status` 从 `submitted` 改为 `approved`，
    受影响行数为 0 即已处理）。
17. **审计日志三条铁律**：① append-only，不提供 UPDATE/DELETE；② **`before/after` JSON 必须剔除
    `pass_hash`**，否则密码哈希进了可查询的表；③ **写入失败不得阻断主流程**（旁路，失败记应用日志即可）。
18. **规则的两个新约束**：`member_ids`（NULL=全组）与 `daily_limit`（NULL=不限），
    后者校验**必须在事务内**，先查后写在并发下会超限。
19. **屏幕时间有两条写入路径**：设备自动上报（`source='auto'`）与手动上传（`source='manual'`），
    共用 `UNIQUE(member_id, report_date)`，**同一天只能存在一条结算记录**。
    **来源不影响覆盖行为**——auto 与 manual 一视同仁，以最新值为准；
    权限只约束「谁能发起手动上传」（默认仅 admin，见 19.17）。
    覆盖必须「回滚旧积分 → 删旧记录 → 按新值重算」，**不能只改数字，更不能新旧相加**。
    **覆盖导致积分下降是正常行为**，不是 bug。
20. **连续打卡是 `screen_night` 规则的专属能力**（19.4 已定，2026-11-02 进一步收紧）：
    手动规则（做家务、练琴等）**既没有连续打卡，也不统计完成次数**，记几次就是几次。
    ⇒ v1 的 `resetStreaksOnViolation` / `namesRelated` 那套「按名称相关重置」逻辑**整段废弃**。
    ⇒ 早期设想的 `daily_limit`（每人每日计分上限）**取消**，手动规则可无限次记录。
    连续计数按**自然日**，一天最多计 1 次。
21. **「按时睡觉」由屏幕时间自动判定，没有手动入口**：
    `rules.trigger_type = 'screen_night'` 的规则**不出现在「记一笔」列表**，
    只能由 12.5 的夜间判定写入。`rules.streaks` 字段保留但**仅对 screen_night 生效**。
    **连续奖励档位全部可配置**（用户 2026-11-02 明确）：结构 `[{every, bonus, mode}]`，
    `every`=连续天数阈值、`bonus`=奖励分、`mode`=`repeat`（每满 N 天发一次）/`once`（本轮只发一次）；
    **档位数不限（建议 ≤5）、多档独立计算、同时命中则都发**。改配置**不追溯历史**。
    判定模型是「**可配置的睡觉时间点 + 夜间时段使用时长**」：
    `night_start_hour`（如 0 = 24:00 前放下手机）~ `night_end_hour`（如 7），
    该时段内 `night_min ≤ night_max_min` 即达标，`night_max_min` 建议默认 0。
    **当日未上报视为未达标、连续清零**——防「卸载 App 保连签」的有意取舍。
    **跨零点按开始日归属**：0:00–07:00 的数据记在**前一天**账上，否则当天无法结算。
    一次上报可同时产生日总量与夜间两条记录，`screen_reports` 用 `record_id` 与 `night_record_id`
    分别指向，**覆盖时必须回滚两条**，漏一个就留孤儿记录。
22. **后台与前台是两套独立账号系统**（2026-11-02 定）：
    `admins`（Web 管理员，**不参与积分、不出现在成员列表/排行榜**）与
    `members`（Android 组长/成员，参与积分）。两表 username 各自唯一、可重名。
    登录接口分离：`/api/admin/login` 与 `/api/app/login`。
    **Token 必须带类型并校验**：`/api/admin/*` 只收 `typ='admin'`，`/api/app/*` 只收 `typ='member'`，
    互打一律 401。
23. **前台没有管理员，只有组长 `leader`**：可发布任务、审核任务、记一笔；
    **不能**改成员/规则/设备、不能看审计——那是后台专属。
    首个成员自动为组长；后台可变更组长，**变更后旧组长立即失效**（不依赖重新登录）。
24. **注册入口只在后台**：Android **无注册页**，前台账号由管理员在后台创建后下发。
    Web 不提供成员只读视图（19.1 已关闭）。

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

## 五、功能口径的单一事实源

**`docs/features.md` 是功能清单的唯一依据**（2026-11-02 起）。它按「✅ 已实现 / ⬜ 待实现 /
⚠️ 需确认 / ⛔ 已知缺口」标注每一条，并含权限矩阵、接口清单、数据模型、验收要点。
**README 已过时，不要拿它当功能依据**；两者冲突时以 `features.md` 与代码为准。
新增或修改功能后，须同步更新该文件的对应条目——它是给人 review 的交付物，不是草稿。

## 六、已知待办 / 未修

- **v3 重构未开始**（2026-11-02）：MySQL/Go 后端、Web 后台改造、Android 原生 App 全部为 `⬜`。
  ⚠️ **架构已变更三次（KV → D1 → MySQL/Go），v2 的 D1 方案作废且零代码产出。**
  进入开发前应先冻结架构，否则每次变更只增加沉没成本。
  清单待决策项见 `docs/features.md` 第 18 节——**先定 18.3（屏幕时间是否允许当天内覆盖）再写结算逻辑**。
- **README.md 与代码已漂移**（2026-11-02 核验）：
  - 文中写 `wrangler.jsonc`，实际文件是 `wrangler.toml`
  - 文中写「`npm run deploy` = vite build + wrangler deploy」，实际是 `vite build && wrangler pages deploy`
  - 文中通篇称「Cloudflare Workers 部署」，实际部署目标是 **Pages**
  - 未提及 `scripts/postbuild.mjs` 这一步（见上，删了会 404）
  ⇒ 未修。改动属文档性质，需用户确认口径后再动。
