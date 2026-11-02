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

> ⚠️ **假设**：Web 端是否保留「成员只读视图」需确认（见 19.1）。当前按「Web 只做后台」设计。

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

-- 后台管理员账号（与前台成员完全独立，不参与积分）
CREATE TABLE admins (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id  BIGINT UNSIGNED NOT NULL,
  username   VARCHAR(20)  NOT NULL UNIQUE,
  pass_hash  VARCHAR(60)  NOT NULL,          -- bcrypt
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family (family_id),
  CONSTRAINT fk_admins_family FOREIGN KEY (family_id) REFERENCES families(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 成员（账号即成员：username 非空即可登录前台）
CREATE TABLE members (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id       BIGINT UNSIGNED NOT NULL,
  name            VARCHAR(32)  NOT NULL,
  score           INT          NOT NULL DEFAULT 0,
  hidden          TINYINT(1)   NOT NULL DEFAULT 0,
  avatar          VARCHAR(255) NULL,          -- 头像 URL 或本地资源标识
  username        VARCHAR(20)  NULL UNIQUE,   -- NULL 可重复，天然支持「部分成员无账号」
  pass_hash       VARCHAR(60)  NULL,          -- bcrypt
  role            ENUM('leader','member') NOT NULL DEFAULT 'member',  -- 组长/成员（前台角色）
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
  streaks    JSON         NULL,             -- 连续奖励档位，结构见 7.6；仅 trigger_type='screen_night' 时生效
  member_ids JSON         NULL,             -- 适用范围：[成员id]；NULL 表示全组生效
  trigger_type ENUM('manual','screen_night') NOT NULL DEFAULT 'manual',
                                            -- manual=手动记一笔；screen_night=屏幕时间夜间判定，不出现手动入口
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
  source       ENUM('manual','screen','device','transfer') NOT NULL DEFAULT 'manual',
  transfer_id  BIGINT UNSIGNED NULL,        -- 赠送产生的一对记录共享同一 transfer
  KEY idx_family_time (family_id, occurred_at DESC),
  KEY idx_member_time (member_id, occurred_at DESC),
  KEY idx_transfer (transfer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 积分赠送（转移关系，便于审计与撤销）
CREATE TABLE transfers (
  id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id      BIGINT UNSIGNED NOT NULL,
  from_member_id BIGINT UNSIGNED NOT NULL,
  to_member_id   BIGINT UNSIGNED NOT NULL,
  amount         INT          NOT NULL,       -- 恒为正数
  note           VARCHAR(64)  NULL,           -- 附言
  created_at     DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  revoked_at     DATETIME(3)  NULL,           -- 非空表示已撤销
  KEY idx_family_time (family_id, created_at DESC),
  KEY idx_from (from_member_id),
  KEY idx_to (to_member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 任务（家长发布，孩子认领后提交，家长确认）
CREATE TABLE tasks (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id    BIGINT UNSIGNED NOT NULL,
  title        VARCHAR(128) NOT NULL,
  points       INT          NOT NULL,         -- 完成后计分（可为负，表示惩罚任务）
  rule_id      BIGINT UNSIGNED NULL,          -- 可选：关联计分规则，复用连续奖励
  assignee_id  BIGINT UNSIGNED NULL,          -- 指定成员；NULL 表示全组可认领
  due_at       DATETIME(3)  NULL,             -- 截止时间
  repeat_rule  VARCHAR(32)  NULL,             -- 'daily' | 'weekly' | NULL（暂不实现，预留）
  status       ENUM('open','closed') NOT NULL DEFAULT 'open',
  created_by   BIGINT UNSIGNED NOT NULL,
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family_status (family_id, status, created_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 任务提交（状态机：submitted → approved / rejected）
CREATE TABLE task_submissions (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id    BIGINT UNSIGNED NOT NULL,
  task_id      BIGINT UNSIGNED NOT NULL,
  member_id    BIGINT UNSIGNED NOT NULL,
  note         VARCHAR(255) NULL,             -- 孩子的说明
  image_url    VARCHAR(255) NULL,             -- 可选凭证
  status       ENUM('submitted','approved','rejected') NOT NULL DEFAULT 'submitted',
  review_by    BIGINT UNSIGNED NULL,
  review_note  VARCHAR(255) NULL,
  reviewed_at  DATETIME(3)  NULL,
  record_id    BIGINT UNSIGNED NULL,          -- 通过后生成的积分记录
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family_status (family_id, status, created_at DESC),
  KEY idx_task (task_id),
  KEY idx_task_member (task_id, member_id, status)  -- 「同一任务同一成员仅一条待审」由应用层事务保证
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 操作审计日志（append-only，禁止 UPDATE / DELETE）
CREATE TABLE audit_logs (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id    BIGINT UNSIGNED NOT NULL,
  operator_id  BIGINT UNSIGNED NULL,          -- 操作者成员 id；设备上报为 NULL
  operator_type ENUM('member','device','system') NOT NULL DEFAULT 'member',
  action       VARCHAR(48)  NOT NULL,         -- 'score.add' / 'member.reset' / 'rule.edit' ...
  target_type  VARCHAR(24)  NULL,             -- 'member' / 'record' / 'rule' / 'task' ...
  target_id    BIGINT UNSIGNED NULL,
  before_json  JSON         NULL,
  after_json   JSON         NULL,
  ip           VARCHAR(45)  NULL,
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family_time (family_id, created_at DESC),
  KEY idx_target (target_type, target_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 站内通知
CREATE TABLE notifications (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id   BIGINT UNSIGNED NOT NULL,
  member_id   BIGINT UNSIGNED NOT NULL,       -- 接收者
  type        VARCHAR(32)  NOT NULL,          -- 'task_review' / 'redeem' / 'score' / 'transfer'
  title       VARCHAR(64)  NOT NULL,
  body        VARCHAR(255) NULL,
  ref_type    VARCHAR(24)  NULL,              -- 关联对象类型
  ref_id      BIGINT UNSIGNED NULL,
  read_at     DATETIME(3)  NULL,
  created_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_member_unread (member_id, read_at, created_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 屏幕时间规则（每成员一条）
CREATE TABLE screen_rules (
  id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id      BIGINT UNSIGNED NOT NULL,
  member_id      BIGINT UNSIGNED NOT NULL UNIQUE,
  threshold_min  INT NOT NULL,
  bonus_points   INT NOT NULL,
  penalty_points INT NOT NULL,
  enabled        TINYINT(1) NOT NULL DEFAULT 1,
  -- 夜间时段判定（用于「按时睡觉」自动打卡；全部为 NULL 表示不启用）
  night_start_hour TINYINT UNSIGNED NULL,   -- 0–23，允许跨零点（如 22）
  night_end_hour   TINYINT UNSIGNED NULL,   -- 0–23（如 7）
  night_max_min    INT NULL,                -- 该时段允许使用的分钟数上限
  night_rule_id    BIGINT UNSIGNED NULL     -- 达标时计入的规则（rules.trigger_type='screen_night'）
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 屏幕时间结算（幂等载体）
CREATE TABLE screen_reports (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id   BIGINT UNSIGNED NOT NULL,
  member_id   BIGINT UNSIGNED NOT NULL,
  report_date DATE         NOT NULL,
  total_min   INT          NOT NULL,
  night_min   INT          NULL,                              -- 夜间时段使用分钟数（用于睡觉打卡判定）
  points      INT          NOT NULL,
  record_id   BIGINT UNSIGNED NULL,                           -- 日总量判定产生的记录
  night_record_id BIGINT UNSIGNED NULL,                       -- 夜间判定产生的记录（睡觉打卡）
  source      ENUM('auto','manual') NOT NULL DEFAULT 'auto',  -- 数据来源
  revision    INT          NOT NULL DEFAULT 1,                -- 被覆盖的次数
  reported_at DATETIME(3)  NOT NULL,
  updated_at  DATETIME(3)  NULL,                              -- 最近一次覆盖时间
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

### 4.1 两套账号体系（v3 核心设计）

**后台与前台是两套完全独立的账号系统**，互不通用、互不共享 token。

| | 后台（Web） | 前台（Android） |
|---|---|---|
| 账号表 | `admins` | `members` |
| 角色 | 管理员（唯一角色） | 组长 `leader` / 成员 `member` |
| 登录接口 | `/api/admin/login` | `/api/app/login` |
| Token 类型 | `typ = 'admin'` | `typ = 'member'` |
| 是否参与积分 | **否**（管理员不是家庭成员） | 是 |

> **为什么分开**：管理员是「运营者」，负责配置与维护，不应该出现在排行榜上；
> 家庭成员是「参与者」，有积分、有记录。混在一起会导致「管理员给自己加分」的信任问题。

### 4.2 权限矩阵

| 角色 | 入口 | 看积分 | 自助兑换 | 记一笔 | 任务发布/审核 | 成员/规则/设备管理 | 审计日志 | 设备上报 |
|---|---|---|---|---|---|---|---|---|
| 后台管理员 `admin` | Web | ✅ 本组 | ❌ | ✅ | ✅ | ✅ 本组 | ✅ | ✅ |
| 家庭组组长 `leader` | Android | ✅ 本组 | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ |
| 普通成员 `member` | Android | ✅ 本组 | ✅ 仅自己 | ❌ | ❌ | ❌ | ❌ | ❌ |
| 设备 | Android | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ 仅绑定成员 |

> **组长是「家庭内协调者」，不是系统管理员**：能发布任务、审核、记一笔，
> 但**不能**改成员、改规则、管设备、看审计——那些是后台专属。
> 组长具体权限边界见 19.18（待确认）。

### 4.3 越权防护硬规则（最高优先级）

1. **Token 类型必须校验**：`/api/admin/*` 只接受 `typ='admin'`，`/api/app/*` 只接受 `typ='member'`。
   前台 token 打后台接口一律 401，反之亦然。
2. **所有 SQL 必须带 `family_id = ?`**。
3. **`family_id` 只能从 JWT claims 取**，禁止从请求体、query、路径参数取。
4. **跨组访问返回 404 而非 403**（避免泄露资源是否存在）。
5. **管理员账号不属于任何成员**，不得出现在成员列表、排行榜、记录中。

---

## 5. 账号与家庭组

### 5.1 后台账号（管理员）

| # | 功能 | 状态 |
|---|---|---|
| 5.1.1 | 注册并创建家庭组（生成 `admins` 记录） | ⬜ |
| 5.1.2 | 后台登录 / 退出 | ⬜ `/api/admin/login` |
| 5.1.3 | 修改密码 | ⬜ |
| 5.1.4 | Token 刷新（Access 2h + Refresh 30d） | ⬜ |
| 5.1.5 | 同一家庭组可有多个管理员 | ⚠️ 待确认（19.19） |
| 5.1.6 | 转让 / 移交管理员 | ⬜ 至少保留一个管理员，否则无人可管 |

### 5.2 前台账号（家庭成员）

| # | 功能 | 状态 |
|---|---|---|
| 5.2.1 | 由管理员在后台创建成员并设置前台登录凭证 | ⬜ |
| 5.2.2 | 前台登录 / 退出 | ⬜ `/api/app/login` |
| 5.2.3 | 修改自己的密码 | ⬜ |
| 5.2.4 | 成员可无账号（仅作为计分对象） | ⬜ |
| 5.2.5 | **首个成员自动成为组长** | ⬜ 后台创建第一个成员时自动 `leader` |
| 5.2.6 | 组长转让 | ⬜ 后台可指定其他成员为组长 |
| 5.2.7 | 一个前台账号仅属一个家庭组 | ⬜ 由 `members.username` 全局唯一保证 |

> **家长通常需要两个账号**：一个后台管理员账号（配置与维护），一个前台成员账号（参与积分）。
> 这是「两套系统」的必然代价，换来的是「管理员不会出现在排行榜上」的清晰边界。

### 5.3 家庭组

| # | 功能 | 状态 |
|---|---|---|
| 5.3.1 | 查看 / 重置邀请码 | ⬜ |
| 5.3.2 | 家庭组改名 | ⬜ |
| 5.3.3 | 退出家庭组（前台成员） | ⚠️ 记录归属待定（19.2） |
| 5.3.4 | 解散家庭组 | ⚠️ 高危，需输入组名二次确认 |
| 5.3.5 | 邀请码加入 | ⬜ 仅用于「已有账号加入另一个组」的场景，家庭场景较少用 |

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
| 7.4 | **规则触发方式** | ⬜ `manual`（手动记一笔）/ `screen_night`（屏幕时间夜间判定） |
| 7.5 | **规则适用范围** | ⬜ `member_ids` 指定生效成员，NULL = 全组 |
| 7.6 | **连续打卡奖励（仅 `screen_night`）** | ⬜ **多档、全部可配置**：天数、分值、是否循环、档位数都由后台设置 |
| 7.7 | 连续打卡计数口径 | ✅ **按自然日**，一天最多计 1 次 |
| 7.8 | 里程碑去重 | ⬜ 以 `streak_award` 记录已发档位，避免重复发奖 |

> **7.6 的档位结构**（存在 `rules.streaks`，JSON 数组，**档位数不限，建议 ≤ 5**）：

```json
[
  { "every": 3,  "bonus": 5,  "mode": "repeat" },   // 每满 3 天 +5
  { "every": 7,  "bonus": 10, "mode": "once" },     // 满 7 天 +10，本轮只发一次
  { "every": 30, "bonus": 50, "mode": "once" }      // 满 30 天 +50
]
```

| 字段 | 含义 | 约束 |
|---|---|---|
| `every` | 连续**天数**阈值 | 整数 ≥ 1，同组内不可重复 |
| `bonus` | 达标时奖励分值 | 整数且 ≠ 0（可为负，表示惩罚） |
| `mode` | `repeat` = 每满 N 天发一次（3、6、9…）；`once` = 本轮连续内只发一次 | 默认 `repeat` |

**关键语义**：

- **多档独立计算**：第 21 天既满 3 的倍数、又满 7 的倍数 → 两档**都发**。
- **`repeat` 与 `once` 的区别**：`every:3 + repeat` 在第 3、6、9 天各发一次；
  `every:3 + once` 只在第 3 天发，之后要等**连续中断重新开始**才会再发。
- **中断即清零**：未达标或未上报 → 连续天数归零，`repeat` 档重新从第 1 天算起，`once` 档也重新可发。
- **改配置不追溯**：修改 `every` / `bonus` / `mode` 后，**历史已结算的日期不重算**。

> **核心边界：连续打卡是 `screen_night` 规则的专属能力。**
> `manual` 规则（做家务、练琴、主动阅读等）**没有连续打卡，也不统计完成次数**——
> 记几次就是几次，只累加积分，不产生任何连续性状态。
> 这是 v3 相对 v1 的重要简化：v1 里「所有规则都能配连续奖励 + 违反时按名称相关重置」
> 那套逻辑（`resetStreaksOnViolation` / `namesRelated`）**整段废弃**。

> **7.4 的含义**：`screen_night` 规则**不出现在「记一笔」的手动列表**中，
> 只能由屏幕时间结算时自动判定并写入（见 12.5）。
> **7.6 的配置位置**：`rules.streaks` 字段保留，但**仅当 `trigger_type='screen_night'` 时生效**；
> `manual` 规则即使填了也不参与计算。

> **7.5 解决的问题**：「练琴 +2」对不学琴的孩子毫无意义，若全组共享，规则列表会越来越长。

> **已移除**：v3 早期设想的「每人每日计分上限 `daily_limit`」**不再需要**——
> 用户明确「其他任务不设完成次数」。手动规则可无限次记录，防刷分交由家长自行把控。

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

> **手动入口的边界**：`trigger_type = 'screen_night'` 的规则**不出现在本页**。
> 「按时睡觉」这类规则只能由屏幕时间结算时自动判定（12.5），**手动计数已去掉**。
> 若家长需要临时修正，走「手动上传使用时长」覆盖（12.4），而不是手动补一条打卡记录。

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

## 10. 积分赠送

### 10.1 语义定义

- **赠送 = 家庭组内成员之间的积分转移**，不是计分行为。
- **家庭总积分守恒**：一方 `-N`，另一方 `+N`，全组积分总和不变。
- **不参与连续打卡**：既不增加打卡次数，也不触发「违反重置」。
- **不可跨家庭组**：受赠方必须与赠送方同组。
- **不可赠送给自己的另一身份**：家庭成员各自独立，禁止 `from == to`。
- **唯一的金额约束是转出方余额**：不设单笔上限、不设每日上限、不设手续费。
  管理员代赠同样受此约束——**不能凭空生成积分**，只能转移已存在的积分。

### 10.2 功能项

| # | 功能 | 状态 |
|---|---|---|
| 10.2.1 | 成员自助赠送（Android） | ⬜ 从自己的积分转出 |
| 10.2.2 | 管理员代任意成员赠送（Web） | ⬜ 指定转出方与受赠方 |
| 10.2.3 | 余额校验（事务内，防负余额） | ⬜ |
| 10.2.4 | 附言（可选，≤ 64 字） | ⬜ |
| 10.2.5 | 赠送记录查询（按成员/时间） | ⬜ |
| 10.2.6 | **撤销赠送** | ⬜ 双方积分回滚，记录标记 `revoked_at` |
| 10.2.7 | 受赠方确认机制 | ⚠️ 待定（19.9） |
| 10.2.8 | 单笔 / 每日赠送上限 | ⚠️ 待定（19.10） |

### 10.3 记录表达

一次赠送产生**两条 `records`**，共享同一 `transfer_id`：

| 视角 | title | points |
|---|---|---|
| 转出方 | `赠送给 小明` | `-N` |
| 受赠方 | `来自 爸爸的赠送` | `+N` |

> 两条记录让双方的积分流水各自完整，无需在展示层做特殊拼接。
> 前端按 `source = 'transfer'` 渲染为可点击的转账卡片，点击可查看对方与附言。

### 10.4 事务逻辑

```sql
BEGIN;
  -- 按 id 升序加锁，避免并发互赠时死锁
  SELECT id, score FROM members
   WHERE id IN (?, ?) AND family_id = ? ORDER BY id FOR UPDATE;

  -- 校验：转出方余额 ≥ amount，且 from != to
  UPDATE members SET score = score - ? WHERE id = ? AND family_id = ?;
  UPDATE members SET score = score + ? WHERE id = ? AND family_id = ?;

  INSERT INTO transfers (...) VALUES (...);
  INSERT INTO records (...) VALUES (转出 -N), (转入 +N);
COMMIT;
```

**三条必须遵守的实现约束**：

1. **两行必须按 `id` 升序加锁** —— 否则 A 赠 B 与 B 赠 A 并发时会死锁。
2. **余额校验必须在事务内** —— 事务外先查后扣，并发下会扣成负数。
3. **`amount` 恒为正整数**，扣分语义由 `from/to` 方向表达，不允许传负数（否则语义反转，易被误用）。

### 10.5 边界规则

| 场景 | 处理 |
|---|---|
| 赠送给自己 | 拒绝，`from_member_id != to_member_id` |
| 金额 ≤ 0 或非整数 | 拒绝 |
| **余额不足** | 拒绝，返回当前余额。**这是唯一的金额约束**（19.10 已关闭：不设单笔/每日上限） |
| 受赠方不在本组 | 拒绝（按 `family_id` 过滤，返回 404） |
| 受赠方被隐藏 | **允许**（隐藏只影响展示，不影响数据） |
| 撤销时转出方余额已不足 | **拒绝撤销**（无法回滚），提示余额不足 |
| 已撤销的赠送再次撤销 | 拒绝（`revoked_at` 非空） |

> 「撤销时余额不足」是必须提前想清楚的边界：转出方收到 10 分后花掉了，再撤销就还不回去。
> 当前规则是**拒绝撤销**，而不是允许负余额。

## 11. 展示与记录

| # | 功能 | 状态 |
|---|---|---|
| 11.1 | 积分排行榜（总榜） | ⬜ |
| 11.2 | **周期排行榜（周榜 / 月榜）** | ⬜ 新增。按 `occurred_at` 范围聚合，解决总榜差距过大导致落后者失去动力 |
| 11.3 | 连续打卡展示（🔥） | ⬜ **仅 `screen_night` 规则显示**；手动规则没有连续概念，不显示 |
| 11.4 | 记录列表服务端分页 | ⬜ |
| 11.5 | 按成员 / 日期范围筛选 | ⬜ |
| 11.6 | 记录搜索 | ⬜ |
| 11.7 | **积分趋势图** | ⬜ 按周/月聚合折线，展示进步曲线 |
| 11.8 | **数据导出 CSV** | ⬜ 新增。导出记录与成员积分，兼作手工备份手段 |
| 11.9 | 记录总量上限 | ⬜ **取消 500 条截断**（MySQL 无此约束） |

> **周期榜的实现口径**：不是「当前积分的快照排名」，而是**按区间内 `records.points` 求和后排名**。
> 两者语义不同——后者能反映「这段时间谁表现好」，前者只是总分比较。实现前须确认取哪种。

---

## 12. 屏幕时间上报

### 12.1 已定语义

| 决策项 | 结论 |
|---|---|
| 计分方向 | 双向：≤ 阈值加分，> 阈值减分 |
| 迟到数据 | 补算但不回滚（指**新增日期**的补记） |
| **重复上报** | **后报覆盖**：同一天多次上报，以最后一次为准，按新值重算积分 |
| **上报值语义** | **当天累计总时长**（不是增量）。客户端不得传增量，服务端也不做累加 |
| 幂等 | `UNIQUE(member_id, report_date)` 保证一天只有一行；重复提交走覆盖逻辑而非拒绝 |
| 认证 | 设备令牌（`X-Device-Token` 头）；手动上传走 JWT |
| 数据来源 | `auto`（设备自动）与 `manual`（手动上传）**共用同一套覆盖规则** |

> **覆盖示例**（对应 19.3 的结论）：
> 第一次上报「当天累计 10 分钟」→ 判达标，记 +2；
> 第二次上报「当天累计 30 分钟」→ **回滚 +2，按 30 分钟重新结算**，若超阈值则改记 -3。
> 最终状态与「一开始就上报 30 分钟」完全一致。

### 12.2 功能项

| # | 功能 | 状态 |
|---|---|---|
| 12.2.1 | 权限引导（`PACKAGE_USAGE_STATS`） | ⬜ |
| 12.2.2 | 采集当日总时长（`queryEvents` 配对 RESUMED/PAUSED） | ⬜ **不用 `INTERVAL_DAILY`** |
| 12.2.3 | 排除系统应用与息屏时段 | ⬜ |
| 12.2.4 | 每日定时上报（WorkManager） | ⬜ |
| 12.2.5 | 打开 App 补算最近 7 天 | ⬜ 可靠性主要来源 |
| 12.2.6 | 失败重试（指数退避） | ⬜ |
| 12.2.7 | 上报状态展示 | ⬜ |
| 12.2.8 | 国产 ROM 保活引导 | ⬜ |
| 12.2.9 | **采集并上报夜间时段使用分钟数**（`night_min`） | ⬜ 供「按时睡觉」自动判定使用，见 12.5 |

### 12.3 结算事务（MySQL 版）

核心是 **upsert + 重算**：同一天已有结算行时，先回滚旧积分与旧记录，再按新值结算。

```sql
BEGIN;
  -- 1. 校验身份（设备令牌 或 管理员 JWT），得到 family_id / member_id
  SELECT family_id, member_id FROM devices
   WHERE token_hash = ? AND revoked = 0 FOR UPDATE;

  -- 2. 查规则
  SELECT threshold_min, bonus_points, penalty_points FROM screen_rules
   WHERE member_id = ? AND enabled = 1;

  -- 3. 锁定当日结算行
  SELECT id, total_min, points, record_id FROM screen_reports
   WHERE member_id = ? AND report_date = ? FOR UPDATE;

  -- 4. 若已存在且数值未变 → 直接 COMMIT，不产生新记录（避免无意义 revision 增长）
  --    若已存在且数值变化 → 回滚旧结果
  UPDATE members SET score = score - :old_points
   WHERE id = :member_id AND family_id = :family_id;
  DELETE FROM records WHERE id = :old_record_id;

  -- 5. 按新值结算（新值 = 请求传入的当天累计值，不是增量）
  UPDATE members SET score = score + :new_points WHERE id = :member_id;
  INSERT INTO records (...) VALUES (...);

  -- 6. 写入 / 覆盖结算行
  UPDATE screen_reports
     SET total_min = :new_total, points = :new_points, record_id = :new_record,
         source = :source, revision = revision + 1, updated_at = NOW(3)
   WHERE member_id = ? AND report_date = ?;
COMMIT;
```

**四条实现要点**：

1. **必须在一个事务内完成「回滚 + 重算 + 覆盖」**，否则中途失败会留下「积分已扣、记录已删、结算行未更新」的脏状态。
2. **数值未变时直接返回成功**，不要无条件 `revision+1`——否则设备每天重试会把 revision 刷到很大。
3. **积分允许因覆盖而下降**（如 30 分钟改成 10 分钟，从扣分变加分）。这是「以最新数据为准」的必然结果，**不是 bug**。
4. **旧记录若已被单独删除**（`DELETE` 受影响行数为 0），跳过删除继续重算，并记一条异常日志，不要因此中断事务。

### 12.4 手动上传手机使用时间

#### （一）为什么需要

自动采集有三个绕不开的盲区：

| 场景 | 自动采集能否覆盖 |
|---|---|
| 孩子手机未安装 App / 未授权 | ❌ |
| iOS 设备（Family Controls 拿不到） | ❌ |
| 国产 ROM 杀后台，WorkManager 长期不执行 | ⚠️ 部分 |
| 设备损坏 / 换机期间的空档 | ❌ |

手动上传是**兜底通道**，不是替代品。

#### （二）功能项

| # | 功能 | 状态 |
|---|---|---|
| 12.4.1 | Web 后台手动录入（选成员 + 日期 + 分钟数） | ⬜ |
| 12.4.2 | Android 端手动补录（入口在屏幕时间页） | ⬜ |
| 12.4.3 | 输入校验（0 ≤ 分钟 ≤ 1440，整数） | ⬜ |
| 12.4.4 | 与已有记录冲突时按优先级处理（见下文「冲突规则」） | ⬜ |
| 12.4.5 | 覆盖已有记录时自动重算积分 | ⬜ |
| 12.4.6 | 手动录入写审计日志（`screen.manual`） | ⬜ |
| 12.4.7 | 手动数据在列表中标示来源（「手动」标签） | ⬜ |
| 12.4.8 | 上传截图作为凭证 | ⚠️ 待定（19.16） |
| 12.4.9 | 普通成员自助上传 | ⚠️ 待定（19.17） |

#### （三）与自动上报的冲突规则（核心）

两者共用 `UNIQUE(member_id, report_date)`，**同一天只能存在一条结算记录**。
自 19.3 拍板后，**来源不再影响覆盖行为**——任何一次新上报都以最新值为准：

| 已有记录 | 新上报来源 | 处理 |
|---|---|---|
| 无 | 自动 / 手动 | 正常结算，`source` 记为本次来源 |
| `auto` | 自动 | **覆盖重算**（设备当天继续使用后补报） |
| `auto` | 手动（admin） | **覆盖重算**，`source` 改为 `manual` |
| `manual` | 自动 | **覆盖重算**，`source` 改回 `auto` |
| `manual` | 手动（admin） | **覆盖重算** |
| 任意 | 手动（普通成员） | 拒绝，权限未开放（19.17） |

> **权限只约束「谁能发起手动上传」，不约束「覆盖是否发生」。**
> 覆盖逻辑本身对 auto / manual 一视同仁，见 12.3 的事务。

**覆盖必须重算，不能只改数字**：

```sql
BEGIN;
  SELECT ... FROM screen_reports
   WHERE member_id = ? AND report_date = ? FOR UPDATE;

  -- 1. 回滚旧积分与旧记录
  UPDATE members SET score = score - :old_points WHERE id = :member_id;
  DELETE FROM records WHERE id = :old_record_id;

  -- 2. 按新值重新结算
  UPDATE members SET score = score + :new_points WHERE id = :member_id;
  INSERT INTO records (...) VALUES (...);

  -- 3. 更新结算行
  UPDATE screen_reports
     SET total_min = :new_total, points = :new_points, record_id = :new_record,
         source = :source, revision = revision + 1, updated_at = NOW(3)
   WHERE member_id = ? AND report_date = ?;
COMMIT;
```

> 若只改 `total_min` 而不回滚旧积分，账就对不上；若把新旧值相加，就会重复计分。
> 这两种错误在验收要点 34、35 中各有对应测试。

#### （四）边界规则

| 场景 | 处理 |
|---|---|
| 分钟数 < 0 或 > 1440 | 拒绝（一天只有 1440 分钟） |
| 非整数 | 拒绝 |
| 未来日期 | 拒绝 |
| 超出可补录窗口（建议 30 天） | 拒绝 |
| 该成员无 `screen_rules` 配置 | 拒绝，提示先配置规则 |
| 该成员属于其他家庭组 | 拒绝（按 `family_id` 过滤，返回 404） |
| 覆盖时旧记录已被单独删除 | 回滚跳过删除，继续重算，并记异常日志 |

### 12.5 夜间时段判定（「按时睡觉」自动打卡）

#### （一）目的

把「按时睡觉」从**手动打卡**改为**屏幕时间自动判定**，并去掉该规则的手动计数入口。
判定不再依赖家长点按钮，而是由夜间时段的实际使用时长决定。

#### （二）判定逻辑

| 项 | 规则 |
|---|---|
| **睡觉时间点** | `night_start_hour`，**可配置**（0–23）。`0` 表示「24:00 前放下手机」 |
| 起床时间点 | `night_end_hour`（0–23），用于界定夜间时段的结束，如 `7` |
| 时段 | `night_start_hour` ~ `night_end_hour`，**允许跨零点**（如 0 → 7） |
| 数据 | 上报时携带 `night_min`（该时段内累计使用分钟数） |
| 达标条件 | `night_min ≤ night_max_min`。**`night_max_min` 建议默认 0**，即该时段完全不使用手机 |
| **归属日** | 按**时段开始日**归属。0:00–07:00 的时段算作「前一天」 |
| 判定频率 | **一天只判定一次**，重复上报按 12.1 的覆盖规则重算 |
| 计分 | 写入 `records`（`source='screen'`），并**推进该规则的连续天数** |

> **「晚上 12 点前放下手机」的配置方式**：`night_start_hour = 0`、`night_end_hour = 7`、
> `night_max_min = 0`。判定的是「0 点到 7 点之间有没有使用手机」，
> 只要用了一分钟就算未达标。若允许「躺床上看 5 分钟」，把 `night_max_min` 调成 5 即可。
>
> ⚠️ **归属日的坑**：0 点开始的时段如果按「当天」归属，会导致「今天判今天、但数据要等明天才有」。
> 因此统一**按开始日归属**——0:00–07:00 的数据记在**前一天**的账上，
> 这样「昨晚有没有按时睡」在当天就能结算。

#### （三）结算行为

| 情况 | 处理 |
|---|---|
| 达标 | 记 `+points`，连续天数 +1；若命中 `streaks` 档位，**额外**记一条奖励记录 |
| 未达标 | 记一条 0 分说明记录（如「夜间使用 45 分钟，超过 0 分钟」），**连续天数清零** |
| 当日未上报 | **视为未达标**，连续天数清零 |

**连续奖励示例**（以下配置全部由后台设置，不是写死的）：

```json
rules.streaks = [
  { "every": 3, "bonus": 5, "mode": "repeat" },   // 每满 3 天 +5
  { "every": 7, "bonus": 10, "mode": "once" }     // 满 7 天 +10（本轮只一次）
]
```

```
第 1 天达标 → +2，连续 1
第 2 天达标 → +2，连续 2
第 3 天达标 → +2，连续 3 → 命中 every:3(repeat) → 额外 +5，共 +7
第 4 天达标 → +2，连续 4
...
第 6 天达标 → +2，连续 6 → 再次命中 every:3(repeat) → 额外 +5
第 7 天达标 → +2，连续 7 → 命中 every:7(once)  → 额外 +10
第 8 天达标 → +2，连续 8（every:7 是 once，不再发）
第 9 天达标 → +2，连续 9 → 命中 every:3(repeat) → 额外 +5
第 10 天未达标 → 0 分说明，连续清零
第 11 天达标 → +2，连续重新从 1 开始，所有档位重新可发
```

> **去重键**：`streak_award: {every, n, from, mode}`。`n` 是命中时的连续天数，
> `from` 是本轮清零的时间戳——两者共同保证「同一轮内的同一档位只发一次」，
> 而中断后重新计数时可以正常再发。

> **配置校验**：`every ≥ 1`、`bonus ≠ 0`、同组内 `every` 不可重复、档位数建议 ≤ 5。
> 非法配置在保存时就拒绝，不要等到结算时才发现。

> **「未上报 = 未达标」是一个取舍，不是疏漏。**
> 若未上报保持连续，孩子只要卸载 App 或关掉授权就能保住连签，机制失效。
> 代价是设备故障会误伤连签——家庭场景下可接受，必要时由家长手动补录（12.4）修正。

#### （四）与日总量规则的关系

两条判定**相互独立**，同一次上报可同时产生两条记录：

```
上报 { total_min: 180, night_min: 45 }
   ├─ 日总量 180 > 阈值 120 → -3 分（record_id）
   └─ 夜间 45 > 上限 30    → 0 分说明 + 连续清零（night_record_id）
```

**覆盖时必须回滚两条**：`screen_reports` 用 `record_id` 与 `night_record_id` 两个字段分别指向，
缺一个就会残留孤儿记录。

#### （五）边界规则

| 场景 | 处理 |
|---|---|
| `night_*` 三项均为 NULL | 不启用夜间判定，只按日总量结算 |
| `night_start == night_end` | 配置非法，拒绝保存 |
| 跨零点时段的日期归属 | 按开始日（`night_start_hour` 所在的自然日） |
| `night_rule_id` 指向的规则不存在或非 `screen_night` | 拒绝结算，返回配置错误 |
| 该规则已有手动记录 | 不允许（`screen_night` 规则不出现在手动入口，历史数据除外） |

---

## 13. Android App（Kotlin 原生）

> 面向**家庭成员**（组长与成员），**不含任何后台管理界面**。
> **无注册入口**——前台账号由管理员在 Web 后台创建后下发。

| # | 界面 | 关键内容 | 可见角色 |
|---|---|---|---|
| 13.1 | 启动页 | 检查前台 token，有效则直进主页 | 全部 |
| 13.2 | 登录页 | 用户名、密码；**无注册按钮**，附「账号由管理员创建」提示 | 全部 |
| 13.3 | 积分榜 | 排名、奖牌、积分、连续天数（仅睡觉规则） | 全部 |
| 13.4 | 成员详情 | 该成员记录（分页） | 全部 |
| 13.5 | 最近记录 | 分页、按成员筛选 | 全部 |
| 13.6 | 我的兑换 | 商城、余额校验、确认兑换 | 全部 |
| 13.7 | 赠送积分 | 选受赠成员、金额与附言、赠送历史 | 全部 |
| 13.8 | 任务 | 可认领任务、提交完成、我的提交 | 全部 |
| 13.9 | **任务审核** | 待审列表、通过 / 驳回 | **仅组长** |
| 13.10 | 通知中心 | 未读列表、点击跳转、标记已读 | 全部 |
| 13.11 | 屏幕时间状态 | 权限状态、今日时长、上报状态、手动补录 | 全部 |
| 13.12 | 设置 | 账号、家庭组信息、退出、改密码、头像 | 全部 |

**共 12 个界面**，其中 1 个（任务审核）仅组长可见。

**组长在 Android 上能做**：发布任务、审核任务、记一笔。
**不能做**：改成员、改规则、管设备、看审计——这些只在 Web 后台。

**技术栈**：Kotlin + Jetpack Compose + Retrofit + OkHttp + Coroutines + Hilt（依赖注入）+ WorkManager。

---

## 14. Web 管理后台

> **仅管理员使用**（`admins` 账号）。不提供成员只读视图——成员看数据请用 Android。

| # | 页面 | 对应功能 |
|---|---|---|
| 14.1 | 登录 / 注册 | 后台管理员账号；注册即创建家庭组 |
| 14.2 | 成员管理 | 增删改、隐藏、清零、**设置前台登录凭证**、设组长、头像 |
| 14.3 | 计分规则 | 增删改、灵活规则、触发方式、适用范围；`screen_night` 规则额外配置**睡觉时间点 + 连续奖励档位**（天数/分值/是否循环，可加多档） |
| 14.4 | 记一笔 | 选成员、选规则、补记日期、自定义 |
| 14.5 | **任务管理** | 发布任务、待审列表、通过/驳回、关闭 |
| 14.6 | 兑换商城 | 物品管理、代兑换、撤销 |
| 14.7 | 赠送管理 | 代任意成员赠送、赠送记录、撤销赠送 |
| 14.8 | 记录 | 分页、筛选、搜索、删除、导出 CSV |
| 14.9 | 统计 | 周期排行榜、积分趋势图 |
| 14.10 | **审计日志** | 按成员/动作/时间筛选，查看 before/after |
| 14.11 | 屏幕时间 | 按成员配置阈值与睡觉时间点、手动录入、上报历史 |
| 14.12 | 设备管理 | 列表、生成令牌、吊销 |
| 14.13 | 家庭组设置 | 组名、邀请码、移交管理员 |

> **复用率下降**：现有 Vue 3 前端是「积分榜 + 后台」的合并体，v3 改为纯管理后台后，
> 积分榜、自助兑换、成员视角的页面**全部要移出**，预计可复用约 **35%**（表单与样式），
> 而非此前估计的 60%。这是「两套账号系统」的直接成本。

---

## 15. 接口清单

响应统一 `{ ok: bool, data?: any, error?: string }`。
**两套前缀，两套鉴权，互不通用**：

- `/api/admin/*` —— 仅接受 `typ='admin'` 的 token
- `/api/app/*` —— 仅接受 `typ='member'` 的 token
- `/api/device/*` —— 仅接受设备令牌

### 15.1 公开接口（无需鉴权）

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.1.1 | `/api/admin/register` | POST | 创建家庭组 + 后台管理员账号 |
| 15.1.2 | `/api/admin/login` | POST | 后台登录 |
| 15.1.3 | `/api/app/login` | POST | 前台登录 |
| 15.1.4 | `/api/admin/refresh` | POST | 后台 token 刷新 |
| 15.1.5 | `/api/app/refresh` | POST | 前台 token 刷新 |

### 15.2 后台接口（`typ='admin'`）

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.2.1 | `/api/admin/password` | POST | 改密码 |
| 15.2.2 | `/api/admin/family` | GET/POST | 查看 / 改名 |
| 15.2.3 | `/api/admin/family/invite-reset` | POST | 重置邀请码 |
| 15.2.4 | `/api/admin/family/transfer-admin` | POST | 移交管理员 |
| 15.2.5 | `/api/admin/member/*` | POST | 成员增删改、隐藏、清零、登录设置、设组长 |
| 15.2.6 | `/api/admin/rule/*` | POST | 规则增删改（含 screen_night 配置） |
| 15.2.7 | `/api/admin/score/*` | POST | 记一笔 / 自定义 / 删除记录 |
| 15.2.8 | `/api/admin/item/*` | POST | 物品增删改、代兑换、撤销 |
| 15.2.9 | `/api/admin/screen-rule/*` | POST | 屏幕时间规则与睡觉时间点配置 |
| 15.2.10 | `/api/admin/screen/report-manual` | POST | 手动录入使用时长 |
| 15.2.11 | `/api/admin/device/*` | POST | 设备生成 / 吊销 |
| 15.2.12 | `/api/admin/task/*` | POST | 发布 / 关闭任务、审核 |
| 15.2.13 | `/api/admin/audit/list` | GET | 审计日志查询 |
| 15.2.14 | `/api/admin/export/records` | GET | 导出 CSV |
| 15.2.15 | `/api/admin/state` | GET | 后台初始化数据 |

### 15.3 前台接口（`typ='member'`）

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.3.1 | `/api/app/password` | POST | 改自己的密码 |
| 15.3.2 | `/api/app/state` | GET | 本组概览（不含全量记录） |
| 15.3.3 | `/api/app/records` | GET | 记录分页 / 筛选 / 搜索 |
| 15.3.4 | `/api/app/item/redeem` | POST | 自助兑换 |
| 15.3.5 | `/api/app/transfer/send` | POST | 赠送积分 |
| 15.3.6 | `/api/app/transfer/list` | GET | 赠送历史 |
| 15.3.7 | `/api/app/task/list` | GET | 可认领任务 |
| 15.3.8 | `/api/app/task/submit` | POST | 提交完成 |
| 15.3.9 | `/api/app/task/review` | POST | **仅 leader**：通过 / 驳回 |
| 15.3.10 | `/api/app/score/add` | POST | **仅 leader**：记一笔 |
| 15.3.11 | `/api/app/task/create` | POST | **仅 leader**：发布任务 |
| 15.3.12 | `/api/app/notification/list` | GET | 通知列表 |
| 15.3.13 | `/api/app/notification/read` | POST | 标记已读 |
| 15.3.14 | `/api/app/stats/ranking` | GET | 排行榜 |
| 15.3.15 | `/api/app/stats/trend` | GET | 趋势图 |
| 15.3.16 | `/api/app/member/avatar` | POST | 改自己的头像 |
| 15.3.17 | `/api/app/screen/report-manual` | POST | 手动补录（19.17 未开放前仅 leader） |

### 15.4 设备接口（设备令牌）

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.4.1 | `/api/device/report` | POST | 上报使用时长（含 `night_min`） |

> **命名说明**：原 `/family/transfer`（转让管理员）已改名 `/family/transfer-admin`，
> 与积分赠送的 `/transfer/*` 区分；且因两套账号分离，「转让管理员」只发生在 `admins` 之间。

> **越权测试基线**：前台 token 打任一 `/api/admin/*` 必须 401；
> 后台 token 打任一 `/api/app/*` 必须 401；普通成员打 leader 专属接口必须 403。

---

## 16. 部署方案（自建服务器）

### 16.1 拓扑

```
公网 80/443 → Nginx → (静态 / → Web dist) + (/api → Go :8080) → MySQL :3306
```

**只暴露 80/443**，Go 与 MySQL 仅监听 `127.0.0.1`。

### 16.2 组件清单

| # | 事项 | 状态 |
|---|---|---|
| 16.2.1 | Docker Compose（mysql / api / nginx） | ⬜ |
| 16.2.2 | Nginx 配置（TLS、gzip、静态缓存、API 反代） | ⬜ |
| 16.2.3 | HTTPS 证书（Let's Encrypt + 自动续期） | ⬜ |
| 16.2.4 | 数据库迁移（golang-migrate，启动时或独立步骤） | ⬜ |
| 16.2.5 | 环境变量管理（`.env`，不入仓库） | ⬜ |
| 16.2.6 | 健康检查接口 `/healthz` | ⬜ |
| 16.2.7 | 日志落盘与轮转 | ⬜ |
| 16.2.8 | 防火墙（仅开 22/80/443） | ⬜ |

### 16.3 备份（**自建方案必须项**）

| # | 事项 | 状态 |
|---|---|---|
| 16.3.1 | `mysqldump` 每日定时备份 | ⬜ |
| 16.3.2 | 备份保留 7 日滚动 | ⬜ |
| 16.3.3 | **异地副本**（对象存储 / 另一台机器） | ⬜ 同机备份无法应对磁盘故障 |
| 16.3.4 | 恢复演练（至少做一次） | ⬜ 没演练过的备份等于没有备份 |

> ⚠️ 从 Cloudflare 托管转为自建，**数据安全责任完全转移到你自己**。Cloudflare 时代不需要操心的事，现在全要操心。

### 16.4 迁移（Cloudflare KV → MySQL）

| # | 步骤 | 状态 |
|---|---|---|
| 16.4.1 | 导出 KV `state` 为 JSON | ⬜ |
| 16.4.2 | 编写导入脚本：JSON → MySQL，生成默认家庭组 | ⬜ |
| 16.4.3 | 校验：成员数、记录数、积分总和逐项比对 | ⬜ **不可只比行数** |
| 16.4.4 | 双写或停机窗口 | ⬜ |
| 16.4.5 | 回滚预案 | ⬜ |

---

## 17. 与 v1 的行为差异（迁移时需注意）

| 项 | v1（KV） | v3（MySQL） |
|---|---|---|
| 记录上限 | 500 条硬截断 | 无上限 |
| 并发写 | 单键覆盖风险 | 事务 + 行锁，需防超兑 |
| 密码 | SHA-256 加盐 | **bcrypt**，旧密码需重置或兼容校验 |
| Token | 30 天固定，无刷新 | Access 2h + Refresh 30d |
| 时间字段 | 毫秒时间戳 | `DATETIME(3)` |

> **密码迁移**：bcrypt 无法直接换算旧哈希。方案：旧账号首次登录时用 SHA-256 校验通过后，立即改写为 bcrypt。需在 service 层保留一段兼容逻辑。

---

## 18. 已知缺口（v1 遗留，v3 应一并解决）

| # | 缺口 | 影响 |
|---|---|---|
| 18.1 | 记录不可撤销 | 已列入 8.7 |
| 18.2 | 成员不可改名 | 已列入 6.7 |
| 18.3 | Token 到期静默失败 | 已列入 5.10 |
| 18.4 | 无数据导出 | 建议加 CSV 导出 |
| 18.5 | 无错误监控 | 建议接入 Sentry 或自建日志告警 |

---

## 19. 待决策项

| # | 事项 | 影响 |
|---|---|---|
| 19.2 | 退出家庭组时记录归属 | 保留 / 带走 / 删除 |
| 19.5 | 服务器规格与操作系统 | 影响部署脚本 |
| 19.6 | 是否公开注册 | 决定是否需要限流/验证码 |
| 19.7 | 域名与备案 | 国内服务器需备案，海外则不需要 |
| 19.8 | 旧 KV 数据是否迁移 | 决定是否需要迁移脚本 |
| 19.9 | 赠送是否需要受赠方确认 | 直接到账（简单）vs 请求-接受（防误操作） |
| 19.11 | 任务提交是否强制图片凭证 | 强制防作弊但增加摩擦，建议可选 |
| 19.12 | 周期排行榜口径 | 区间内得分求和 vs 期末总分快照（语义不同，见 11 节注） |
| 19.13 | 审计日志保留期 | 建议 180 天，需配套定时清理任务 |
| 19.14 | 头像存储方式 | 本地磁盘（须纳入备份）vs 对象存储（额外成本） |
| 19.15 | 任务驳回后重提次数上限 | 建议 3 次，防止刷屏 |
| 19.16 | 手动上传是否需要截图凭证 | 有凭证可核对但增加摩擦；建议可选 |
| 19.17 | 普通成员能否自助手动上传 | 开放则孩子可能少报时长作弊；建议仅 admin |
| 19.18 | 组长的权限边界 | 当前设计为「发布/审核任务、记一笔」；是否还要代兑换、撤销记录 |
| 19.19 | 同一家庭组是否允许多个后台管理员 | 家庭场景单人足够；多人可分担但增加移交复杂度 |

> **已关闭的决策**：
>
> - **19.1（Web 是否保留成员只读视图）** —— 2026-11-02 拍板为「**不保留**」：
>   Web 仅做管理后台，成员看数据请用 Android。详见 4.1 与 14 节。
>   此项关闭后，现有 Vue 前端可复用率由 60% 下调至约 **35%**。
>
> - **19.3（屏幕时间覆盖策略）** —— 2026-11-02 拍板为「**后报覆盖**」：
>   同一天多次上报以最后一次为准，按新值重算积分（回滚旧结果 + 重新结算）。
>   详见 12.1 与 12.3。此项曾从 v1 拖延至今，**关闭后同时解除了 12.1 与 12.4 的矛盾**。
> - **19.4（连续打卡计数口径）** —— 2026-11-02 拍板为「**连续打卡只属于屏幕时间**」：
>   计数按自然日（一天最多 1 次）；「按时睡觉」由屏幕时间夜间判定驱动，**去掉手动计数**；
>   **其他任务没有连续打卡，也不统计完成次数**（`daily_limit` 随之取消）。
>   详见 7.4–7.8 与 12.5。
> - **19.10（赠送上限）** —— 2026-11-02 拍板为「**只受转出方已有积分限制**」：
>   不设单笔上限、不设每日上限；管理员代赠同样不能凭空生成积分。详见 10.1 与 10.5。

---

## 20. 任务认领与家长确认

### 20.1 目的

v3 之前的「记一笔」是**家长单方面操作**，孩子只能被动接受。这让积分变成「家长的恩赐」而非「自己的成就」。
本节补上**孩子主动参与**的闭环：家长发布任务 → 孩子认领并提交 → 家长确认 → 自动计分。

### 20.2 状态机

```
任务 open
   └─ 孩子提交 → submission: submitted
        ├─ 家长通过 → approved  → 写入 records，任务可关闭
        └─ 家长驳回 → rejected  → 孩子可重新提交（新 submission 行）
```

### 20.3 功能项

| # | 功能 | 状态 |
|---|---|---|
| 20.3.1 | 发布任务（标题、分值、指定成员、截止时间） | ⬜ |
| 20.3.2 | 任务列表（孩子视角：可认领 / 进行中 / 已完成） | ⬜ |
| 20.3.3 | 提交完成（附说明，可选附图） | ⬜ |
| 20.3.4 | 待审列表（家长视角） | ⬜ |
| 20.3.5 | 通过 / 驳回（可填驳回理由） | ⬜ |
| 20.3.6 | 通过后自动写入积分记录 | ⬜ |
| 20.3.7 | 关联计分规则（复用其分值与适用范围） | ⬜ **任务不产生连续打卡**，仅按规则分值计分 |
| 20.3.8 | 任务关闭 / 删除 | ⬜ |
| 20.3.9 | 重复任务（每日/每周） | ⛔ 暂不实现，`repeat_rule` 字段预留 |

### 20.4 实现约束

1. **通过时必须走 `score/add` 的同一套逻辑**，不要另写一份计分代码——否则积分累加、审计日志、赠送余额校验会出现两套口径。
   **任务不参与连续打卡**：连续打卡是 `screen_night` 规则的专属能力，任务与手动规则一样没有连续性状态。
2. **「同一任务同一成员仅一条待审」由应用层事务保证**（MySQL 不支持部分索引，不能靠 UNIQUE 约束）。
3. **通过动作必须幂等**：`task_submissions.status` 从 `submitted` 条件更新为 `approved`，
   受影响行数为 0 则说明已处理过，直接返回成功。
4. **驳回后可重新提交**，但要限制次数（建议 3 次）防止刷屏。

### 20.5 边界规则

| 场景 | 处理 |
|---|---|
| 任务已关闭后提交 | 拒绝 |
| 同一任务重复提交 | 拒绝（已有 `submitted` 记录） |
| 家长审核自己的提交 | 允许，但记录审计日志 |
| 删除有提交记录的任务 | 拒绝，改为关闭（保留历史） |
| 任务分值为负（惩罚任务） | 允许，走减分逻辑 |
| 截止时间已过 | 提交时提示，但仍允许（家庭场景不必强卡） |

---

## 21. 操作审计日志

### 21.1 目的

管理员可任意改分、清零、删记录，**当前无任何追溯**。多租户环境下，一旦积分对不上账，无法定位。
这是清单中**优先级最高的技术缺口**——不是体验问题，是数据可信度问题。

### 21.2 记录范围

| 动作 | action 值 | 是否记 before/after |
|---|---|---|
| 记一笔 / 自定义计分 | `score.add` / `score.custom` | 是 |
| 删除记录 | `record.delete` | 是（完整记录快照） |
| 积分清零 | `member.reset` | 是（原积分） |
| 成员增删改 | `member.*` | 是 |
| 规则增删改 | `rule.*` | 是 |
| 兑换 / 撤销兑换 | `item.redeem` / `item.revoke` | 是 |
| 积分赠送 / 撤销 | `transfer.send` / `transfer.revoke` | 是 |
| 任务审核 | `task.review` | 是 |
| 设备生成 / 吊销 | `device.*` | 是 |
| 登录 / 修改密码 | `auth.login` / `auth.password` | 否（仅记录事件） |

### 21.3 功能项

| # | 功能 | 状态 |
|---|---|---|
| 21.3.1 | 全量写入审计日志 | ⬜ |
| 21.3.2 | Web 后台查询（按成员 / 动作 / 时间筛选） | ⬜ |
| 21.3.3 | 单条记录查看 before/after 差异 | ⬜ |
| 21.3.4 | 日志保留期与清理（建议 180 天） | ⬜ |
| 21.3.5 | 日志导出 | ⬜ |

### 21.4 实现约束

1. **append-only**：应用层不得提供 UPDATE / DELETE 接口；数据库层建议用独立账号或触发器限制。
2. **必须脱敏**：`before_json` / `after_json` 若包含成员对象，**必须剔除 `pass_hash`**，
   否则等于把密码哈希写进了可查询的日志表。
3. **写入失败不能阻断主流程**：审计日志是旁路，写失败时记应用日志并继续，不要因为审计失败导致记分失败。
4. **`family_id` 必须来自 JWT**，与其他表一致，避免日志本身成为越权入口。

---

## 22. 站内通知与成员头像

### 22.1 站内通知

| # | 功能 | 状态 |
|---|---|---|
| 22.1.1 | 通知生成（任务待审 / 审核结果 / 兑换成功 / 收到赠送） | ⬜ |
| 22.1.2 | 未读数角标 | ⬜ |
| 22.1.3 | 通知列表与标记已读 | ⬜ |
| 22.1.4 | 点击跳转到对应对象 | ⬜ |
| 22.1.5 | 系统推送（FCM / 厂商通道） | ⛔ 明确不做：国内需接多家通道，成本远高于收益 |

> **不做推送的取舍**：Android 端靠「打开 App 时拉取未读」即可满足家庭场景。
> 若日后确有实时性需求，再评估厂商通道。

### 22.2 成员头像

| # | 功能 | 状态 |
|---|---|---|
| 22.2.1 | 预设头像选择（内置一组） | ⬜ |
| 22.2.2 | 自定义上传 | ⬜ |
| 22.2.3 | 图片校验（格式、尺寸 ≤ 2 MB、服务端二次校验） | ⬜ |
| 22.2.4 | 排行榜与记录列表展示头像 | ⬜ |

> **自建服务器的存储决策**：头像存本地磁盘目录，由 Nginx 直接托管。
> ⚠️ **该目录必须纳入备份范围**，否则数据库恢复后头像全部丢失。

---

## 23. 验收要点

**多租户隔离（最高优先级）**
1. A 组 admin 无法通过任何接口读写 B 组数据（逐接口测）。
2. 篡改请求体 / query 中的 `family_id` 无效——服务端只认 JWT。
3. 一个 username 无法同时存在于两个家庭组。

**事务与并发**
4. 同一成员并发兑换 10 次，余额不出现负数。
5. 同一份屏幕时间重复上报 10 次，积分只变一次。
6. 删除记录后，积分与连续计数正确回滚。
7. **A 与 B 并发互赠，不出现死锁**（按 id 升序加锁）。
8. **赠送前后家庭总积分不变**（转出与转入等额）。
9. 撤销赠送后，双方积分与记录均正确回滚；转出方余额不足时撤销被拒绝。

**任务与审计**
10. 同一任务同一成员重复提交，只产生一条待审记录。
11. 同一提交被并发审核两次，只计一次分（条件更新受影响行数为 0 即返回成功）。
12. 任务通过后的计分，与「记一笔」走同一套计分逻辑；**任务不产生连续打卡**。
13. 审计日志能查到每一次改分、清零、删记录的 before/after。
14. 审计日志中**不含任何 `pass_hash`**。
15. 审计日志写入失败时，主流程仍然成功。

**规则与统计**
16. 手动规则（`manual`）即使配了 `streaks`，也**不产生连续奖励、不统计次数**，只累加积分。
17. 设置了 `member_ids` 的规则，不在范围内的成员看不到也调不动。
18. 周期排行榜的数值与「区间内记录求和」一致。

**部署**
19. 服务器重启后所有服务自动拉起。
20. 备份文件能在**空库**上完整恢复（真做一次）。
21. 头像目录也在备份范围内（恢复后头像不丢）。
22. HTTPS 证书自动续期正常。
23. MySQL 与 Go 端口不对外暴露。

**Android**
24. 未授权 `PACKAGE_USAGE_STATS` 时给出引导，不崩溃。
25. 杀后台后打开 App 能补齐遗漏日期。
26. 设备令牌吊销后立即 401。

**屏幕时间覆盖（19.3 已定：后报覆盖）**
27. 先报「当天累计 10 分钟」（达标 +2），再报「当天累计 30 分钟」（超阈值 -3）
    → 最终积分**等于直接上报 30 分钟的结果**，即 +2 已被回滚。
28. 覆盖后旧记录被删除，记录列表**不出现两条**屏幕时间记录。
29. 重复上报**相同数值**时，`revision` 不增长，也不产生新记录。
30. 覆盖导致积分下降是**允许的**，不触发任何保护或告警逻辑。
31. 手动上传由普通成员发起时被拒绝（19.17 未开放）；管理员可覆盖自动记录。
32. 分钟数为 1441 或负数时被拒绝。
33. 覆盖操作在审计日志中可查到 `before` / `after`。
34. 事务中途失败时，积分、记录、结算行三者**不会出现半更新状态**。

**夜间判定（睡觉打卡，19.4 已定）**
35. 配置「0 点前放下手机」（0:00–7:00、允许 0 分钟）：夜间使用 0 分钟 → 记 +N，连续 +1。
36. 夜间使用 45 分钟 → 记 0 分说明记录，连续天数**清零**。
37. 当日未上报 → **视为未达标**，连续天数清零。
38. 0:00–07:00 的数据归属**前一天**，不归到当天。
39. 一次上报同时触发日总量与夜间两条记录；覆盖时**两条都被正确回滚**，无孤儿记录。
40. `screen_night` 规则**不出现在「记一笔」列表**中。
41. 连续 3 天达标触发 `+5` 奖励；`repeat` 档在第 6、9 天再次触发，`once` 档只触发一次。
42. 修改「睡觉时间点」或「连续奖励档位」后，历史已结算的日期**不追溯重算**。
43. 配置 `every = 0` 或 `bonus = 0` 时保存被拒绝；同组内 `every` 重复时保存被拒绝。
44. 多档同时命中时（如第 21 天既满 3 又满 7）**两档都发放**。

**赠送上限（19.10 已定：只受余额限制）**
45. 赠送金额等于余额时成功，余额变为 0。
46. 赠送金额超出余额 1 分即被拒绝。
47. 管理员代赠时，转出方余额不足同样被拒绝——**不能凭空生成积分**。

**两套账号体系隔离（最高优先级）**
48. 前台 token 调用任一 `/api/admin/*` 返回 **401**。
49. 后台 token 调用任一 `/api/app/*` 返回 **401**。
50. 普通成员调用 leader 专属接口（发布任务、审核任务、记一笔）返回 **403**。
51. 管理员**不出现在**成员列表、排行榜、记录中——`admins` 与 `members` 完全隔离。
52. 同一个字符串作为后台用户名和前台用户名可同时存在，互不干扰（两套独立命名空间）。
53. 后台变更组长后，**旧组长立即失去** leader 接口权限（不依赖重新登录）。
