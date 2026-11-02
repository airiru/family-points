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

- **Web = 管理后台**，面向管理员（成员、睡觉规则、任务、兑换、设备、审计）
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
  frozen_score    INT          NOT NULL DEFAULT 0,   -- 因发布任务被冻结的积分，不可动用
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
  streaks    JSON         NULL,             -- 连续奖励档位，结构见 7.3
  member_ids JSON         NULL,             -- 适用范围：[成员id]；NULL 表示全组生效
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

-- 任务（任何成员可发布；积分从发布者冻结后转给完成者）
CREATE TABLE tasks (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id     BIGINT UNSIGNED NOT NULL,
  title         VARCHAR(128) NOT NULL,
  points        INT          NOT NULL,        -- 悬赏分值，恒为正（从发布者转移给完成者）
  publisher_id  BIGINT UNSIGNED NOT NULL,     -- 发布者，也是悬赏积分的来源
  escrow_points INT          NOT NULL,        -- 冻结中的分值，正常等于 points
  assignee_id   BIGINT UNSIGNED NULL,         -- 指定完成人；NULL 表示全组可认领
  completed_by  BIGINT UNSIGNED NULL,         -- 实际完成者（通过后写入）
  due_at        DATETIME(3)  NULL,            -- 截止时间
  repeat_rule   VARCHAR(32)  NULL,            -- 'daily' | 'weekly' | NULL（暂不实现，预留）
  status        ENUM('open','closed') NOT NULL DEFAULT 'open',
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family_status (family_id, status, created_at DESC),
  KEY idx_publisher (publisher_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 任务提交（状态机：submitted → approved / rejected）
CREATE TABLE task_submissions (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id    BIGINT UNSIGNED NOT NULL,
  task_id      BIGINT UNSIGNED NOT NULL,
  member_id    BIGINT UNSIGNED NOT NULL,
  note         VARCHAR(255) NULL,             -- 孩子的说明
  image_url    VARCHAR(255) NULL,             -- 可选凭证
  status       ENUM('submitted','approved','rejected','expired') NOT NULL DEFAULT 'submitted',
  vote_deadline DATETIME(3) NULL,            -- 投票截止时间
  review_note  VARCHAR(255) NULL,            -- 驳回理由
  reviewed_at  DATETIME(3)  NULL,
  record_id    BIGINT UNSIGNED NULL,          -- 通过后生成的积分记录
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family_status (family_id, status, created_at DESC),
  KEY idx_task (task_id),
  KEY idx_task_member (task_id, member_id, status)  -- 「同一任务同一成员仅一条待审」由应用层事务保证
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 任务审核投票（除发布者外全员过半同意才通过）
CREATE TABLE task_votes (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id     BIGINT UNSIGNED NOT NULL,
  submission_id BIGINT UNSIGNED NOT NULL,
  voter_id      BIGINT UNSIGNED NOT NULL,     -- 投票成员
  vote          ENUM('approve','reject') NOT NULL,
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_submission_voter (submission_id, voter_id),  -- 一人一票
  KEY idx_submission (submission_id)
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
  night_rule_id    BIGINT UNSIGNED NULL     -- 达标时计入的睡觉规则（rules.id）
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

| 能力 | 后台管理员 | 组长（每组 1 人） | 普通成员 |
|---|---|---|---|
| 查看本组记录与统计 | ✅ | ✅ | ✅ |
| **发布 / 关闭任务** | ❌（管理员无积分） | ✅ | ✅ |
| **任务投票** | ❌（非成员，无投票权） | ✅ | ✅（发布者除外） |
| **积分赠送** | ✅ | ✅ | ✅ |
| 自助兑换 | ❌（无积分） | ✅ | ✅ 仅自己 |
| **增加 / 踢出成员** | ✅ | ✅ | ❌ |
| **配置睡觉时间与奖励规则** | ✅ | ✅ | ❌ |
| **商城商品管理** | ✅ | ✅ | ❌ |
| 设备管理（令牌） | ✅ | ❌ | ❌ |
| 审计日志 | ✅ | ❌ | ❌ |
| 家庭组设置 / 移交管理员 | ✅ | ❌ | ❌ |

**四条关键边界**：

1. **管理员不能发布任务**——任务需要冻结发布者的积分，而管理员不参与积分。
   管理员只能「审核」与「管理」，不能「悬赏」。
2. **任何人都不能审核自己提交的任务**——否则等于自己给自己发分（从发布者账上扣）。
3. **组长只有一名**（用户明确）。后台可指定任意成员为组长，变更即生效。
4. **已移除的权限**：手动「记一笔」、手动补录使用时长、删除记录——**三者全部取消**，
   后台与前台都不再提供。积分只能通过「任务 / 赠送 / 睡觉打卡」产生。

### 4.3 越权防护硬规则（最高优先级）

1. **Token 类型必须校验**：`/api/admin/*` 只接受 `typ='admin'`，`/api/app/*` 只接受 `typ='member'`。
   前台 token 打后台接口一律 401，反之亦然。
2. **所有 SQL 必须带 `family_id = ?`**。
3. **`family_id` 只能从 JWT claims 取**，禁止从请求体、query、路径参数取。
4. **跨组访问返回 404 而非 403**（避免泄露资源是否存在）。
5. **管理员账号不属于任何成员**，不得出现在成员列表、排行榜、记录中。
6. **配置即租户数据**：睡觉时间点、连续奖励档位、阈值、分值等**全部按 `family_id` 隔离**，
   存在数据库表里（`rules` / `screen_rules`），**不得写成全局常量、环境变量或配置文件**。
   A 组改配置绝不影响 B 组。任何"全局默认配置"都必须落库并带 `family_id`。

---

## 5. 账号与家庭组

### 5.1 注册：创建或加入家庭组

**用户可自助注册**，注册时必须二选一：**创建家庭组** 或 **加入家庭组**。

| 选择 | 系统动作 | 获得的身份 |
|---|---|---|
| **创建家庭组** | 建 `families` + `admins`（本人）+ `members`（本人，`role='leader'`） | **后台管理员 + 前台组长**（双身份） |
| **加入家庭组** | 输入邀请码，建 `members`（`role='member'`） | 仅前台成员 |

> **双身份说明**：创建者会生成**两条记录**（`admins` 一条、`members` 一条），
> 两套 token 分别签发。这是「两套账号系统」与「自助创建」的折中——
> 创建者既能管后台，又能参与积分，但**在排行榜上只以成员身份出现一次**。

| # | 功能 | 状态 |
|---|---|---|
| 5.1.1 | 注册页：选择「创建家庭组」或「加入家庭组」 | ⬜ |
| 5.1.2 | 创建家庭组（自动成为管理员 + 组长） | ⬜ |
| 5.1.3 | 通过邀请码加入家庭组 | ⬜ |
| 5.1.4 | 注册时校验：用户名全局唯一、密码强度、家庭组名非空 | ⬜ |
| 5.1.5 | 一个前台账号仅属一个家庭组 | ⬜ 由 `members.username` 全局唯一保证 |

### 5.2 后台账号（管理员）

| # | 功能 | 状态 |
|---|---|---|
| 5.2.1 | 后台登录 / 退出 | ⬜ `/api/admin/login` |
| 5.2.2 | 修改密码 | ⬜ |
| 5.2.3 | Token 刷新（Access 2h + Refresh 30d） | ⬜ |
| 5.2.4 | 同一家庭组可有多个管理员 | ⚠️ 待确认（19.19） |
| 5.2.5 | 移交管理员 | ⬜ 至少保留一个管理员，否则无人可管 |
| 5.2.6 | **创建者以外的人如何获得后台账号** | ⚠️ 待确认（19.20）：邀请 / 组长升级 / 不开放 |

### 5.3 前台账号（家庭成员）

| # | 功能 | 状态 |
|---|---|---|
| 5.3.1 | **账号一律本人自助注册**（Android 有注册页） | ⬜ **不存在代建账号**，任何人都不能为他人创建登录凭证 |
| 5.3.2 | 前台登录 / 退出 | ⬜ `/api/app/login` |
| 5.3.3 | 修改自己的密码 | ⬜ |
| 5.3.4 | 组长可添加**无账号成员**（占位，仅作为计分对象） | ⬜ 适用于婴幼儿；该成员不能登录、无投票权 |
| 5.3.5 | **首个成员自动成为组长** | ⬜ 创建家庭组时本人即组长 |
| 5.3.6 | 变更组长 | ⬜ **每组仅一名**；指定新组长后原组长自动降为普通成员 |
| 5.3.7 | **组长可增加 / 踢出成员**（前台） | ⬜ 增加仅限「无账号成员」；踢出后记录归属待定（19.2） |
| 5.3.8 | **组长可配置睡觉时间与奖励规则** | ⬜ 与后台同权，配置落库并带 `family_id` |
| 5.3.9 | **组长可管理商城商品** | ⬜ 增删改商品，不含代兑换 |

> **家长通常需要两个账号**：一个后台管理员（配置与维护），一个前台成员（参与积分）。
> 创建家庭组时会自动生成这两个身份，无需分别注册。
>
> **Android 端没有管理员角色**：前台只有组长与成员，且**任何人都不能替他人创建账号**——
> 成员必须自己注册（创建家庭组或输入邀请码加入）。组长能做的只是「添加无账号占位成员」。

### 5.4 家庭组

| # | 功能 | 状态 |
|---|---|---|
| 5.4.1 | 查看 / 重置邀请码 | ⬜ |
| 5.4.2 | 家庭组改名 | ⬜ |
| 5.4.3 | 退出家庭组（前台成员） | ⚠️ 记录归属待定（19.2） |
| 5.4.4 | 解散家庭组 | ⚠️ 高危，需输入组名二次确认 |
| 5.4.5 | 邀请码有效期与重置 | ⬜ 建议可重置，防止邀请码外泄 |

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

## 7. 睡觉规则（唯一的自动计分规则）

> **记一笔已移除，因此不存在「手动规则」。** 现在系统里只有一种规则：**睡觉规则**。
> 任务的分值在发布时直接指定，不走规则表。

| # | 功能 | 状态 |
|---|---|---|
| 7.1 | **睡觉规则（`screen_night`）** | ⬜ 唯一规则类型；每成员一条，见 12.4 |
| 7.2 | **睡觉时间点配置** | ⬜ `night_start_hour` / `night_end_hour` / `night_max_min` |
| 7.3 | **连续奖励档位（全部可配置）** | ⬜ 天数、分值、是否循环、档位数都由组长或管理员设置 |
| 7.4 | 连续打卡计数口径 | ✅ **按自然日**，一天最多计 1 次 |
| 7.5 | 里程碑去重 | ⬜ 以 `streak_award` 记录已发档位，避免重复发奖 |
| 7.6 | 配置按家庭组隔离 | ⬜ 落库带 `family_id`，无全局默认值 |

### 7.3 连续奖励档位结构

存在 `rules.streaks`（JSON 数组，**档位数不限，建议 ≤ 5**）：

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
- **`repeat` 与 `once` 的区别**：`repeat` 按倍数循环；`once` 只在达到阈值那一次发，
  要等连续中断重来才会再发。
- **中断即清零**：未达标或未上报 → 连续天数归零，所有档位重新可发。
- **改配置不追溯**：修改 `every` / `bonus` / `mode` 后，**历史已结算的日期不重算**。
- **配置按家庭组隔离**：存在 `rules` 表（带 `family_id`），**每个家庭组各配各的**。
  不存在「全局默认档位」，A 组改成「5 天 +8」不影响 B 组。

---

## 8. 积分来源与流转

> **本节是全局不变量说明**。任何改动积分的地方都必须符合下表，否则账目必然出错。

### 8.1 四种变动，两类性质

| 来源 | 性质 | 方向 | 家庭总积分 |
|---|---|---|---|
| **任务完成** | **转移** | 发布者 → 完成者 | **不变** |
| **积分赠送** | **转移** | 成员 → 成员 | **不变** |
| **睡觉打卡** | **生成** | 系统 → 成员 | **增加** |
| **商城兑换** | **消耗** | 成员 → 系统（销毁） | **减少** |

**推论**：

```
家庭总积分 = Σ(睡觉奖励) − Σ(兑换消耗)
```

转移操作**永远不改变总量**，只改变分布。这条是验收时的核心校验口径。

### 8.2 已移除的入口

| 移除项 | 原因 |
|---|---|
| **记一笔**（按规则 / 自定义） | 积分必须「有来源」——要么别人转移给你，要么系统奖励。凭空加分无法追溯 |
| **手动补录使用时长** | 同上，且与自动上报存在覆盖冲突 |
| **删除记录** | 不可逆，会破坏流水完整性 |

> **这三项在后台与前台都不再提供**，接口层也不保留（返回 404，不是隐藏按钮）。
> 需要修正错误时，只能用「反向操作」：任务驳回、赠送撤销、覆盖上报。

### 8.3 余额与冻结

| 字段 | 含义 |
|---|---|
| `members.score` | **可用余额**，可兑换、可赠送、可发布任务 |
| `members.frozen_score` | **冻结中**，因发布任务被预留，不能动用 |

- 成员总资产 = `score + frozen_score`
- 发布任务时：`score -= N`、`frozen_score += N`
- 任务通过时：发布者 `frozen_score -= N`，完成者 `score += N`
- 任务关闭 / 无人完成：发布者 `frozen_score -= N`、`score += N`（退回）

> **为什么需要冻结**：若不冻结，发布者可以先发任务、再把积分花光，
> 孩子完成后无分可给，等于白干。冻结保证「悬赏必有兑付能力」。

---

## 9. 兑换商城

| # | 功能 | 状态 |
|---|---|---|
| 9.1 | **商品增删改（组长或管理员）** | ⬜ 组长在前台即可管理，无需登录后台 |
| 9.2 | 成员自助兑换（Android） | ⬜ 仅能用**可用余额**，不含冻结积分 |
| 9.3 | 余额校验（前后端双重） | ⬜ |
| 9.4 | **兑换撤销** | ⬜ 仅管理员 |
| 9.5 | 并发扣减防超兑 | ⬜ **需在事务内 `SELECT ... FOR UPDATE`**，v1 的 KV 无此问题但 MySQL 有 |

> **兑换是积分唯一的「销毁」出口**：兑换后积分从家庭总池中消失（`score -= cost`），
> 不转移给任何人。这是家庭总积分减少的唯一途径（另一途径是无人认领任务退回，但那不减少总量）。

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
| 认证 | **仅设备令牌**（`X-Device-Token` 头）；手动上传已移除 |
| 数据来源 | 只有 `auto`（设备自动上报） |

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
| 12.2.9 | **采集并上报夜间时段使用分钟数**（`night_min`） | ⬜ 供「按时睡觉」自动判定使用，见 12.4 |

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

### 12.4 夜间时段判定（「按时睡觉」自动打卡）

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

> **配置的层级与隔离**：
> 睡觉时间点、允许时长、阈值分值都存在 `screen_rules` 表（**每成员一条**，带 `family_id`），
> 因此 **① 每个家庭组各配各的；② 同一组内还能按成员分别设**（如小学生 21:00、初中生 22:30）。
> 后台提供「**应用到全组**」的快捷操作，避免逐个配置。
> **不存在全局默认值**——任何默认值也必须落库并归属到具体 `family_id`。

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
> 代价是设备故障会误伤连签。**注意：手动补录已移除，因此这类误伤无法事后修正**，
> 连续天数只能从下一次达标重新累计。这是取消手动录入后必须接受的硬约束。

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

> 面向**家庭成员**（组长与成员），**不含后台管理**。
> **无注册入口**——前台账号由管理员在 Web 后台创建后下发。

| # | 界面 | 关键内容 | 可见角色 |
|---|---|---|---|
| 13.1 | 启动页 | 检查前台 token，有效则直进主页 | 全部 |
| 13.2 | 登录 / 注册 | 登录；注册时选择「创建家庭组」或「输入邀请码加入」 | 全部 |
| 13.3 | 积分榜 | 排名、奖牌、可用积分、冻结积分、连续天数 | 全部 |
| 13.4 | 成员详情 | 该成员记录（分页） | 全部 |
| 13.5 | 最近记录 | 分页、按成员筛选 | 全部 |
| 13.6 | 我的兑换 | 商城、余额校验、确认兑换 | 全部 |
| 13.7 | 赠送积分 | 选受赠成员、金额与附言、赠送历史 | 全部 |
| 13.8 | **任务** | 发布任务（选分值、指定完成人）、认领、提交、我的任务 | 全部 |
| 13.9 | **任务投票** | 待投票列表、同意/反对、实时票数（X/Y） | 全部（**发布者除外**） |
| 13.10 | 通知中心 | 未读列表、点击跳转、标记已读 | 全部 |
| 13.11 | 屏幕时间状态 | 权限状态、今日时长、上报状态 | 全部 |
| 13.12 | 设置 | 账号、家庭组信息、退出、改密码、头像 | 全部 |
| 13.13 | **成员管理** | 增加成员、踢出成员 | **仅组长** |
| 13.14 | **睡觉规则配置** | 睡觉时间点、允许时长、连续奖励档位 | **仅组长** |
| 13.15 | **商品管理** | 商品增删改 | **仅组长** |

**共 15 个界面**，其中 3 个（13.13–13.15）仅组长可见。

> **组长的管理能力放在前台**，不需要登录 Web 后台——这是本次权限重划的核心：
> 组长管「家庭内的事务」（成员、睡觉规则、商品），管理员管「系统与安全」（设备、审计、家庭组）。

---

## 14. Web 管理后台

> **仅管理员使用**（`admins` 账号）。不提供成员只读视图——成员看数据请用 Android。

| # | 页面 | 对应功能 |
|---|---|---|
| 14.1 | 登录 / 注册 | 后台管理员账号；注册即创建家庭组 |
| 14.2 | 成员管理 | 增删改、隐藏、设组长、头像；**不能代建账号**（仅可添加无账号占位成员） |
| 14.3 | 睡觉规则 | 睡觉时间点、允许时长、连续奖励档位（与组长同权） |
| 14.4 | 任务管理 | 查看全部任务与提交、代审核 |
| 14.5 | 兑换商城 | 商品管理、兑换撤销 |
| 14.6 | 赠送管理 | 赠送记录、撤销赠送 |
| 14.7 | 记录 | **只读**：分页、筛选、搜索、导出 CSV（**无删除**） |
| 14.8 | 统计 | 周期排行榜、积分趋势图 |
| 14.9 | 审计日志 | 按成员/动作/时间筛选，查看 before/after |
| 14.10 | 屏幕时间 | 查看上报历史（**只读，无手动录入**） |
| 14.11 | 设备管理 | 列表、生成令牌、吊销 |
| 14.12 | 家庭组设置 | 组名、邀请码、移交管理员 |

**共 12 个页面**。相比早期版本删除了「记一笔」页与「手动录入」入口。

---

## 15. 接口清单

响应统一 `{ ok: bool, data?: any, error?: string }`。
**两套前缀，两套鉴权，互不通用**：

- `/api/admin/*` —— 仅接受 `typ='admin'` 的 token
- `/api/app/*` —— 仅接受 `typ='member'` 的 token（组长与成员同池，靠 `role` 区分）
- `/api/device/*` —— 仅接受设备令牌

### 15.1 公开接口（无需鉴权）

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.1.1 | `/api/register` | POST | 注册：`mode=create`（建组+管理员+组长）或 `mode=join`（邀请码加入） |
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
| 15.2.5 | `/api/admin/member/*` | POST | 成员增删改、隐藏、设组长、登录设置 |
| 15.2.6 | `/api/admin/rule/*` | POST | 睡觉规则与连续奖励档位配置 |
| 15.2.7 | `/api/admin/item/*` | POST | 商品增删改、兑换撤销 |
| 15.2.8 | `/api/admin/task/*` | POST | 查看全部、代审核 |
| 15.2.9 | `/api/admin/audit/list` | GET | 审计日志查询 |
| 15.2.10 | `/api/admin/export/records` | GET | 导出 CSV |
| 15.2.11 | `/api/admin/device/*` | POST | 设备生成 / 吊销 |
| 15.2.12 | `/api/admin/state` | GET | 后台初始化数据 |

> **已移除**：`/api/admin/score/*`（记一笔）、`/api/admin/screen/report-manual`（手动录入）。
> 接口层**直接删除，返回 404**，不是隐藏。

### 15.3 前台接口（`typ='member'`）

**所有成员可用**

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.3.1 | `/api/app/password` | POST | 改自己的密码 |
| 15.3.2 | `/api/app/state` | GET | 本组概览（不含全量记录） |
| 15.3.3 | `/api/app/records` | GET | 记录分页 / 筛选 / 搜索（**只读**） |
| 15.3.4 | `/api/app/item/list` | GET | 商城列表 |
| 15.3.5 | `/api/app/item/redeem` | POST | 自助兑换 |
| 15.3.6 | `/api/app/transfer/send` | POST | 赠送积分 |
| 15.3.7 | `/api/app/transfer/list` | GET | 赠送历史 |
| 15.3.8 | `/api/app/task/list` | GET | 任务列表 |
| 15.3.9 | `/api/app/task/create` | POST | **发布任务**（需余额充足，冻结积分） |
| 15.3.10 | `/api/app/task/close` | POST | 关闭自己的任务（退回冻结） |
| 15.3.11 | `/api/app/task/submit` | POST | 提交完成 |
| 15.3.12 | `/api/app/task/vote` | POST | **投票**（同意/反对），发布者无投票权 |
| 15.3.13 | `/api/app/notification/list` | GET | 通知列表 |
| 15.3.14 | `/api/app/notification/read` | POST | 标记已读 |
| 15.3.15 | `/api/app/stats/ranking` | GET | 排行榜 |
| 15.3.16 | `/api/app/stats/trend` | GET | 趋势图 |
| 15.3.17 | `/api/app/member/avatar` | POST | 改自己的头像 |

**仅组长（`role='leader'`）**

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.3.18 | `/api/app/member/add` | POST | 添加**无账号占位成员**（不能设置登录凭证） |
| 15.3.19 | `/api/app/member/remove` | POST | 踢出成员（记录归属见 19.2） |
| 15.3.20 | `/api/app/rule/save` | POST | 睡觉时间点与连续奖励档位 |
| 15.3.21 | `/api/app/item/save` | POST | 商品增删改 |

### 15.4 设备接口（设备令牌）

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.4.1 | `/api/device/report` | POST | 上报使用时长（含 `night_min`） |

> **越权测试基线**：前台 token 打任一 `/api/admin/*` 必须 401；
> 后台 token 打任一 `/api/app/*` 必须 401；
> 普通成员打 15.3.18–15.3.21 必须 403；
> 发布者调用投票接口必须 403；非本组成员投票必须 404。

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
| 19.19 | 同一家庭组是否允许多个后台管理员 | 家庭场景单人足够；多人可分担但增加移交复杂度 |
| 19.20 | 创建者以外的人如何获得后台账号 | 邀请 / 组长自动升级 / 不开放，三选一 |
| 19.21 | 投票后能否改票 | 允许改票会反复拉扯；建议一票定终身 |

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
>   详见 7.4 与 12.4。
> - **19.10（赠送上限）** —— 2026-11-02 拍板为「**只受转出方已有积分限制**」：
>   不设单笔上限、不设每日上限；管理员代赠同样不能凭空生成积分。详见 10.1 与 10.5。
> - **19.18（组长权限边界）** —— 2026-11-02 拍板：**任何成员**可发布/关闭任务、审核任务、
>   查看本组全部记录与统计；**组长**额外可增加/踢出成员、配置睡觉时间与奖励规则、管理商城商品。
>   组长**每组仅一名**。详见 4.2、5.2、13 节。
> - **19.16 / 19.17（手动上传凭证与权限）** —— **随功能一并作废**：
>   手动补录已整体移除，相关决策不再需要。

---

## 20. 任务（悬赏与确认）

### 20.1 目的与模型

任务不再是「家长单向发分」，而是**成员之间的积分转移**：

```
发布者冻结 N 分 → 任务 open → 有人提交 → 审核通过 → N 分从发布者转给完成者
                                              └─ 驳回 → 可重新提交
                    └─ 关闭 / 无人完成 → N 分退回发布者
```

**任何成员都可以发布任务**（含组长与普通成员），只要余额足够。
**管理员不能发布**——管理员不参与积分，没有可冻结的余额。

### 20.2 状态机

```
tasks.status: open → closed
task_submissions.status: submitted → approved / rejected / expired
```

### 20.3 审核规则：全员投票，过半同意

**审核不是单人决定，而是投票制。**

| 项 | 规则 |
|---|---|
| 有投票权 | **有登录账号的成员 − 发布者**（完成者**可以**自投） |
| 通过条件 | **同意票 > 有投票权人数 ÷ 2**（严格过半） |
| 提前驳回 | 反对票达到「同意已不可能过半」时立即结束 |
| 投票期 | 提交后 **24 小时**，或任务截止时间（取较早者） |
| 超时未过半 | 状态置 `expired`，**不转移积分**，完成者可重新提交 |
| 一人一票 | `UNIQUE(submission_id, voter_id)` 保证 |

**过半判定示例**（完成者计入投票人数，可自投）：

| 有账号成员数 | 有投票权（除发布者） | 通过所需同意票 | 说明 |
|---|---|---|---|
| 5 | 4 | **3**（> 2） | 完成者自投后还需 2 人 |
| 4 | 3 | **2**（> 1.5） | 完成者自投后还需 1 人 |
| 3 | 2 | **2**（> 1） | 完成者自投后还需 1 人 |
| 2 | 1 | **1**（> 0.5） | 完成者自投即通过 |
| 1 | 0 | — | **无法过半，禁止发布任务** |

> **无账号成员不参与投票**：分母只统计**有登录账号**的成员。
> 婴幼儿占位成员没有账号，不投票，也不计入过半线——否则会人为抬高门槛。

### 20.4 单人家庭的边界

| 情况 | 处理 |
|---|---|
| 有账号成员 = 1（只有自己） | **禁止发布任务**——除发布者外无人可投，永远无法过半 |
| 有账号成员 ≥ 2 | 正常投票，完成者可自投 |

> **为什么 2 人家庭不降级**：完成者自投是**允许**的（用户明确），
> 2 人家庭里完成者投自己一票即 1/1 > 0.5，直接通过。这是规则的自然结果，不需要特例。
> 代价是 2 人家庭的审核实质上是「完成者自证」——但发布者仍可关闭任务、不兑付，
> 且拒绝完成者需要另一名成员，结构上无法更强。

### 20.5 功能项

| # | 功能 | 状态 |
|---|---|---|
| 20.5.1 | **发布任务**（标题、悬赏分值、指定完成人、截止时间） | ⬜ 任何成员可发，需余额充足 |
| 20.5.2 | **发布时冻结积分**（`score → frozen_score`） | ⬜ 保证「悬赏必有兑付能力」 |
| 20.5.3 | 任务列表（可认领 / 进行中 / 已完成） | ⬜ |
| 20.5.4 | 提交完成（附说明，可选附图） | ⬜ |
| 20.5.5 | **待投票列表**（除发布者外全员可见） | ⬜ |
| 20.5.6 | **投票（同意 / 反对）** | ⬜ 一人一票，可改票？建议不可改 |
| 20.5.7 | 实时票数展示（已同意 X / 需要 Y） | ⬜ |
| 20.5.8 | **达成过半 → 自动通过并转移积分** | ⬜ |
| 20.5.9 | 反对票足以否决 → 自动驳回 | ⬜ |
| 20.5.10 | 超时未过半 → `expired`，冻结不动、任务保持 open | ⬜ |
| 20.5.11 | 关闭任务 → 冻结积分退回发布者 | ⬜ |
| 20.5.12 | 重复任务（每日/每周） | ⛔ 暂不实现，`repeat_rule` 预留 |

### 20.6 结算事务

**发布任务**：

```sql
BEGIN;
  SELECT id, score FROM members WHERE id = :publisher AND family_id = :fid FOR UPDATE;
  -- 校验 score >= points
  UPDATE members SET score = score - :points, frozen_score = frozen_score + :points
   WHERE id = :publisher;
  INSERT INTO tasks (..., publisher_id, escrow_points = :points, status='open');
COMMIT;
```

**投票通过（核心）**：

```sql
BEGIN;
  -- 1. 幂等：条件更新，受影响行数 0 说明已处理
  UPDATE task_submissions SET status='approved', reviewed_at=NOW(3)
   WHERE id=? AND status='submitted';

  -- 2. 冻结释放 → 转给完成者
  UPDATE members SET frozen_score = frozen_score - :points WHERE id = :publisher;
  UPDATE members SET score = score + :points WHERE id = :completer;

  -- 3. 写入两条流水（发布者 -N、完成者 +N），source='task'
  INSERT INTO records (...) VALUES (...), (...);

  UPDATE tasks SET completed_by = :completer, status='closed' WHERE id = :task;
COMMIT;
```

**投票时必须重新统计**，不能只凭客户端传来的票数：

```sql
SELECT
  SUM(vote='approve') AS approve_cnt,
  SUM(vote='reject')  AS reject_cnt,
  (SELECT COUNT(*) FROM members WHERE family_id=? AND id<>:publisher) AS eligible
FROM task_votes WHERE submission_id=?;
```

> 判定与「转移积分」**必须在同一事务内**，否则并发投票可能触发两次转移。

### 20.7 边界规则

| 场景 | 处理 |
|---|---|
| 发布时余额不足 | 拒绝，提示当前可用余额 |
| **发布者 = 完成者** | **拒绝**（自己给自己悬赏无意义，且可绕过余额校验） |
| **发布者参与投票** | **禁止**（发布者无投票权） |
| **完成者自投** | **允许**（计入同意票） |
| 无账号成员投票 | 不可能（无登录凭证）；也不计入分母 |
| 同一人重复投票 | 拒绝（`UNIQUE(submission_id, voter_id)`） |
| 投票后再改票 | **建议不允许**，避免反复拉扯 |
| 任务已关闭后提交 | 拒绝 |
| 同一任务重复提交 | 拒绝（已有 `submitted`） |
| 删除有提交记录的任务 | 拒绝，改为关闭（保留历史） |
| 悬赏分值 ≤ 0 | 拒绝（任务只做转移，不做惩罚） |
| 投票期内成员被踢出 | 该成员票作废，重新计算过半线 |

> **为什么禁止「发布者 = 完成者」**：若允许，成员可以发布「给自己 +5」的任务并自己审核通过，
> 等于凭空加分——绕过了「积分必须有来源」这条不变量。

---

## 21. 操作审计日志

### 21.1 目的

管理员可任意改分、清零、删记录，**当前无任何追溯**。多租户环境下，一旦积分对不上账，无法定位。
这是清单中**优先级最高的技术缺口**——不是体验问题，是数据可信度问题。

### 21.2 记录范围

| 动作 | action 值 | 是否记 before/after |
|---|---|---|
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
4. A 组修改睡觉时间点、连续奖励档位后，B 组的配置与结算结果完全不受影响。
5. 配置接口不接受「全局默认值」参数——任何配置写入都必须落到具体 `family_id`。

**账号与权限隔离**
6. 前台 token 调用任一 `/api/admin/*` 返回 **401**。
7. 后台 token 调用任一 `/api/app/*` 返回 **401**。
8. 普通成员调用组长专属接口（成员增删、睡觉规则、商品管理）返回 **403**。
9. 管理员**不出现在**成员列表、排行榜、记录中。
10. 管理员调用「发布任务」被拒绝——管理员不参与积分，无冻结来源。
11. **发布者调用投票接口**返回 403（发布者无投票权）。
12. **发布者与完成者相同**的任务被拒绝。
13. 后台变更组长后，旧组长**立即失去**组长接口权限（不依赖重新登录）。
14. 变更组长后，**组内仍只有一名** `leader`。

**积分不变量（核心账目）**
15. 任务通过前后，**家庭总积分不变**（发布者 -N，完成者 +N）。
16. 赠送前后，**家庭总积分不变**。
17. 睡觉奖励使家庭总积分**增加**。
18. 兑换使家庭总积分**减少**。
19. `frozen_score` **不计入可用余额**——冻结中的积分不能兑换、不能赠送、不能再发布任务。

**任务经济**
20. 发布任务时可用余额不足 → 拒绝，且不产生任务记录。
21. 发布成功后：发布者 `score -= N`、`frozen_score += N`。
22. 关闭任务后：发布者 `frozen_score -= N`、`score += N`（退回）。
23. 审核通过后：发布者 `frozen_score -= N`，完成者 `score += N`。
24. 同一提交被并发投票触发两次通过，只转移一次积分（条件更新受影响行数为 0 即返回成功）。
25. 悬赏分值 ≤ 0 时拒绝创建。

**任务投票（过半同意）**
26. 5 人家庭（发布者除外 4 人）：3 票同意即通过；2 票同意不通过。
27. 4 人家庭（发布者除外 3 人）：2 票同意即通过。
28. 3 人家庭（发布者除外 2 人）：需 2 票全票同意才通过。
29. **2 人家庭**：完成者自投 1 票即 1/1 过半，**直接通过**（不走降级）。
30. **有账号成员仅 1 人**时禁止发布任务（除发布者外无人可投）。
31. **无账号占位成员不计入投票分母**——3 个有账号 + 2 个无账号 = 分母按 3 算。
32. 同一人重复投票被拒绝（`UNIQUE(submission_id, voter_id)`）。
33. 反对票达到「同意已不可能过半」时立即驳回，不必等满 24 小时。
34. 投票超时未过半 → 状态 `expired`，**积分不转移**，冻结仍归发布者。
35. 投票期内成员被踢出，其票作废并**重新计算过半线**。

**睡觉规则**
36. 配置「0 点前放下手机」（0:00–7:00、允许 0 分钟）：夜间使用 0 分钟 → 记 +N，连续 +1。
37. 夜间使用 45 分钟 → 记 0 分说明记录，连续天数清零。
38. 当日未上报 → 视为未达标，连续天数清零。
39. 0:00–07:00 的数据归属**前一天**，不归到当天。
40. 一次上报同时触发日总量与夜间两条记录；覆盖时两条都被正确回滚，无孤儿记录。
41. `repeat` 档在第 6、9 天再次触发，`once` 档只触发一次。
42. 修改睡觉时间点或奖励档位后，历史已结算日期**不追溯重算**。
43. 配置 `every = 0` 或 `bonus = 0` 时保存被拒绝；同组内 `every` 重复时保存被拒绝。
44. 多档同时命中时（如第 21 天既满 3 又满 7）两档都发放。

**已移除功能必须真的不存在**
45. `/api/admin/score/*`（记一笔）返回 **404**，不是 403 或静默成功。
46. 记录接口**不提供删除方法**（DELETE 返回 405）。
47. 不存在手动补录使用时长的接口（404）。
48. 后台与前台界面均**不出现**「记一笔」「手动录入」「删除记录」入口。

**并发与事务**
49. 同一成员并发兑换 10 次，余额不出现负数。
50. 同一份屏幕时间重复上报 10 次，积分只变一次。
51. A 与 B 并发互赠，不出现死锁（按 id 升序加锁）。
52. 撤销赠送后，双方积分与记录均正确回滚；转出方余额不足时撤销被拒绝。
53. 事务中途失败时，积分、记录、结算行三者不会出现半更新状态。

**审计日志**
54. 能查到每一次积分变动、任务审核、赠送、配置修改的 before/after。
55. 审计日志中**不含任何 `pass_hash`**。
56. 审计日志写入失败时，主流程仍然成功。

**部署**
57. 服务器重启后所有服务自动拉起。
58. 备份文件能在**空库**上完整恢复（真做一次）。
59. 头像目录也在备份范围内（恢复后头像不丢）。
60. HTTPS 证书自动续期正常。
61. MySQL 与 Go 端口不对外暴露。

**Android**
62. 未授权 `PACKAGE_USAGE_STATS` 时给出引导，不崩溃。
63. 杀后台后打开 App 能补齐遗漏日期。
64. 设备令牌吊销后立即 401。
