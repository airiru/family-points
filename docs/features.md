# 家庭积分榜 · 功能清单 v3

> 状态：`✅ 已实现` / `⬜ 待实现` / `⚠️ 需确认` / `⛔ 已知缺口`
> 最后更新：2026-11-02
> **v3 是自建服务器方案，v1（KV）与 v2（D1）的设计已全部作废。**

## 架构演进记录

| 版本 | 存储 | 后端 | 客户端 | 部署 | 状态 |
|---|---|---|---|---|---|
| v1 | Cloudflare KV | Worker (JS) | Web only | Cloudflare Pages | 代码现状 |
| v2 | Cloudflare D1 | Worker (JS) | Web + Android | Cloudflare Pages | ❌ 作废 |
| **v3** | **MySQL 8** | **Go** | **Web 后台 + Android 原生** | **自建服务器** | 本文档 |

> **警示**：三次变更意味着 v2 的 D1 schema、Cloudflare 绑定方案全部作废，且**零代码产出**。
> 建议本版确认后**冻结架构**再进入开发，否则每次变更都只增加沉没成本。

---

## 1. 架构总览

```
        ┌─────────────────────┐        ┌──────────────────────┐
        │  Web 管理后台        │        │  Android App         │
        │  Vue 3 + Vite       │        │  Kotlin + Compose    │
        │  管理员使用          │        │  家庭成员使用         │
        └──────────┬──────────┘        └───────────┬──────────┘
                   │  HTTPS / JSON                 │ + 屏幕时间采集
                   └───────────────┬───────────────┘
                                   ▼
                        ┌────────────────────┐
                        │  Nginx             │  TLS 终止 / 静态资源 / 反向代理
                        └─────────┬──────────┘
                                  ▼
                        ┌────────────────────┐
                        │  Go API (Gin)      │  127.0.0.1:8080
                        └─────────┬──────────┘
                                  ▼
                        ┌────────────────────┐
                        │  MySQL 8           │  127.0.0.1:3306
                        └────────────────────┘
```

**职责划分（本次关键变化）**：

- **Web = 管理后台**，面向管理员（成员管理、规则、记一笔、兑换、设备、家庭组）
- **Android = 成员端**，面向家庭成员（看积分、自助兑换、屏幕时间上报）
- ⇒ Android **不再承载后台管理界面**，界面数从 v2 的 17 个降至 **约 9 个**

> ⚠️ **假设**：Web 端是否保留「成员只读视图」需确认（见 18.1）。当前按「Web 只做后台」设计。

---

## 2. 技术选型（Go 生态）

| 层 | 选型 | 理由 |
|---|---|---|
| Web 框架 | **Gin** | 生态最成熟、中间件丰富、文档完善 |
| 数据库访问 | **sqlx**（手写 SQL） | 家庭项目 SQL 简单，可控性优于 ORM；若偏好 ORM 可换 GORM |
| 认证 | **JWT**（`golang-jwt/jwt/v5`） | 无状态，双端通用 |
| 密码 | **bcrypt**（cost 10） | 优于 v1 的 SHA-256 加盐 |
| 配置 | **env + viper** | 敏感信息走环境变量，不入库不入仓 |
| 日志 | **slog**（标准库） | Go 1.21+ 内置，结构化输出 |
| 迁移 | **golang-migrate** | 版本化 SQL 迁移，可回滚 |
| 部署 | **Docker Compose** | MySQL + API + Nginx 一键起，环境一致 |

**Go 项目结构建议**：

```
server/
├── cmd/api/main.go          # 入口
├── internal/
│   ├── config/              # 配置加载
│   ├── middleware/          # JWT 鉴权、CORS、Recover、请求日志
│   ├── handler/             # HTTP 层：参数校验、响应封装
│   ├── service/             # 业务逻辑：计分、结算、连续打卡
│   ├── repo/                # SQL 访问层
│   └── model/               # 结构体
├── migrations/              # 0001_init.up.sql / down.sql
├── go.mod
└── Dockerfile
```

---

## 3. 数据模型（MySQL 8）

```sql
-- 家庭组
CREATE TABLE families (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  name            VARCHAR(64)  NOT NULL,
  invite_code     CHAR(8)      NOT NULL UNIQUE,
  owner_member_id BIGINT UNSIGNED NULL,
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 成员（账号即成员：username 非空即可登录）
CREATE TABLE members (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id       BIGINT UNSIGNED NOT NULL,
  name            VARCHAR(32)  NOT NULL,
  score           INT          NOT NULL DEFAULT 0,
  hidden          TINYINT(1)   NOT NULL DEFAULT 0,
  username        VARCHAR(20)  NULL UNIQUE,   -- NULL 可重复，天然支持「部分成员无账号」
  pass_hash       VARCHAR(60)  NULL,          -- bcrypt
  role            ENUM('admin','member') NOT NULL DEFAULT 'member',
  streak_reset_at JSON         NULL,          -- { "规则名": "时间戳" }
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family (family_id),
  CONSTRAINT fk_members_family FOREIGN KEY (family_id) REFERENCES families(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 计分规则
CREATE TABLE rules (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id  BIGINT UNSIGNED NOT NULL,
  name       VARCHAR(64)  NOT NULL,
  points     INT          NULL,             -- NULL = 灵活规则
  flex       TINYINT(1)   NOT NULL DEFAULT 0,
  streaks    JSON         NULL,             -- [{every, bonus}]
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family (family_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 兑换物品
CREATE TABLE items (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id  BIGINT UNSIGNED NOT NULL,
  name       VARCHAR(64)  NOT NULL,
  cost       INT          NOT NULL,
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family (family_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 积分记录
CREATE TABLE records (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id    BIGINT UNSIGNED NOT NULL,
  member_id    BIGINT UNSIGNED NOT NULL,
  member_name  VARCHAR(32)  NOT NULL,       -- 快照，避免改名污染历史
  title        VARCHAR(128) NOT NULL,
  points       INT          NOT NULL,
  occurred_at  DATETIME(3)  NOT NULL,       -- 事项发生时间
  added_at     DATETIME(3)  NOT NULL,       -- 实际写入时间（补记判定）
  streak_award JSON         NULL,
  source       ENUM('manual','screen','device') NOT NULL DEFAULT 'manual',
  KEY idx_family_time (family_id, occurred_at DESC),
  KEY idx_member_time (member_id, occurred_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 屏幕时间规则（每成员一条）
CREATE TABLE screen_rules (
  id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id      BIGINT UNSIGNED NOT NULL,
  member_id      BIGINT UNSIGNED NOT NULL UNIQUE,
  threshold_min  INT NOT NULL,
  bonus_points   INT NOT NULL,
  penalty_points INT NOT NULL,
  enabled        TINYINT(1) NOT NULL DEFAULT 1
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 屏幕时间结算（幂等载体）
CREATE TABLE screen_reports (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id   BIGINT UNSIGNED NOT NULL,
  member_id   BIGINT UNSIGNED NOT NULL,
  report_date DATE         NOT NULL,
  total_min   INT          NOT NULL,
  points      INT          NOT NULL,
  record_id   BIGINT UNSIGNED NULL,
  reported_at DATETIME(3)  NOT NULL,
  UNIQUE KEY uk_member_date (member_id, report_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 设备令牌
CREATE TABLE devices (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id    BIGINT UNSIGNED NOT NULL,
  member_id    BIGINT UNSIGNED NOT NULL,
  name         VARCHAR(32)  NOT NULL,
  token_hash   CHAR(64)     NOT NULL,
  revoked      TINYINT(1)   NOT NULL DEFAULT 0,
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  last_seen_at DATETIME(3)  NULL,
  KEY idx_token (token_hash),
  KEY idx_family (family_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

**四条设计要点：**

1. **`username` 可空且唯一** —— MySQL 允许多个 NULL 参与 UNIQUE，正好表达「有的成员没有账号」，同时保证「一个账号只属一个家庭组」。
2. **`records.member_name` 快照** —— 成员改名不污染历史。
3. **`UNIQUE(member_id, report_date)`** —— 幂等下沉到数据库，重试不会产生重复结算。
4. **`ON DELETE CASCADE` 仅用于 families→members** —— 其余关联表**不级联**，避免误删数据链。删除成员时由 service 层显式处理记录。

---

## 4. 角色与权限

| 角色 | 入口 | 查看 | 自助兑换 | 后台管理 | 设备上报 |
|---|---|---|---|---|---|
| 未登录 | — | ❌ | ❌ | ❌ | ❌ |
| 成员 `member` | Android / Web | ✅ 本组 | ✅ 仅自己 | ❌ | ❌ |
| 管理员 `admin` | Web（推荐）/ Android | ✅ 本组 | ✅ | ✅ 本组 | ❌ |
| 设备 | Android | ❌ | ❌ | ❌ | ✅ 仅绑定成员 |

**多租户硬规则（最高优先级）**：

- 所有 SQL **必须带 `family_id = ?`**
- `family_id` **只能从 JWT claims 取**，**禁止从请求体、query、路径参数取**
- 任何跨组访问一律返回 404（而非 403，避免泄露资源是否存在）

---

## 5. 家庭组与账号

| # | 功能 | 状态 |
|---|---|---|
| 5.1 | 注册并创建家庭组（创建者自动 admin） | ⬜ |
| 5.2 | 邀请码加入家庭组 | ⬜ |
| 5.3 | 一个用户仅属一个家庭组 | ⬜ 由 `username` 全局唯一保证 |
| 5.4 | 查看 / 重置邀请码 | ⬜ |
| 5.5 | 家庭组改名 | ⬜ |
| 5.6 | 转让管理员 | ⬜ 必须有，否则唯一 admin 丢失即无人可管 |
| 5.7 | 退出家庭组 | ⚠️ 记录归属待定（18.2） |
| 5.8 | 解散家庭组 | ⚠️ 高危，需输入组名二次确认 |
| 5.9 | 登录 / 退出 | ⬜ JWT |
| 5.10 | **Token 刷新** | ⬜ v3 新增。Access Token 2h + Refresh Token 30d，解决 v1「30 天到期静默失败」 |
| 5.11 | 修改密码 | ⬜ v3 新增 |

---

## 6. 成员管理

| # | 功能 | 状态 |
|---|---|---|
| 6.1 | 添加成员（可无账号） | ⬜ |
| 6.2 | 删除成员（连带记录，需二次确认） | ⬜ |
| 6.3 | 隐藏 / 显示成员 | ⬜ |
| 6.4 | 积分清零 | ⬜ |
| 6.5 | 查看成员记录（分页） | ⬜ |
| 6.6 | 为成员设置登录与角色 | ⬜ |
| 6.7 | **成员改名** | ⬜ 历史记录因快照不受影响 |
| 6.8 | 成员排序 | ⬜ 按积分降序 |

---

## 7. 计分规则

| # | 功能 | 状态 |
|---|---|---|
| 7.1 | 固定分值规则 | ⬜ |
| 7.2 | 灵活规则 | ⬜ |
| 7.3 | 规则增删改 | ⬜ |
| 7.4 | 连续打卡奖励（多档） | ⬜ |
| 7.5 | 里程碑只奖励一次 | ⬜ |
| 7.6 | 违反时重置连续计数 | ⬜ 名称互相包含即相关 |
| 7.7 | 按次数 vs 按自然日计数 | ⚠️ v1 按次数，v3 建议改按自然日（18.4） |

---

## 8. 记一笔

| # | 功能 | 状态 |
|---|---|---|
| 8.1 | 按规则一键记分 | ⬜ |
| 8.2 | 灵活规则填分值 | ⬜ |
| 8.3 | 自定义事项记分 | ⬜ |
| 8.4 | 补记历史日期 | ⬜ 东八区当日 12:00 折算，不接受未来 |
| 8.5 | 补记标识 | ⬜ `added_at - occurred_at > 12h` |
| 8.6 | 0 分中性记录 | ⬜ |
| 8.7 | **撤销 / 删除记录** | ⬜ v3 新增，删除需回滚积分并重算连续计数 |

---

## 9. 兑换商城

| # | 功能 | 状态 |
|---|---|---|
| 9.1 | 物品增删改 | ⬜ |
| 9.2 | 管理员代任意成员兑换 | ⬜ |
| 9.3 | 成员自助兑换（Android） | ⬜ |
| 9.4 | 余额校验（前后端双重） | ⬜ |
| 9.5 | **兑换撤销** | ⬜ v3 新增 |
| 9.6 | 并发扣减防超兑 | ⬜ **需在事务内 `SELECT ... FOR UPDATE`**，v1 的 KV 无此问题但 MySQL 有 |

---

## 10. 展示与记录

| # | 功能 | 状态 |
|---|---|---|
| 10.1 | 积分排行榜 | ⬜ |
| 10.2 | 连续打卡展示（🔥） | ⬜ |
| 10.3 | 记录列表服务端分页 | ⬜ |
| 10.4 | 按成员 / 日期范围筛选 | ⬜ |
| 10.5 | 记录搜索 | ⬜ |
| 10.6 | 积分趋势图 | ⬜ 建议 v3.1 再做 |
| 10.7 | 记录总量上限 | ⬜ **取消 500 条截断**（MySQL 无此约束） |

---

## 11. 屏幕时间上报

### 11.1 已定语义

| 决策项 | 结论 |
|---|---|
| 计分方向 | 双向：≤ 阈值加分，> 阈值减分 |
| 迟到数据 | 补算但不回滚 |
| 幂等 | `UNIQUE(member_id, report_date)` |
| 认证 | 设备令牌（`X-Device-Token` 头） |
| 结算锁定 | 首次上报即锁定（⚠️ 18.3 待确认是否允许当天内覆盖） |

### 11.2 功能项

| # | 功能 | 状态 |
|---|---|---|
| 11.2.1 | 权限引导（`PACKAGE_USAGE_STATS`） | ⬜ |
| 11.2.2 | 采集当日总时长（`queryEvents` 配对 RESUMED/PAUSED） | ⬜ **不用 `INTERVAL_DAILY`** |
| 11.2.3 | 排除系统应用与息屏时段 | ⬜ |
| 11.2.4 | 每日定时上报（WorkManager） | ⬜ |
| 11.2.5 | 打开 App 补算最近 7 天 | ⬜ 可靠性主要来源 |
| 11.2.6 | 失败重试（指数退避） | ⬜ |
| 11.2.7 | 上报状态展示 | ⬜ |
| 11.2.8 | 国产 ROM 保活引导 | ⬜ |

### 11.3 结算事务（MySQL 版）

```sql
BEGIN;
  -- 1. 校验设备
  SELECT d.family_id, d.member_id FROM devices d
   WHERE d.token_hash = ? AND d.revoked = 0 FOR UPDATE;

  -- 2. 查规则
  SELECT threshold_min, bonus_points, penalty_points FROM screen_rules
   WHERE member_id = ? AND enabled = 1;

  -- 3. 幂等检查（UNIQUE 索引兜底，冲突则回滚）
  INSERT INTO screen_reports (family_id, member_id, report_date, total_min, points, reported_at)
  VALUES (?, ?, ?, ?, ?, NOW(3));

  -- 4. 结算积分
  UPDATE members SET score = score + ? WHERE id = ? AND family_id = ?;
  INSERT INTO records (...) VALUES (...);
COMMIT;
```

> 用 `INSERT` 触发唯一键冲突来保证幂等，比「先 SELECT 再判断」更可靠——并发下不会双写。

---

## 12. Android App（Kotlin 原生）

> 面向**家庭成员**，不含后台管理界面（后台在 Web）。

| # | 界面 | 关键内容 |
|---|---|---|
| 12.1 | 启动页 | 检查 token，有效则直进主页 |
| 12.2 | 登录页 | 用户名、密码、去注册 |
| 12.3 | 注册 / 加入 | 创建家庭组 或 输入邀请码加入 |
| 12.4 | 积分榜 | 排名、奖牌、积分、连续次数 |
| 12.5 | 成员详情 | 该成员记录（分页） |
| 12.6 | 最近记录 | 分页、按成员筛选 |
| 12.7 | 我的兑换 | 商城、余额校验、确认兑换 |
| 12.8 | 屏幕时间状态 | 权限状态、今日时长、上报状态、手动上报 |
| 12.9 | 设置 | 账号、家庭组信息、退出、修改密码 |

**共 9 个界面**（v2 方案为 17 个，因后台移出而减少）。

**技术栈**：Kotlin + Jetpack Compose + Retrofit + OkHttp + Coroutines + Hilt（依赖注入）+ WorkManager。

---

## 13. Web 管理后台

| # | 页面 | 对应功能 |
|---|---|---|
| 13.1 | 登录 / 注册 | 账号、家庭组 |
| 13.2 | 成员管理 | 增删改、隐藏、清零、登录设置 |
| 13.3 | 计分规则 | 增删改、灵活规则、连续档位 |
| 13.4 | 记一笔 | 选成员、选规则、补记日期、自定义 |
| 13.5 | 兑换商城 | 物品管理、代兑换、撤销 |
| 13.6 | 记录 | 分页、筛选、搜索、删除 |
| 13.7 | 屏幕时间 | 按成员配置阈值与分值 |
| 13.8 | 设备管理 | 列表、生成令牌、吊销 |
| 13.9 | 家庭组设置 | 组名、邀请码、角色、转让管理员 |

> 现有 Vue 3 前端可复用约 **60%** 的结构与样式，主要改动在 API 层与新增页面。

---

## 14. 接口清单

统一前缀 `/api`，响应 `{ ok: bool, data?: any, error?: string }`。

| # | 接口 | 方法 | 鉴权 |
|---|---|---|---|
| 14.1 | `/auth/register` | POST | 无 |
| 14.2 | `/auth/login` | POST | 无 |
| 14.3 | `/auth/refresh` | POST | refresh token |
| 14.4 | `/auth/password` | POST | JWT |
| 14.5 | `/family/join` | POST | JWT |
| 14.6 | `/family/info` | GET | JWT |
| 14.7 | `/family/update` | POST | admin |
| 14.8 | `/family/invite/reset` | POST | admin |
| 14.9 | `/family/transfer` | POST | admin |
| 14.10 | `/family/leave` | POST | JWT |
| 14.11 | `/state` | GET | JWT |
| 14.12 | `/records` | GET | JWT |
| 14.13 | `/member/*` | POST | admin |
| 14.14 | `/rule/*` | POST | admin |
| 14.15 | `/score/*` | POST | admin |
| 14.16 | `/item/*` | POST | admin |
| 14.17 | `/item/redeem` | POST | JWT |
| 14.18 | `/screen-rule/*` | POST | admin |
| 14.19 | `/device/*` | POST | admin |
| 14.20 | `/device/report` | POST | **设备令牌** |

---

## 15. 部署方案（自建服务器）

### 15.1 拓扑

```
公网 80/443 → Nginx → (静态 / → Web dist) + (/api → Go :8080) → MySQL :3306
```

**只暴露 80/443**，Go 与 MySQL 仅监听 `127.0.0.1`。

### 15.2 组件清单

| # | 事项 | 状态 |
|---|---|---|
| 15.2.1 | Docker Compose（mysql / api / nginx） | ⬜ |
| 15.2.2 | Nginx 配置（TLS、gzip、静态缓存、API 反代） | ⬜ |
| 15.2.3 | HTTPS 证书（Let's Encrypt + 自动续期） | ⬜ |
| 15.2.4 | 数据库迁移（golang-migrate，启动时或独立步骤） | ⬜ |
| 15.2.5 | 环境变量管理（`.env`，不入仓库） | ⬜ |
| 15.2.6 | 健康检查接口 `/healthz` | ⬜ |
| 15.2.7 | 日志落盘与轮转 | ⬜ |
| 15.2.8 | 防火墙（仅开 22/80/443） | ⬜ |

### 15.3 备份（**自建方案必须项**）

| # | 事项 | 状态 |
|---|---|---|
| 15.3.1 | `mysqldump` 每日定时备份 | ⬜ |
| 15.3.2 | 备份保留 7 日滚动 | ⬜ |
| 15.3.3 | **异地副本**（对象存储 / 另一台机器） | ⬜ 同机备份无法应对磁盘故障 |
| 15.3.4 | 恢复演练（至少做一次） | ⬜ 没演练过的备份等于没有备份 |

> ⚠️ 从 Cloudflare 托管转为自建，**数据安全责任完全转移到你自己**。Cloudflare 时代不需要操心的事，现在全要操心。

### 15.4 迁移（Cloudflare KV → MySQL）

| # | 步骤 | 状态 |
|---|---|---|
| 15.4.1 | 导出 KV `state` 为 JSON | ⬜ |
| 15.4.2 | 编写导入脚本：JSON → MySQL，生成默认家庭组 | ⬜ |
| 15.4.3 | 校验：成员数、记录数、积分总和逐项比对 | ⬜ **不可只比行数** |
| 15.4.4 | 双写或停机窗口 | ⬜ |
| 15.4.5 | 回滚预案 | ⬜ |

---

## 16. 与 v1 的行为差异（迁移时需注意）

| 项 | v1（KV） | v3（MySQL） |
|---|---|---|
| 记录上限 | 500 条硬截断 | 无上限 |
| 并发写 | 单键覆盖风险 | 事务 + 行锁，需防超兑 |
| 密码 | SHA-256 加盐 | **bcrypt**，旧密码需重置或兼容校验 |
| Token | 30 天固定，无刷新 | Access 2h + Refresh 30d |
| 时间字段 | 毫秒时间戳 | `DATETIME(3)` |

> **密码迁移**：bcrypt 无法直接换算旧哈希。方案：旧账号首次登录时用 SHA-256 校验通过后，立即改写为 bcrypt。需在 service 层保留一段兼容逻辑。

---

## 17. 已知缺口（v1 遗留，v3 应一并解决）

| # | 缺口 | 影响 |
|---|---|---|
| 17.1 | 记录不可撤销 | 已列入 8.7 |
| 17.2 | 成员不可改名 | 已列入 6.7 |
| 17.3 | Token 到期静默失败 | 已列入 5.10 |
| 17.4 | 无数据导出 | 建议加 CSV 导出 |
| 17.5 | 无错误监控 | 建议接入 Sentry 或自建日志告警 |

---

## 18. 待决策项

| # | 事项 | 影响 |
|---|---|---|
| 18.1 | Web 是否保留「成员只读视图」 | 决定 Web 是纯后台还是双角色 |
| 18.2 | 退出家庭组时记录归属 | 保留 / 带走 / 删除 |
| 18.3 | 屏幕时间是否允许当天内覆盖 | 决定结算分支 |
| 18.4 | 连续打卡按次数还是自然日 | 影响结算与文案 |
| 18.5 | 服务器规格与操作系统 | 影响部署脚本 |
| 18.6 | 是否公开注册 | 决定是否需要限流/验证码 |
| 18.7 | 域名与备案 | 国内服务器需备案，海外则不需要 |
| 18.8 | 旧 KV 数据是否迁移 | 决定是否需要迁移脚本 |

---

## 19. 验收要点

**多租户隔离（最高优先级）**
1. A 组 admin 无法通过任何接口读写 B 组数据（逐接口测）。
2. 篡改请求体 / query 中的 `family_id` 无效——服务端只认 JWT。
3. 一个 username 无法同时存在于两个家庭组。

**事务与并发**
4. 同一成员并发兑换 10 次，余额不出现负数。
5. 同一份屏幕时间重复上报 10 次，积分只变一次。
6. 删除记录后，积分与连续计数正确回滚。

**部署**
7. 服务器重启后所有服务自动拉起。
8. 备份文件能在**空库**上完整恢复（真做一次）。
9. HTTPS 证书自动续期正常。
10. MySQL 与 Go 端口不对外暴露。

**Android**
11. 未授权 `PACKAGE_USAGE_STATS` 时给出引导，不崩溃。
12. 杀后台后打开 App 能补齐遗漏日期。
13. 设备令牌吊销后立即 401。
