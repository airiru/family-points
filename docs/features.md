# 家庭积分榜 · 功能清单 v2

> 状态：`✅ 已实现` / `⬜ 待实现` / `⚠️ 需确认` / `⛔ 已知缺口`
> 最后更新：2026-11-02

## v2 架构变更（相对 v1）

| 维度 | v1 | v2 |
|---|---|---|
| 存储 | Cloudflare KV，单键 `state` 存全量 JSON | **Cloudflare D1（SQLite）**，关系表 |
| 后端 | JavaScript（Worker 单文件） | **JavaScript（不变，数据访问层重写）** |
| 客户端 | 仅 Web（Vue 3 + Vite） | **Web（保留）+ Android 原生（Kotlin，全量功能）** |
| 租户模型 | 单家庭 | **多租户：多用户，每用户仅属一个家庭组** |
| 屏幕时间 | 无 | **Android 原生采集 + 上报 + 自动计分** |

> **前提假设**：Web 端保留，与 Android 端并存，功能对齐。若 Web 端要下线，需另行确认。

---

## 1. 架构总览

```
┌──────────────┐   ┌──────────────────┐
│  Web (Vue3)  │   │  Android (Kotlin)│
│  浏览器      │   │  WebView 不采用  │
└──────┬───────┘   └────────┬─────────┘
       │  HTTPS / JSON      │  + 屏幕时间采集
       └────────┬───────────┘
                ▼
        ┌───────────────┐
        │ Worker (JS)   │  单一后端，服务双端
        │ 鉴权 / 业务    │
        └───────┬───────┘
                ▼
        ┌───────────────┐
        │ D1 (SQLite)   │  多租户，family_id 隔离
        └───────────────┘
```

**核心约定**：接口响应统一 `{ ok, data?, error? }`。双端共用同一套 API，**不为某一端开后门**。

---

## 2. 数据模型（D1 Schema）

```sql
-- 家庭组
families (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  invite_code TEXT NOT NULL UNIQUE,   -- 加入用，可重置
  owner_member_id TEXT,               -- 创建者
  created_at INTEGER NOT NULL
)

-- 成员（账号即成员：username 非空即可登录）
members (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL REFERENCES families(id),
  name TEXT NOT NULL,
  score INTEGER NOT NULL DEFAULT 0,
  hidden INTEGER NOT NULL DEFAULT 0,
  username TEXT UNIQUE,               -- 全局唯一 ⇒ 天然保证「一人只属一组」
  salt TEXT, pass_hash TEXT,
  role TEXT NOT NULL DEFAULT 'member',-- 'admin' | 'member'
  streak_reset_at TEXT,               -- JSON: { 规则名: 毫秒时间戳 }
  created_at INTEGER NOT NULL
)

rules (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  name TEXT NOT NULL,
  points INTEGER,                     -- NULL 表示灵活规则
  flex INTEGER NOT NULL DEFAULT 0,
  streaks TEXT,                       -- JSON: [{every, bonus}]
  created_at INTEGER NOT NULL
)

items (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  name TEXT NOT NULL,
  cost INTEGER NOT NULL,
  created_at INTEGER NOT NULL
)

records (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  member_id TEXT NOT NULL,
  member_name TEXT NOT NULL,          -- 冗余，避免改名后历史错乱
  title TEXT NOT NULL,
  points INTEGER NOT NULL,
  time INTEGER NOT NULL,              -- 事项发生时间
  added_at INTEGER NOT NULL,          -- 实际写入时间（补记判定）
  streak_award TEXT,                  -- JSON: {every, n, from}
  source TEXT NOT NULL DEFAULT 'manual' -- 'manual' | 'screen' | 'device'
)

-- 屏幕时间规则（每成员一条）
screen_rules (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  member_id TEXT NOT NULL UNIQUE,
  threshold_min INTEGER NOT NULL,
  bonus_points INTEGER NOT NULL,
  penalty_points INTEGER NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1
)

-- 屏幕时间上报结算记录（幂等载体）
screen_reports (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  member_id TEXT NOT NULL,
  date TEXT NOT NULL,                 -- YYYY-MM-DD
  total_min INTEGER NOT NULL,
  points INTEGER NOT NULL,
  record_id TEXT,
  reported_at INTEGER NOT NULL,
  UNIQUE(member_id, date)             -- ← 幂等键，数据库层兜底
)

-- 设备令牌
devices (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  member_id TEXT NOT NULL,
  name TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  revoked INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  last_seen_at INTEGER
)

-- 索引（免费版 50 查询/请求，索引是降低 rows_read 的关键）
CREATE INDEX idx_records_family_time ON records(family_id, time DESC);
CREATE INDEX idx_records_member_time  ON records(member_id, time DESC);
CREATE INDEX idx_members_family       ON members(family_id);
CREATE INDEX idx_rules_family         ON rules(family_id);
CREATE INDEX idx_items_family         ON items(family_id);
CREATE INDEX idx_devices_token        ON devices(token_hash);
```

**三条设计要点：**

1. **`username` 全局唯一**是实现「一个用户只能加入一个家庭组」的最简手段——不需要额外的成员关系表。
2. **`records.member_name` 冗余存储**：现有实现里记录名来自成员当前名字，改名会污染历史。v2 改为快照。
3. **`UNIQUE(member_id, date)`** 把幂等约束下沉到数据库，应用层重试不会产生重复行。

---

## 3. 角色与权限矩阵

| 角色 | 范围 | 查看 | 自助兑换 | 后台 | 设备上报 |
|---|---|---|---|---|---|
| 未登录 | — | ❌ | ❌ | ❌ | ❌ |
| 普通成员 | 本组 | ✅ 仅本组 | ✅ 仅自己 | ❌ | ❌ |
| 管理员 | 本组 | ✅ 仅本组 | ✅ | ✅ 仅本组 | ❌ |
| 设备 | 本组单成员 | ❌ | ❌ | ❌ | ✅ 仅绑定成员 |

**越权防护硬规则**：所有 SQL **必须带 `family_id = ?` 条件**，且 `family_id` 只能从 token 解析结果取，**禁止从请求体取**。这是多租户最容易出事故的地方。

---

## 4. 家庭组与账号

| # | 功能 | 状态 | 说明 |
|---|---|---|---|
| 4.1 | 注册即创建家庭组 | ⬜ | 用户名 + 密码 + 家庭组名，创建者自动为 admin |
| 4.2 | 邀请码加入家庭组 | ⬜ | 输入邀请码加入，默认角色 member |
| 4.3 | 一个用户仅属一个家庭组 | ⬜ | `members.username` 全局唯一约束保证 |
| 4.4 | 查看/重置邀请码 | ⬜ | 仅 admin |
| 4.5 | 退出家庭组 | ⚠️ | 需定：退出后成员记录保留还是带走？建议保留在原组并解除账号绑定 |
| 4.6 | 家庭组改名 | ⬜ | 仅 admin |
| 4.7 | 转让管理员 | ⚠️ | 建议实现，否则唯一 admin 丢失将无人可管理 |
| 4.8 | 解散家庭组 | ⚠️ | 高危操作，需二次确认 + 输入组名确认 |
| 4.9 | 登录 / 退出 | ⬜ | token 机制沿用 v1（HMAC-SHA256，30 天） |
| 4.10 | token 续期 | ⛔ | v1 遗留缺口，v2 仍未解决，Android 端到期会静默失败 |

---

## 5. 成员管理

| # | 功能 | 状态 |
|---|---|---|
| 5.1 | 添加成员（可无账号） | ⬜ |
| 5.2 | 删除成员（连带记录） | ⬜ |
| 5.3 | 隐藏/显示成员 | ⬜ |
| 5.4 | 积分清零 | ⬜ |
| 5.5 | 查看成员全部记录 | ⬜ 分页加载 |
| 5.6 | 为成员设置登录与角色 | ⬜ |
| 5.7 | **成员改名** | ⬜ v2 新增（v1 缺失），历史记录因 `member_name` 快照不受影响 |
| 5.8 | 成员排序 | ⬜ 按积分降序 |

---

## 6. 计分规则

| # | 功能 | 状态 |
|---|---|---|
| 6.1 | 固定分值规则 | ⬜ |
| 6.2 | 灵活规则（记一笔时填分值） | ⬜ |
| 6.3 | 规则编辑 / 删除 | ⬜ |
| 6.4 | 连续打卡奖励（多档） | ⬜ `streaks` JSON |
| 6.5 | 里程碑只奖励一次 | ⬜ 以 `streak_award` 去重 |
| 6.6 | 违反时重置连续计数 | ⬜ 名称互相包含即视为相关 |
| 6.7 | 按自然日去重打卡 | ⚠️ v1 按「次数」计数，文案已改称"次"。v2 是否改按「天」需确认 |

---

## 7. 记一笔

| # | 功能 | 状态 |
|---|---|---|
| 7.1 | 按规则一键记分 | ⬜ |
| 7.2 | 灵活规则填分值 | ⬜ |
| 7.3 | 自定义事项记分 | ⬜ |
| 7.4 | 补记历史日期 | ⬜ 东八区当日 12:00 折算，不接受未来 |
| 7.5 | 补记标识（`added_at - time > 12h`） | ⬜ |
| 7.6 | 0 分中性记录 | ⬜ |
| 7.7 | **撤销 / 删除单条记录** | ⬜ v2 新增（v1 缺失），删除需回滚 `score` 并重算连续计数 |

---

## 8. 兑换商城

| # | 功能 | 状态 |
|---|---|---|
| 8.1 | 物品增删改 | ⬜ |
| 8.2 | 管理员代任意成员兑换 | ⬜ |
| 8.3 | 成员自助兑换 | ⬜ |
| 8.4 | 余额校验（前后端双重） | ⬜ |
| 8.5 | 兑换记录可撤销 | ⬜ v2 新增 |

---

## 9. 展示与记录

| # | 功能 | 状态 |
|---|---|---|
| 9.1 | 积分排行榜（🥇🥈🥉） | ⬜ |
| 9.2 | 连续打卡展示（🔥） | ⬜ |
| 9.3 | 记录列表分页 | ⬜ **改为服务端分页**（v1 是前端分页，全量下发） |
| 9.4 | 按成员 / 日期范围筛选 | ⬜ 日期范围 v2 新增 |
| 9.5 | 记录总量上限 | ⬜ **取消 500 条截断**（D1 无此约束） |
| 9.6 | 记录搜索 | ⬜ v2 新增 |
| 9.7 | 积分趋势图 | ⬜ v2 新增（按周/月） |

> **重要优化**：`/api/state` 不再返回全量 `records`。改为返回 members / rules / items / screenRules + **最近 N 条**记录，历史记录走独立分页接口。否则每次请求都会扫全表，`rows_read` 迅速耗尽 5M/天。

---

## 10. 屏幕时间上报（Android 原生）

### 10.1 已定语义

| 决策项 | 结论 |
|---|---|
| 计分方向 | 双向：当日总时长 ≤ 阈值 → 加分；> 阈值 → 减分 |
| 迟到数据 | 补算但不回滚：按原日期补记，已发出的分不动 |
| 幂等 | `UNIQUE(member_id, date)` + 应用层查重 |
| 认证 | 设备令牌（`x-device` 头），独立于用户账号 |
| 结算锁定 | 某天首次上报即锁定结果，后续同日更新不改变（⚠️ 见 17.1） |

### 10.2 功能项

| # | 功能 | 状态 |
|---|---|---|
| 10.2.1 | 权限检查与引导（`PACKAGE_USAGE_STATS`） | ⬜ |
| 10.2.2 | 采集当日总时长（`queryEvents` 配对 RESUMED/PAUSED） | ⬜ **不用 `INTERVAL_DAILY`**，会按自然日切桶导致跨天失真 |
| 10.2.3 | 排除系统应用与息屏时段 | ⬜ |
| 10.2.4 | 每日定时上报（WorkManager） | ⬜ |
| 10.2.5 | 打开 App 时补算最近 7 天 | ⬜ **可靠性主要来源** |
| 10.2.6 | 失败重试（指数退避） | ⬜ 依赖幂等，重试安全 |
| 10.2.7 | 上报状态展示（最近成功时间、待上报天数） | ⬜ |
| 10.2.8 | 国产 ROM 保活引导 | ⬜ 仅提示，不强求 |
| 10.2.9 | 分时段明细上报 | ⬜ D1 已解除 KV 写入瓶颈，可支持（⚠️ 见 17.3） |

### 10.3 结算逻辑

```
1. 校验设备令牌 → 得到 (family_id, member_id)
2. 查 screen_rules 是否存在且 enabled
3. 查 screen_reports 是否已有 (member_id, date)
   └─ 有 → 返回 { settled:false, reason:'已结算' }
4. pts = total_min <= threshold_min ? +bonus_points : -penalty_points
5. 事务内：更新 members.score / 插入 records / 插入 screen_reports
```

**必须用 `db.batch()` 包成一次请求**：免费版单次调用上限 50 条查询，且 batch 只计 1 次。

---

## 11. 设备管理（后台）

| # | 功能 | 状态 |
|---|---|---|
| 11.1 | 设备列表（名称、绑定成员、最后上报时间） | ⬜ |
| 11.2 | 生成令牌（明文仅展示一次，库中只存哈希） | ⬜ |
| 11.3 | 吊销设备（立即失效） | ⬜ |
| 11.4 | 设备改名 | ⬜ |
| 11.5 | 每成员可绑定的设备数 | ⚠️ 建议 1–2 台 |

---

## 12. Android App 界面清单（Kotlin 原生）

### 12.1 公开区（未登录）

| # | 界面 | 关键元素 |
|---|---|---|
| 12.1.1 | 启动页 | 检查本地 token，有效则直进主页 |
| 12.1.2 | 登录页 | 用户名、密码、登录按钮、去注册 |
| 12.1.3 | 注册页 | 用户名、密码、家庭组名（新建）或邀请码（加入） |
| 12.1.4 | 屏幕时间权限引导 | 说明用途、跳转系统设置、检测授权状态 |

### 12.2 主区（登录后）

| # | 界面 | 对应 v1 功能 |
|---|---|---|
| 12.2.1 | 积分榜 | 排名、奖牌、成员积分、连续次数 |
| 12.2.2 | 成员详情 | 该成员全部记录（分页） |
| 12.2.3 | 最近记录 | 分页、按成员筛选 |
| 12.2.4 | 我的兑换 | 商城列表、余额校验、确认兑换 |
| 12.2.5 | 设置 | 账号信息、家庭组信息、退出登录、上报状态入口 |

### 12.3 后台区（仅 admin）

| # | 界面 | 对应 v1 功能 |
|---|---|---|
| 12.3.1 | 成员管理 | 增删、隐藏、清零、改名、登录设置 |
| 12.3.2 | 计分规则 | 增删改、灵活规则、连续奖励多档 |
| 12.3.3 | 记一笔 | 选成员、选规则、补记日期、自定义事项 |
| 12.3.4 | 兑换商城 | 物品增删改、代兑换、撤销 |
| 12.3.5 | 记录 | 全量分页、筛选、搜索、删除 |
| 12.3.6 | 屏幕时间规则 | 按成员配置阈值/加分/减分 |
| 12.3.7 | 设备管理 | 列表、生成令牌、吊销 |
| 12.3.8 | 家庭组设置 | 组名、邀请码、成员角色、转让管理员 |

**共 17 个界面。** 这是本次改造中工作量最大的部分——v1 的 Web 端全部 UI 需在 Compose 中重写一遍。

---

## 13. Web 端

| # | 功能 | 状态 |
|---|---|---|
| 13.1 | 保留现有 Vue 3 页面结构 | ⬜ 适配 D1 接口 |
| 13.2 | 新增家庭组注册/加入流程 | ⬜ |
| 13.3 | 新增屏幕时间规则与设备管理 Tab | ⬜ |
| 13.4 | 记录改为服务端分页 | ⬜ |

---

## 14. 接口清单

| # | 接口 | 方法 | 鉴权 | 状态 |
|---|---|---|---|---|
| 14.1 | `/api/setup` | POST | 无 | ⬜ 改造：创建家庭组 + admin |
| 14.2 | `/api/register` | POST | 无 | ⬜ 新增 |
| 14.3 | `/api/join` | POST | 无 | ⬜ 新增（邀请码） |
| 14.4 | `/api/login` | POST | 无 | ⬜ |
| 14.5 | `/api/state` | GET | token | ⬜ 改造：不含全量 records |
| 14.6 | `/api/records` | GET | token | ⬜ 新增：分页/筛选/搜索 |
| 14.7 | `/api/member/*` | POST | admin | ⬜ add/setLogin/del/hide/reset/rename |
| 14.8 | `/api/rule/*` | POST | admin | ⬜ add/edit/del |
| 14.9 | `/api/score/*` | POST | admin | ⬜ add/custom/del |
| 14.10 | `/api/item/*` | POST | admin | ⬜ add/edit/del/redeem/revoke |
| 14.11 | `/api/item/redeemSelf` | POST | member | ⬜ |
| 14.12 | `/api/family/*` | POST | admin | ⬜ rename/invite/reset/transfer |
| 14.13 | `/api/screenRule/*` | POST | admin | ⬜ save/del |
| 14.14 | `/api/device/*` | POST | admin | ⬜ add/list/revoke/rename |
| 14.15 | `/api/device/report` | POST | **x-device** | ⬜ 设备专用 |

---

## 15. 迁移方案（KV → D1）

| 步骤 | 内容 | 状态 |
|---|---|---|
| 15.1 | 建 D1 库，执行 schema | ⬜ |
| 15.2 | 编写一次性迁移脚本：读 KV `state` → 写入 D1 | ⬜ |
| 15.3 | 迁移时生成默认家庭组，所有数据归入该组 | ⬜ |
| 15.4 | 校验：成员数、记录数、积分总和逐项比对 | ⬜ **不可只比对行数** |
| 15.5 | 保留 KV 只读 7 天作为回滚路径 | ⬜ |
| 15.6 | 切换 `wrangler.toml` 绑定，移除 KV | ⬜ |

> 迁移脚本必须幂等（可重复执行），且**先在生产数据的副本上跑一遍**。

---

## 16. 平台额度约束（D1 免费版）

| 项目 | 免费额度 | 影响 |
|---|---|---|
| Rows read | 5,000,000/天 | 全量下发 records 会迅速耗尽 → 必须服务端分页 |
| Rows written | 100,000/天 | 充足 |
| **Queries / invocation** | **50** | 复杂聚合需合并查询，写操作用 `batch()` |
| 单库容量 | 500 MB | 家庭场景远用不到 |
| 数据库数 | 10 | 单库多表即可，无需每户一库 |
| 单库并发 | 单线程、查询串行 | 家庭场景无感 |
| Workers 请求 | 100,000/天 | 充足 |
| Workers CPU | 10 ms/次 | D1 查询等待不计 CPU，安全 |

---

## 17. 待决策项

| # | 事项 | 影响 |
|---|---|---|
| 17.1 | 屏幕时间是否允许「当天内覆盖」 | 决定结算分支：首次锁定 vs 当日可更新 |
| 17.2 | 退出家庭组时记录如何处理 | 保留 / 带走 / 删除 |
| 17.3 | 是否需要分时段明细上报 | 影响表结构与上报体量 |
| 17.4 | 连续打卡按「次数」还是「自然日」 | 影响结算逻辑与文案 |
| 17.5 | Web 端是否长期保留 | 决定是否双端维护 UI |
| 17.6 | 是否开放公开注册 | 决定是否需要防滥用（验证码/限流） |
| 17.7 | 单家庭组人数上限 | 影响列表性能与 UI |

---

## 18. 验收要点

**多租户隔离（最高优先级）**
1. A 组 admin 无法通过任何接口读写 B 组数据（逐个接口测）。
2. 篡改请求体中的 `family_id` 无效——服务端只认 token。
3. 一个 username 无法同时存在于两个家庭组。

**D1 正确性**
4. 同一份屏幕时间重复上报 10 次，积分只变一次。
5. 断网 3 天后恢复，3 天数据全部补记且日期正确。
6. 删除一条记录后，成员积分与连续计数均正确回滚。
7. 老数据迁移后，成员数/记录数/积分总和与迁移前逐项一致。

**Android**
8. 未授予 `PACKAGE_USAGE_STATS` 时给出明确引导，不崩溃。
9. 国产 ROM 杀后台后，打开 App 能补齐遗漏日期。
10. 设备令牌被吊销后，上报立即返回 401 且不写入数据。

**额度**
11. 单次请求 SQL 数 ≤ 50；单次响应不含全量记录。
