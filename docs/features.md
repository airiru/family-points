# 家庭积分榜 · 功能清单 v3

> 状态：`✅ 已实现` / `⬜ 待实现` / `⚠️ 需确认` / `⛔ 已知缺口`
> 最后更新：2026-11-03
> **v3 是自建服务器方案，v1（KV）与 v2（D1）的设计已全部作废。**

## 架构演进记录

| 版本 | 存储 | 后端 | 客户端 | 部署 | 状态 |
|---|---|---|---|---|---|
| v1 | Cloudflare KV | Worker (JS) | Web only | Cloudflare Pages | **代码已删除**（2026-11-03） |
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
- ⇒ Android **不再承载后台管理界面**，共 **15 个界面**（其中 3 个仅组长可见，见 13 节）

> **19.1 已关闭（2026-11-02）**：Web **不保留**成员只读视图，成员看数据一律用 Android。

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
  invite_code_expires_at DATETIME(3) NULL,    -- 邀请码 7 天有效期（5.2.5），重置即改写
  -- 不设 owner_member_id：组长身份唯一来源是 members.role='leader'（见设计要点 6），
  -- 两处都存必然分叉
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 被踢出 / 自行退出后的禁入名单（同组不可再次加入）
CREATE TABLE family_bans (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id  BIGINT UNSIGNED NOT NULL,
  member_id  BIGINT UNSIGNED NOT NULL,
  reason     ENUM('kicked','left') NOT NULL,
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_family_member (family_id, member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 后台管理员账号（与前台成员完全独立，不参与积分）
CREATE TABLE admins (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  username   VARCHAR(20)  NOT NULL UNIQUE,
  pass_hash  VARCHAR(60)  NOT NULL,          -- bcrypt
  must_change_password TINYINT(1) NOT NULL DEFAULT 1,  -- 首次登录强制改密（含默认账号）
  disabled   TINYINT(1)   NOT NULL DEFAULT 0,          -- 停用后无法登录（19.19）
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
  -- 注意：管理员是「网站运营者」，不属于任何家庭组，可跨组查看与管理。
  -- 家庭用户（含组长）永远没有后台账号。
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 成员（账号即成员：username 非空即可登录前台）
CREATE TABLE members (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id       BIGINT UNSIGNED NULL,       -- 注册时为空；被组长邀请或自行创建家庭组后才有值
  name            VARCHAR(32)  NOT NULL,
  score           INT          NOT NULL DEFAULT 0,
  frozen_score    INT          NOT NULL DEFAULT 0,   -- 因发布任务被冻结的积分，不可动用
  hidden          TINYINT(1)   NOT NULL DEFAULT 0,
  avatar          VARCHAR(64)  NULL,          -- **预设头像标识**（如 'bear'），不是 URL；无自定义上传
  username        VARCHAR(20)  NOT NULL UNIQUE,  -- 注册即创建账号，全局唯一 ⇒ 一人只属一个家庭组
  pass_hash       VARCHAR(60)  NOT NULL,         -- bcrypt
  role            ENUM('leader','member') NOT NULL DEFAULT 'member',  -- 组长/成员（前台角色）
  -- 「每组仅一名组长」下沉到数据库：组长时取 family_id，否则 NULL；UNIQUE 允许多个 NULL
  leader_family_id BIGINT UNSIGNED GENERATED ALWAYS AS (IF(role='leader', family_id, NULL)) STORED,
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family (family_id),
  UNIQUE KEY uk_leader_per_family (leader_family_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 计分规则
CREATE TABLE rules (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id  BIGINT UNSIGNED NOT NULL,
  name       VARCHAR(64)  NOT NULL,
  points     INT          NOT NULL,         -- 达标奖励分（睡觉规则唯一类型，见 7.4）
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

-- 兑换单（P4 已实现，与 0003_ledger 迁移一致）
CREATE TABLE redemptions (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id  BIGINT UNSIGNED NOT NULL,
  member_id  BIGINT UNSIGNED NOT NULL,
  item_id    BIGINT UNSIGNED NULL,          -- 商品可能被删，允许为空
  item_name  VARCHAR(64)  NOT NULL,         -- 快照
  cost       INT          NOT NULL,         -- 价格快照
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  revoked_at DATETIME(3)  NULL,             -- 预留：兑换撤销（当前不做）
  KEY idx_family_time (family_id, created_at DESC),
  KEY idx_member (member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 商品券（兑换后生成，与兑换单 1:1）
CREATE TABLE coupons (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id     BIGINT UNSIGNED NOT NULL,
  member_id     BIGINT UNSIGNED NOT NULL,   -- 持有者 = 兑换者
  redemption_id BIGINT UNSIGNED NOT NULL,   -- 来源兑换单
  item_name     VARCHAR(64)  NOT NULL,      -- 快照，商品改名/删除不影响已发的券
  cost          INT          NOT NULL,
  status        ENUM('unused','pending','redeemed') NOT NULL DEFAULT 'unused',
  reject_count  INT          NOT NULL DEFAULT 0,   -- 被驳回次数
  vote_deadline DATETIME(3)  NULL,          -- 核销投票截止
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  settled_at    DATETIME(3)  NULL,          -- 核销完成时间
  UNIQUE KEY uk_redemption (redemption_id),
  KEY idx_member_status (member_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 商品券核销投票（与 task_votes 同构）
CREATE TABLE coupon_votes (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id  BIGINT UNSIGNED NOT NULL,
  coupon_id  BIGINT UNSIGNED NOT NULL,
  voter_id   BIGINT UNSIGNED NOT NULL,
  vote       ENUM('approve','reject') NOT NULL,
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_coupon_voter (coupon_id, voter_id),
  KEY idx_coupon (coupon_id)
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
  source       ENUM('screen','night','task','transfer','redeem') NOT NULL,  -- 与 8.1 四种变动对应
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
  assignee_id   BIGINT UNSIGNED NULL,         -- 当前接取者；NULL = 待接取。除发布者外任何成员可接
  completed_by  BIGINT UNSIGNED NULL,         -- 通过后的实际完成者（通常等于 assignee_id）
  due_at        DATETIME(3)  NULL,            -- 截止时间
  repeat_rule   VARCHAR(32)  NULL,            -- 'daily' | 'weekly' | NULL（暂不实现，预留）
  status        ENUM('open','taken','closed') NOT NULL DEFAULT 'open',
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
  note         VARCHAR(255) NOT NULL,         -- 提交说明：**唯一凭证，必填**（无图片，19.11 定）
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
  family_id    BIGINT UNSIGNED NULL,          -- 管理员登录等系统事件可为空
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
  source      ENUM('auto') NOT NULL DEFAULT 'auto',           -- 手动上传已移除，只有设备自动上报
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

-- Refresh Token（可吊销；原文不落库，只存 SHA-256）
CREATE TABLE refresh_tokens (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  subject_type     ENUM('admin','member') NOT NULL,  -- 两套账号体系共用一张表
  subject_id       BIGINT UNSIGNED NOT NULL,
  token_hash       CHAR(64)     NOT NULL UNIQUE,     -- sha256 hex
  expires_at       DATETIME(3)  NOT NULL,
  revoked_at       DATETIME(3)  NULL,                -- 轮换/改密/登出后置位
  replaced_by_hash CHAR(64)     NULL,                -- 轮换出的新 token，便于追溯
  created_at       DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_subject (subject_type, subject_id),
  KEY idx_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

```

**设计要点（2026-11-03 随 P0 建表校正）：**

1. **`members.username` 非空且全局唯一** —— 系统里**不存在「无账号成员」**（决策 24），
   所以它不能为空；`UNIQUE` 同时保证「一个账号只属一个家庭组」。
   （原文写「可空且唯一」是 v2 遗留，已作废。）
2. **`records.member_name` 快照** —— 成员改名不污染历史。
3. **`UNIQUE(member_id, report_date)`** —— 幂等下沉到数据库，重试不会产生重复结算。
4. **不建外键约束** —— 一致性由 service 层事务 + 唯一索引保证。
   不建外键是为了避免级联误删与迁移时的锁风险；**删除成员时由 service 显式处理记录**。
5. **`records.source` 只有五个值**：`screen`（屏幕时间日总量）、`night`（睡觉打卡）、
   `task`（任务转移）、`transfer`（赠送转移）、`redeem`（兑换消耗）—— 与 8.1 的四种变动一一对应。
   `manual` / `device` 已随「记一笔」「手动上传」移除而废弃。
6. **组长身份唯一来源是 `members.role`** —— `families` 表**不设** `owner_member_id`，
   两处都存必然分叉。「每组仅一名组长」用生成列 + UNIQUE 下沉到数据库：`leader_family_id = IF(role='leader', family_id, NULL)`，
   `UNIQUE(leader_family_id)` —— 多个 NULL 不冲突，因此每组最多一名 `leader`。
   变更组长必须在同一事务里「旧组长降级 + 新组长升级」，否则第二条 UPDATE 会撞唯一键。
7. **连续天数不落计数器** —— 由 `screen_reports` 按自然日回溯计算（原 `members.streak_reset_at` 已删）。
   计数器在「覆盖上报」时极难回滚正确，回溯计算天然幂等。

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

> **管理员是网站运营者**，不属于任何家庭组，**可管理所有家庭组的所有内容**；
> **组长是家庭用户**，只在本组内有权，**没有任何后台权限**。

| 能力 | 运营者（后台） | 组长（每组 1 人，前台） | 普通成员（前台） |
|---|---|---|---|
| 后台入口 | ✅ | ❌ **永远没有** | ❌ |
| **跨组管理所有家庭组** | ✅ **全量** | ❌ | ❌ |
| 管理员账号管理（新增/停用） | ✅ | ❌ | ❌ |
| 查看本组记录与统计 | ✅ | ✅ | ✅ |
| 发布 / 关闭任务 | ❌（非成员，无积分） | ✅ | ✅ |
| 任务投票 | ❌（非成员） | ✅ | ✅（发布者除外） |
| 积分赠送 | ❌（非成员） | ✅ | ✅ |
| 自助兑换 | ❌（无积分） | ✅ | ✅ 仅自己 |
| **邀请成员 / 踢出成员** | ✅ | ✅ | ❌ |
| **生成 / 重置邀请码** | ✅ | ✅ | ❌ |
| **配置睡觉时间与奖励规则** | ✅ | ✅ | ❌ |
| **商城商品管理** | ✅ | ✅ | ❌ |
| 设备管理（令牌） | ✅ | ✅ | ❌ |
| 审计日志 | ✅ | ❌ | ❌ |
| 解散家庭组 | ✅ | ❌ | ❌ |

**五条关键边界**：

1. **家庭用户永远没有后台账号**——包括组长。后台是运营者的工具，不是家长的。
2. **管理员不能发布组内任务、不能投票、不能兑换**——管理员不是家庭成员，没有积分。
   > **未来例外（见 25 节，暂不实现）**：将来做全服任务时，管理员发的不是个人积分
   > 而是**家庭星**（另一本账，平台计价）。不写 `records`、不动 `members.score`。
   > 当前不存在全服任务，本条仍按原文执行。
3. **组长只有一名**，且只在本组内有效；变更后原组长立即降为普通成员。
4. **已移除的权限**：手动「记一笔」、手动补录使用时长、删除记录——**三者全部取消**。
5. **退出 / 踢出后积分清零且禁止回组**（见 5.3）。

### 4.3 越权防护硬规则（最高优先级）

1. **Token 类型必须校验**：`/api/admin/*` 只接受 `typ='admin'`，`/api/app/*` 只接受 `typ='member'`。
   前台 token 打后台接口一律 401，反之亦然。
2. **前台 SQL 必须带 `family_id = ?`**；`family_id` 只能从 JWT claims 取，
   禁止从请求体、query、路径参数取。跨组访问返回 **404**（避免泄露资源是否存在）。
3. **后台管理员是例外**：`typ='admin'` 可跨组操作，但**必须显式传入目标 `family_id` 且写审计日志**。
   管理员 token 不允许调用 `/api/app/*`。
4. **管理员账号不属于任何成员**，不得出现在成员列表、排行榜、记录中。
5. **实现落点**（2026-11-03，P2 完成）：前台组内接口统一挂 `RequireFamily` 中间件，
   `family_id` 从 `members.family_id` 读；管理员跨组接口一律 `:family_id` 路径参数，
   解析失败/组不存在都返回 **404**（连 400 都不用，避免用状态码差异探测 id 是否存在）。
   跨组访问先记应用日志，P8 换成写 `audit_logs`。
6. **配置即租户数据**：睡觉时间点、连续奖励档位、阈值、分值等**全部按 `family_id` 隔离**，
   存在数据库表里（`rules` / `screen_rules`），**不得写成全局常量、环境变量或配置文件**。

---

## 5. 账号与家庭组

### 5.1 注册：用户自己创建账号

**注册就是创建账号**。注册后 `family_id` 为空，此时还不能参与积分。

| # | 功能 | 状态 |
|---|---|---|
| 5.1.1 | 注册账号（用户名、密码、昵称） | ⬜ 用户名全局唯一 |
| 5.1.2 | 注册时可选**创建家庭组**（成为该组组长） | ⬜ **仅获得前台组长身份，不获得任何后台权限** |
| 5.1.3 | 一个账号只能属于一个家庭组 | ⬜ `members.username` 全局唯一保证 |
| 5.1.4 | 用户名规则 | ✅ 3–20 位，仅字母 / 数字 / 下划线 |
| 5.1.5 | 密码规则 | ✅ 至少 8 位、最多 72 字节（bcrypt 上限），**不要求复杂度** |

> ⚠️ **创建家庭组 ≠ 获得后台管理员**。后台是给**网站运营者**用的，
> 家庭用户（包括组长）**永远没有后台账号**，也没有任何后台入口。

### 5.2 加入家庭组：邀请码 / 组长邀请

| # | 功能 | 状态 |
|---|---|---|
| 5.2.1 | **组长生成 / 重置邀请码** | ✅ 每组一个，重置后旧码立即失效 |
| 5.2.2 | **用户输入邀请码加入家庭组** | ✅ 校验：码有效、本人 `family_id` 为空、未被该组禁入 |
| 5.2.3 | 组长按用户名直接邀请 | ⬜ 对方确认后加入；与邀请码并存 |
| 5.2.4 | 加入后 `family_id` 设为该组，`role='member'` | ✅ |
| 5.2.5 | **邀请码有效期 7 天** | ✅ 过期后需组长重置；重置后旧码立即失效 |

### 5.3 退出与踢出（积分清零 + 禁入）

| # | 功能 | 状态 |
|---|---|---|
| 5.3.1 | 成员自行退出家庭组 | ✅ **必须二次确认**（前端弹窗 + 后端 `confirm=true`） |
| 5.3.2 | 组长踢出成员 | ✅ 同样二次确认；不能踢自己 |
| 5.3.3 | **退出 / 踢出后积分清零** | ✅ `score=0`、`frozen_score=0`；积分不带走、不退还 |
| 5.3.4 | **禁止再次加入该家庭组** | ✅ 写入 `family_bans`，`UNIQUE(family_id, member_id)` |
| 5.3.5 | 退出 / 踢出后 `family_id` 置 NULL | ✅ 可加入**其他**家庭组 |
| 5.3.6 | 历史记录保留在原组 | ✅ 记录不删除（`member_name` 快照仍在） |
| 5.3.7 | 冻结中的积分（发布了未结任务） | ⚠️ 待 P6：任务表还没写入路径，届时在**同一事务**里关闭任务 |
| 5.3.8 | **组长不能直接退出** | ✅ 必须先转让组长，否则 409（否则组内会没人能管理） |

> **积分清零的三条含义**：
> ① 该成员在原组的 `score` 与 `frozen_score` 归零；
> ② 原组的历史记录保留（审计与流水完整），但不再计入其个人余额；
> ③ 未结算任务一并关闭，冻结积分不退回（因为余额已清零）。

### 5.4 后台账号（网站运营者）

| # | 功能 | 状态 |
|---|---|---|
| 5.4.1 | **系统自带一个默认管理员账号** | ✅ `go run ./cmd/migrate -seed-admin`，取 `ADMIN_USERNAME`/`ADMIN_PASSWORD`（开发库为 `admins`/`admins`）；**已有管理员则跳过**，幂等 |
| 5.4.2 | **默认账号首次登录强制改密** | ⬜ 未改密前除改密接口外全部拒绝 |
| 5.4.3 | 运营方可在后台新增 / 停用管理员账号 | ⬜ 账号管理本身也是后台功能 |
| 5.4.4 | 后台登录 / 退出 | ⬜ `/api/admin/login` |
| 5.4.5 | 修改密码 | ⬜ |
| 5.4.6 | Token 刷新（Access 2h + Refresh 30d） | ⬜ |
| 5.4.7 | **可管理所有家庭组的所有内容** | ⬜ 成员、规则、任务、商品、设备、数据、审计 |
| 5.4.8 | 管理员操作全部写审计日志 | ⬜ 因为可跨组，追溯更重要 |

> **后台账号与前台账号毫无关系**：
> - **不在同一张表**（`admins` vs `members`），字段、密码、token 全部独立
> - 后台账号**不绑定任何家庭组**，也不参与积分
> - 前台账号（含组长）**永远无法登录后台**，反之亦然
> - **不存在「家庭用户兼管理员」的情况**
>
> **管理员是运营方，权限是全量的**：可以管理任意家庭组的任意内容。
> 唯一不能做的是「以成员身份参与积分」——发布任务、投票、兑换、赠送都需要成员身份与积分余额，
> 管理员不具备。

### 5.5 家庭组

| # | 功能 | 状态 |
|---|---|---|
| 5.5.1 | 家庭组改名 | ✅ 组长可改 |
| 5.5.2 | 解散家庭组 | ⚠️ 高危，需输入组名二次确认；仅运营者在后台可执行 |
| 5.5.3 | 组长转让 | ✅ 组长发起，同事务「先降旧、再升新」；每组仅一名（唯一索引兜底） |

---

## 6. 成员管理

| # | 功能 | 状态 |
|---|---|---|
| 6.1 | 添加成员（由组长邀请已注册用户，见 5.3） | ⬜ |
| 6.2 | **踢出成员**（= 5.3 退出流程，**历史记录保留**） | ✅ 原文写「连带记录」与 5.3.6 冲突，已按 5.3 修正 |
| 6.3 | 隐藏 / 显示成员 | ✅ 只影响展示，不影响数据 |
| 6.4 | 积分清零 | ⬜ |
| 6.5 | 查看成员记录（分页） | ⬜ |
| 6.6 | 为成员设置登录与角色 | ⬜ |
| 6.7 | **成员改名** | ✅ 本人或组长可改；历史记录因快照不受影响 |
| 6.8 | 成员排序 | ⬜ 按积分降序 |

---

## 7. 睡觉规则（唯一的自动计分规则）

> **记一笔已移除，因此不存在「手动规则」。** 现在系统里只有一种规则：**睡觉规则**。
> 任务的分值在发布时直接指定，不走规则表。

| # | 功能 | 状态 |
|---|---|---|
| 7.1 | **睡觉规则（`screen_night`）** | ✅ 唯一规则类型；每成员通过 `screen_rules.night_rule_id` 引用 |
| 7.2 | **睡觉时间点配置** | ✅ `night_start_hour` / `night_end_hour` / `night_max_min` |
| 7.3 | **连续奖励档位（全部可配置）** | ✅ 天数、分值、循环方式、档位数都可配，保存时校验 |
| 7.4 | 连续打卡计数口径 | ✅ **按自然日**，一天最多计 1 次 |
| 7.5 | 里程碑去重 | ✅ 靠「`repeat` 按倍数 / `once` 仅在 `n == every` 时发」天然去重，`streak_award` 只做记录 |
| 7.6 | 配置按家庭组隔离 | ✅ 落库带 `family_id`；**建组时写入一份初始配置**（不是全局默认值） |

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

- **多档独立计算**：两档**都是 `repeat`** 时，第 21 天既满 3 的倍数又满 7 的倍数 → 两档**都发**。
  （若 7 天那档是 `once`，则只在第 7 天发，第 21 天不再发——这是 `once` 的定义，不是 bug。）
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

> **铁律（2026-11-03 用户明确）：每一次积分变动都必须写一条 `records`**。
> 包括兑换（负分）、赠送（两条：转出 -N / 转入 +N）。
> 将来若做撤销，也只能**再写反向记录，不删原记录**（当前无撤销功能，见 9.4 / 10.2.6）。
> 不允许任何「只改 `members.score` 不写流水」的路径 —— 否则账目无法对账。
> 唯一的例外是开发脚手架 `cmd/devtool`（只用于准备测试余额，APP_ENV=dev 才可运行）。

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
| 9.1 | **商品增删改（组长或管理员）** | ✅ 组长在前台管理；管理员入口待 P8 |
| 9.2 | 成员自助兑换（Android） | ✅ 仅用**可用余额**，冻结积分不动 |
| 9.3 | 余额校验（事务内） | ✅ 锁成员行后校验，失败不写流水、不扣分 |
| 9.4 | 兑换撤销 | ⚠️ **暂不做**（2026-11-03 用户明确）；接口与代码已移除，`redemptions.revoked_at` 为预留字段 |
| 9.5 | 并发扣减防超兑 | ✅ `SELECT ... FOR UPDATE` 锁成员行 |

> **兑换是积分唯一的「销毁」出口**：兑换后积分从家庭总池中消失（`score -= cost`），
> 不转移给任何人。这是家庭总积分减少的唯一途径（另一途径是无人认领任务退回，但那不减少总量）。

### 9.6 商品券（兑换后的核销流程）

**兑换不再是一次性动作，而是「扣分 → 发券 → 用券 → 投票核销」的闭环。**

```
兑换商品 → 扣分 → 生成「商品券」（unused）
                     └─ 点击使用 → 发起核销（pending，进入投票）
                          ├─ 全员过半同意 → redeemed（已核销，流程结束）
                          ├─ 反对足够 / 超时 → 回到 unused（券仍有效，可再次发起）
                          └─ 有投票权为 0 人（1 人家庭）→ 直接 redeemed
```

| # | 功能 | 状态 |
|---|---|---|
| 9.6.1 | 兑换成功后**自动生成商品券** | ✅ 与扣分同事务；券记录 `item_name` / `cost` 快照 |
| 9.6.2 | **我的券**列表（未使用 / 待核销 / 已核销） | ✅ Android 13.7 |
| 9.6.3 | **点击使用** → 发起核销，进入投票 | ✅ 行锁保证同一张券只有一个 pending |
| 9.6.4 | **核销投票**（除申请人外全员过半同意） | ✅ 与任务投票同构，见 20.3 |
| 9.6.5 | 投票通过 → 券置 `redeemed` | ✅ 记录 `settled_at` |
| 9.6.6 | 投票驳回 / 超时 → 券回到 `unused` | ✅ `reject_count + 1`，可再次发起 |
| 9.6.7 | 券的有效期 | ✅ **长期有效**（2026-11-03 拍板）——不设过期，无需定时清理 |

**为什么需要投票核销**：兑换时积分已经扣了，但东西还没真正拿到。
券是「已付钱待提货」的凭证；核销投票是家人确认「确实兑现了」，
避免「兑换完自己说已经用了」——这与任务投票是同一套监督逻辑。

**投票口径**（与任务一致）：

| 项 | 规则 |
|---|---|
| 申请人 | 券的持有者本人 |
| 有投票权 | 家庭组全体成员 − 申请人 |
| 通过条件 | 同意票 > 有投票权人数 ÷ 2 |
| 投票期 | 24 小时 |
| 有投票权 = 0 | **直接核销**，不走投票（1 人家庭无法自我监督，也不该卡死） |
| 一人一票 | `UNIQUE(coupon_id, voter_id)`，投后不可改 |

**状态机**：

```
coupons.status:  unused ──使用──> pending ──通过──> redeemed
                    ↑                │
                    └──驳回 / 超时────┘
```

**实现补充（2026-11-03）**：除「我的券」外，另有 `GET /api/app/coupons/pending`
返回本组「待我核销」的券——否则别人兑换的券无人可投，核销机制会空转。
1 人家庭（有投票权 0 人）由服务端直接置 `redeemed`，不走投票。

---

## 10. 积分赠送

### 10.1 语义定义

- **赠送 = 家庭组内成员之间的积分转移**，不是计分行为。
- **家庭总积分守恒**：一方 `-N`，另一方 `+N`，全组积分总和不变。
- **不参与连续打卡**：既不增加打卡次数，也不触发「违反重置」。
- **不可跨家庭组**：受赠方必须与赠送方同组。
- **不可赠送给自己的另一身份**：家庭成员各自独立，禁止 `from == to`。
- **无需受赠方确认，直接到账**：发起即生效，双方积分立即变动，不设「待接受」状态。
- **唯一的金额约束是转出方余额**：不设单笔上限、不设每日上限、不设手续费。
  管理员代赠同样受此约束——**不能凭空生成积分**，只能转移已存在的积分。

### 10.2 功能项

| # | 功能 | 状态 |
|---|---|---|
| 10.2.1 | 成员自助赠送（Android） | ✅ 从自己的积分转出 |
| 10.2.2 | 管理员代任意成员赠送（Web） | ✅ **要做**（2026-11-03 拍板）；属后台功能，受「不能凭空生成积分」约束 |
| 10.2.3 | 余额校验（事务内，防负余额） | ✅ 按 `id` 升序 `FOR UPDATE`，防死锁 |
| 10.2.4 | 附言（可选，≤ 64 字） | ✅ |
| 10.2.5 | 赠送记录查询（按成员/时间） | ✅ 列表；按成员筛选待 P7 |
| 10.2.6 | 撤销赠送 | ⚠️ **暂不做**（2026-11-03 用户明确）；接口与代码已移除，`transfers.revoked_at` 为预留字段 |
| 10.2.7 | 受赠方确认机制 | ✅ **不需要**（19.9 定：发起即到账，无「待接受」状态） |
| 10.2.8 | 单笔 / 每日赠送上限 | ✅ **不设上限**（19.10 定：唯一约束是转出方余额） |

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
| 撤销时**受赠方**余额已不足 | ⚠️ 暂不做撤销；将来若做，回滚要由受赠方退回，余额不足则拒绝 |
| 已撤销的赠送再次撤销 | ⚠️ 同上，将来用 `revoked_at` 非空判断 |

> 「撤销时余额不足」是必须提前想清楚的边界：受赠方收到 10 分后花掉了，再撤销就还不回去。
> ⚠️ 原文写「转出方余额已不足」有误——回滚时**是受赠方要退回**，余额约束在受赠方身上。
> 当前规则是**拒绝撤销**，而不是允许负余额。

## 11. 榜单与记录

### 11.1 两个榜，两种口径

**积分榜与得分榜是两个独立的榜单，回答不同的问题。**

| | 积分榜（总榜） | 得分榜（周期榜） |
|---|---|---|
| 统计什么 | **当前剩余积分** = `score + frozen_score` | **区间内累计得分** = `points > 0` 的记录之和 |
| 本质 | **净额**：收支都已结算完的余额 | **毛额**：只累加「赚到的」，不看「花掉的」 |
| 回答的问题 | 我现在还剩多少分 | 这段时间谁表现好 |
| 时间维度 | 累计至今 | 本周 / 本月，可切换 |
| 兑换、赠送、任务收支的影响 | **已全部反映在余额里**（不再单独加减） | **不计入**（只看赚到的） |
| 冻结积分 | 计入（仍是自己的，只是暂时不能用） | 不计入（冻结不是「得分」） |
| 用途 | 决定能兑换什么 | 决定这周谁最棒 |

**得分榜的计入范围**：

| 记录类型 | 计入得分榜 |
|---|---|
| 睡觉奖励（`+`） | ✅ |
| 任务收入（`+`） | ✅ |
| 收到赠送（`+`） | ✅ |
| 送出赠送（`−`） | ❌ 不是「得分」 |
| 兑换消耗（`−`） | ❌ 不是「得分」 |
| 0 分说明记录 | ❌ |

> **为什么分开**：单一榜单无法同时表达「资产」和「表现」。
> 兑换大件的人余额会掉，但他这周可能最努力——用积分榜看资产、用得分榜看努力，两者不冲突。

### 11.2 功能项

| # | 功能 | 状态 |
|---|---|---|
| 11.2.1 | **积分榜**（按 `score + frozen_score` 降序） | ✅ 返回可用余额与冻结额，客户端自行标注 |
| 11.2.2 | **得分榜**（周 / 月切换，按区间正分之和降序） | ✅ 默认本周（周一起算）；月榜按自然月 |
| 11.2.3 | 连续打卡展示（🔥） | ✅ 榜单每行带 `streak`；最近一条结算行非今天/昨天即视为中断 |
| 11.2.4 | 记录列表服务端分页 | ✅ `limit` / `offset`，默认 50、上限 200 |
| 11.2.5 | 按成员 / 日期范围筛选 | ✅ `member_id` / `from` / `to`（含当天）/ `source` |
| 11.2.6 | 记录搜索 | ✅ `keyword` 对标题模糊匹配 |
| 11.2.7 | 积分趋势图 | ✅ 按天聚合，返回 `earned`（正分和）与 `net`（净变动） |
| 11.2.8 | 数据导出 CSV | ✅ `GET /api/admin/export/records`，带 UTF-8 BOM（Excel 不乱码） |
| 11.2.9 | 记录总量上限 | ✅ **无截断**（MySQL 无此约束，分页读） |

**得分榜查询口径**：

```sql
SELECT member_id, SUM(points) AS period_score
  FROM records
 WHERE family_id = ? AND occurred_at >= ? AND occurred_at < ?
   AND points > 0
 GROUP BY member_id
 ORDER BY period_score DESC;
```

> 注意用 `occurred_at`（事项发生时间）而非 `added_at`（写入时间），
> 否则补记的记录会算到错误的周期里。

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
| 12.2.9 | **采集并上报夜间时段使用分钟数**（`night_min`） | ✅ 服务端已支持；Android 端采集待 P9 |

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

**四条实现要点**（2026-11-03 已实现，见 `server/internal/repo/screen.go` 的 `Settle`）：

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
> **实现（2026-11-03）**：新成员入组时**自动复制组长的屏幕时间规则**，
> 否则他进来第一件事就是「设备上报失败：还没有屏幕时间规则」。之后组长可单独调整。
> 创建家庭组时在**同一事务**里写入一份初始配置
> （规则「按时睡觉」+2、档位 3 天 +5 / 7 天 +10、日总量阈值 120 分钟、夜间 0–7 点允许 0 分钟），
> 之后各组独立修改，互不影响。

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
> **有注册入口**——用户自助注册（见 5.1），**管理员不能代建账号**（决策 24）。

| # | 界面 | 关键内容 | 可见角色 |
|---|---|---|---|
| 13.1 | 启动页 | 检查前台 token，有效则直进主页 | 全部 |
| 13.2 | 登录 / 注册 | 登录；注册创建账号，可顺带创建家庭组（成为组长） | 全部 |
| 13.3 | **积分榜 / 得分榜** | 顶部切换两个榜；积分榜看余额，得分榜看周/月表现 | 全部 |
| 13.4 | 成员详情 | 该成员记录（分页） | 全部 |
| 13.5 | 最近记录 | 分页、按成员筛选 | 全部 |
| 13.6 | 我的兑换 | 商城、余额校验、确认兑换 | 全部 |
| 13.7 | **我的券** | 未使用 / 待核销 / 已核销三态；**点击使用**发起核销投票 | 全部 |
| 13.8 | 赠送积分 | 选受赠成员、金额与附言、赠送历史 | 全部 |
| 13.9 | **任务** | 发布任务（分值、截止时间）、**接取**、放弃接取、提交、我的任务 | 全部 |
| 13.10 | **任务投票** | 待投票列表、同意/反对、实时票数（X/Y） | 全部（**发布者除外**） |
| 13.11 | 通知中心 | 未读列表、点击跳转、标记已读 | 全部 |
| 13.12 | 屏幕时间状态 | 权限状态、今日时长、上报状态 | 全部 |
| 13.13 | 设置 | 账号、家庭组信息、退出、改密码、头像（**预设**） | 全部 |
| 13.14 | **成员管理** | 邀请已注册用户加入、踢出成员、待处理邀请、**生成 / 重置邀请码** | **仅组长** |
| 13.15 | **睡觉规则配置** | 睡觉时间点、允许时长、连续奖励档位 | **仅组长** |
| 13.16 | **商品管理** | 商品增删改 | **仅组长** |

**共 16 个界面**，其中 3 个（13.14–13.16）仅组长可见。
> 未来升级还会加 2 个界面（全服任务、家庭星榜，见 25 节），当前**不做**。

> **组长的管理能力放在前台**，不需要登录 Web 后台——这是本次权限重划的核心：
> 组长管「家庭内的事务」（成员、睡觉规则、商品），管理员管「系统与安全」（设备、审计、家庭组）。

> **原型（2026-11-03）**：15 个界面的高保真原型见
> [`design/android-prototype.html`](../design/android-prototype.html)，
> 可直接在浏览器打开、点击任一屏幕放大。原型里的字段与本节、第 3 节表结构一一对应，
> **不是自由发挥的视觉稿**——若原型与本文档冲突，以本文档为准并修原型。

---

## 14. Web 管理后台

> **仅管理员使用**（`admins` 账号）。不提供成员只读视图——成员看数据请用 Android。

| # | 页面 | 对应功能 |
|---|---|---|
| 14.1 | 登录 | **仅登录，无注册入口**；管理员由部署时环境变量初始化（见 5.4） |
| 14.2 | 成员管理 | 查看成员、隐藏、设组长、头像（**仅预设**）、**邀请已注册用户** |
| 14.3 | 睡觉规则 | 睡觉时间点、允许时长、连续奖励档位（与组长同权） |
| 14.4 | 任务管理 | 查看全部任务与提交、代审核 |
| 14.5 | 兑换商城 | 商品管理、兑换撤销 |
| 14.6 | 赠送管理 | 赠送记录、撤销赠送 |
| 14.7 | 记录 | **只读**：分页、筛选、搜索、导出 CSV（**无删除**） |
| 14.8 | 统计 | 积分榜、得分榜（周/月）、积分趋势图 |
| 14.9 | 审计日志 | 按成员/动作/时间筛选，查看 before/after |
| 14.10 | 屏幕时间 | 查看上报历史（**只读，无手动录入**） |
| 14.11 | 设备管理 | 列表、生成令牌、吊销 |
| 14.12 | 家庭组设置 | 组名、移交管理员 |

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
| 15.1.1 | `/api/register` | POST | 注册账号；可传 `createFamily` 顺带创建家庭组（**只成为组长，不产生后台身份**） |
| 15.1.2 | `/api/admin/login` | POST | 后台登录 |
| 15.1.3 | `/api/app/login` | POST | 前台登录 |
| 15.1.4 | `/api/admin/refresh` | POST | 后台 token 刷新 |
| 15.1.5 | `/api/app/refresh` | POST | 前台 token 刷新 |
| 15.1.6 | `/api/app/family/join` | POST | 凭邀请码加入家庭组（需前台 token，`family_id` 为空） |

### 15.2 后台接口（`typ='admin'`）

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.2.1 | `/api/admin/password` | POST | 改密码 |
| 15.2.2 | `/api/admin/family` | GET/POST | 查看 / 改名 |
| 15.2.3 | `/api/admin/family/:family_id/invite` | POST | 按用户名把已注册用户拉进该组 |
| 15.2.4 | `/api/admin/family/transfer-admin` | POST | 移交管理员 |
| 15.2.5 | `/api/admin/family/:family_id/member/kick` | POST | 踢出成员（组长不可直接踢，须先换组长） |
| 15.2.22 | `/api/admin/family/:family_id/member/hidden` | POST | 隐藏 / 显示成员 |
| 15.2.23 | `/api/admin/family/:family_id/member/leader` | POST | 后台指定新组长（同事务降旧升新） |
| 15.2.24 | `/api/admin/family/:family_id/rename` | POST | 改家庭组名 |
| 15.2.6 | `/api/admin/family/:family_id/rule/save` | POST | 睡觉规则与连续奖励档位（与组长同权） |
| 15.2.7 | `/api/admin/family/:family_id/item/save` | POST | 商品增改（与组长同权；撤销兑换已按 9.4 移除） |
| 15.2.8 | `/api/admin/task/*` | POST | 查看全部、代审核 |
| 15.2.9 | `/api/admin/audit/list` | GET | 审计日志查询 |
| 15.2.10 | `/api/admin/export/records` | GET | 导出 CSV |
| 15.2.11 | `/api/admin/device/*` | POST | 设备生成 / 吊销 |
| 15.2.12 | `/api/admin/state` | GET | 后台初始化数据 |
| 15.2.13 | `/api/admin/accounts` · `/api/admin/accounts/disable` | GET/POST | 管理员账号列表 / 新增 / 停用（不能停用自己） |
| 15.2.14 | `/api/admin/families` | GET | 所有家庭组列表（运营方视角） |
| 15.2.15 | `/api/admin/family/:family_id` | GET | 跨组查看单个家庭组（**family_id 只能走路径参数**） |
| 15.2.16 | `/api/admin/family/:family_id/members` | GET | 跨组查看成员（含被隐藏的） |
| 15.2.19 | `/api/admin/me` | GET | 当前管理员信息（含 `must_change_password`） |
| 15.2.20 | `/api/admin/family/:family_id/device` | POST/GET | 生成设备令牌（明文只返回一次）/ 列出设备 |
| 15.2.21 | `/api/admin/family/:family_id/device/revoke` | POST | 吊销设备令牌（吊销后上报立即 401） |

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
| 15.3.5 | `/api/app/item/redeem` | POST | 自助兑换（事务内扣分 + 写负分流水 + **生成商品券**） |
| 15.3.6 | `/api/app/coupons` | GET | **我的券**列表（含状态与驳回次数） |
| 15.3.7 | `/api/app/coupon/use` | POST | **使用券**：发起核销，进入投票（`unused → pending`） |
| 15.3.8 | `/api/app/coupon/vote` | POST | **核销投票**（同意/反对），申请人无投票权 |
| 15.3.9 | `/api/app/transfer/send` | POST | 赠送积分 |
| 15.3.10 | `/api/app/transfer/list` | GET | 赠送历史 |
| 15.3.11 | `/api/app/task/list` | GET | 任务列表 |
| 15.3.12 | `/api/app/task/create` | POST | **发布任务**（需余额充足，冻结积分；不指定完成人） |
| 15.3.13 | `/api/app/task/claim` | POST | **接取任务**（除发布者外任何成员，先到先得） |
| 15.3.14 | `/api/app/task/abandon` | POST | **放弃接取**（仅接取者，任务回到 open） |
| 15.3.15 | `/api/app/task/close` | POST | 关闭自己的任务（退回冻结） |
| 15.3.16 | `/api/app/task/submit` | POST | 提交完成（**仅接取者本人**） |
| 15.3.17 | `/api/app/task/vote` | POST | **投票**（同意/反对），发布者无投票权 |
| 15.3.18 | `/api/app/notification/list` | GET | 通知列表 |
| 15.3.19 | `/api/app/notification/read` | POST | 标记已读 |
| 15.3.20 | `/api/app/stats/ranking` | GET | `type=points`（积分榜）或 `type=score`（得分榜，带 `period=week\|month`） |
| 15.3.21 | `/api/app/stats/trend` | GET | 趋势图 |
| 15.3.22 | `/api/app/member/avatar` | POST | 改自己的头像（**只接受预设标识**，见 22.2） |
| 15.3.23 | `/api/app/family/leave` | POST | 自行退出家庭组（积分清零 + 写入 `family_bans`） |
| 15.3.24 | `/api/app/family` | GET | 本组信息（**family_id 取自 token**，不接受任何参数） |
| 15.3.25 | `/api/app/family/members` | GET | 本组成员（过滤被隐藏的） |
| 15.3.26 | `/api/app/family/rename` | POST | 改家庭组名（**组长**） |
| 15.3.27 | `/api/app/member/rename` | POST | 改昵称（本人或**组长**） |
| 15.3.28 | `/api/app/records` | GET | 本组流水分页（`limit` / `offset`） |
| 15.3.29 | `/api/app/me` | GET | 当前成员信息（`score` / `frozen_score` / `role` / `family_id`） |
| 15.3.30 | `/api/app/screen/reports` | GET | 结算历史（本人；组长可查他人） |
| 15.3.31 | `/api/app/task/detail` | GET | 任务详情：提交列表 + 每条的票数与票 |

**仅组长（`role='leader'`）**

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.3.32 | `/api/app/member/invite` | POST | 邀请已注册用户加入（按用户名） |
| 15.3.33 | `/api/app/member/remove` | POST | 踢出成员（积分清零 + 写入 `family_bans`；**需 `confirm=true`**） |
| 15.3.34 | `/api/app/family/invite-code` | GET/POST | 查看 / 重置邀请码 |
| 15.3.35 | `/api/app/rule` · `/api/app/rule/save` | GET/POST | 睡觉规则与连续奖励档位（保存限**组长**） |
| 15.3.36 | `/api/app/item/save` · `/api/app/item/delete` | POST | 商品增改 / 删除（**组长**） |
| 15.3.37 | `/api/app/member/hidden` | POST | 隐藏 / 显示成员 |
| 15.3.38 | `/api/app/family/transfer-leader` | POST | **组长转让**（同事务降旧升新） |
| 15.3.39 | `/api/app/screen-rule` · `/api/app/screen-rule/save` | GET/POST | 每成员屏幕时间规则（保存限**组长**） |

### 15.4 设备接口（设备令牌）

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.4.1 | `/api/device/screen/report` | POST | 上报当天累计时长（`total_min`）与夜间时长（`night_min`）；**后报覆盖**，`report_date` 缺省按归属日推算 |

> **越权测试基线**：前台 token 打任一 `/api/admin/*` 必须 401；
> 后台 token 打任一 `/api/app/*` 必须 401；
> 普通成员打 15.3.21–15.3.25（组长专属）必须 403；
> 发布者调用投票接口必须 403；非本组成员投票必须 404。

---

## 16. 部署方案（自建服务器）——**开发阶段不涉及，上线前再执行**

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
| 16.2.9 | **初始化默认管理员账号** | ⬜ 账号与初始密码从环境变量读取（`ADMIN_USER` / `ADMIN_PASS`） |
| 16.2.10 | **首次登录强制改密** | ⬜ 未改密前，除改密接口外所有后台接口返回 403 |

### 16.3 备份（**自建方案必须项**）

| # | 事项 | 状态 |
|---|---|---|
| 16.3.1 | `mysqldump` 每日定时备份 | ⬜ |
| 16.3.2 | 备份保留 **30 日**滚动（2026-11-03 由 7 日上调） | ⬜ 与日志 180 天口径的差距仍有，但 30 天覆盖「月底才发现误删」的典型场景 |
| 16.3.3 | **异地副本**（对象存储 / 另一台机器） | ⬜ 同机备份无法应对磁盘故障 |
| 16.3.4 | 恢复演练（至少做一次） | ⬜ 没演练过的备份等于没有备份 |

> ⚠️ 从 Cloudflare 托管转为自建，**数据安全责任完全转移到你自己**。Cloudflare 时代不需要操心的事，现在全要操心。

### 16.4 迁移：**不做**（19.8 定：一切从 0 开始）

> **2026-11-03 拍板**：**不迁移任何旧 KV 数据**，v3 是全新起点。
> ⇒ **不需要**导出脚本、导入脚本、数据比对、双写窗口、回滚预案 —— 整节工作量为零。

| # | 事项 | 处理 |
|---|---|---|
| 16.4.1 | 旧 KV `state` 数据 | **弃置不用**。Cloudflare 上原样留着即可（不删、不读），停服后自然失效 |
| 16.4.2 | 旧账号 / 旧密码 | **不存在**——没有账号被迁过来，所有人都重新注册 |
| 16.4.3 | 旧积分 / 旧记录 | **不继承**，新库从空表开始 |

> ⚠️ **由此产生的一条硬约束**：v3 **不需要任何「兼容旧数据」的分支**——
> 不用为旧 SHA-256 密码保留校验逻辑（见 17 节），不用为老字段做默认值兜底，
> 不用为「历史记录只有 500 条」做补偿。凡是为了迁就 v1 数据而加的代码，都是多余的。

---

## 17. 与 v1 的行为差异（**仅供对照，不涉及迁移**）

| 项 | v1（KV） | v3（MySQL） |
|---|---|---|
| 记录上限 | 500 条硬截断 | 无上限 |
| 并发写 | 单键覆盖风险 | 事务 + 行锁，需防超兑 |
| 密码 | SHA-256 加盐 | **bcrypt**（无旧哈希需兼容，见下） |
| Token | 30 天固定，无刷新 | Access 2h + Refresh 30d |
| 时间字段 | 毫秒时间戳 | `DATETIME(3)` |

> **密码无需迁移**（19.8 定：不迁旧数据）：既然没有旧账号被搬过来，
> **不要**为 SHA-256 写惰性改写分支、不要保留双算法校验——
> 注册即 bcrypt，登录只认 bcrypt。这是「从 0 开始」最直接的一处收益。

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

### 19.A 挂起：部署阶段再定（**不阻塞开发**）

> **2026-11-03 拍板：先开发，不管部署。** 以下三条统一**挂起**，
> 等 v3 功能跑通、准备上线时再定。开发期按「默认口径」实现，**留好挂载点即可**。

| # | 事项 | 挂起后开发期怎么办 |
|---|---|---|
| 19.5 | 服务器规格与操作系统 | 只影响 `Dockerfile` 基础镜像与 `compose` 资源限制，**后补**。开发期用本机或 Docker 起一个 MySQL 8 即可 |
| 19.6 | 是否公开注册 | **先按「开放注册」实现**（注册语义见 5.1）。限流与验证码**不实现**，但路由层要留中间件挂载点，日后加一行即可 |
| 19.7 | 域名与备案 | 与代码无关。开发期一律 `localhost`；HTTPS / 证书 / 备案全部延后 |

> **判据**：挂起项**不得**成为「这个功能先不做」的理由。
> 若某功能确实依赖它，就在本文档标注「部署阶段补」，**不要静默跳过**。

### 19.B 已关闭的决策

> - **19.1（Web 是否保留成员只读视图）** —— 2026-11-02 拍板为「**不保留**」：
>   Web 仅做管理后台，成员看数据请用 Android。详见 4.1 与 14 节。
>   （原「Vue 前端可复用率 60% → 35%」的估算已作废：v1 代码已于 2026-11-03 删除，
>   Web 后台按新方案重写，无既有前端可复用。）
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
> - **19.12（周期排行榜口径）** —— 2026-11-02 拍板：**拆成两个榜**，
>   积分榜只统计剩余积分、得分榜只统计区间得分。详见 11.1。
> - **19.9（赠送是否需受赠方确认）** —— 2026-11-02 拍板：**不需要**，直接到账。
> - **19.21（投票后能否改票）** —— 2026-11-02 拍板：**不能改**，一人一票定终身。
> - **19.19（是否允许多个后台管理员）** —— 2026-11-02 拍板：**允许**。
>   系统自带默认账号，运营方可在后台新增/停用其他管理员。详见 5.4。
> - **19.22（邀请码有效期）** —— 2026-11-02 拍板：**7 天**，过期需组长重置。
> - **19.20（创建者以外的人如何获得后台账号）** —— **不成立**：
>   家庭用户（含组长）永远没有后台账号，管理员由部署方单独创建。详见 5.4。
> - **19.2（退出 / 踢出家庭组时记录归属）** —— 2026-11-02 拍板：
>   **积分清零**（`score` 与 `frozen_score` 归零），**历史记录保留在原组**，
>   且**禁止再次加入该家庭组**（写入 `family_bans`）。详见 5.3。
> - **19.16 / 19.17（手动上传凭证与权限）** —— **随功能一并作废**：
>   手动补录已整体移除，相关决策不再需要。
> - **19.8（旧 KV 数据是否迁移）** —— 2026-11-03 拍板：**不迁移，一切从 0 开始**。
>   ⇒ 无迁移脚本、无双写窗口、无回滚预案；旧账号/旧积分/旧记录全部不继承。
>   连带取消：bcrypt 的 SHA-256 惰性兼容分支（见 17 节）、任何为迁就 v1 数据写的兜底代码。
>   详见 16.4。
> - **19.11（任务提交是否强制图片凭证）** —— 2026-11-03 拍板：**暂不做图片凭证**，
>   提交只写**文字说明**。理由：做图片就要新增上传接口 + 存储目录 + 格式/体积校验 +
>   纳入备份，收益（投票人多一个参考）远小于成本；审核本就靠投票，文字说明足够。
>   ⇒ `task_submissions.image_url` **字段一并删除**，不是留着不用。
>   日后若确要做，必须同时补：上传接口、存储方案（本地磁盘或对象存储）、
>   格式与 ≤2MB 校验、服务端二次校验、目录纳入备份。
> - **19.13（审计日志保留期）** —— 2026-11-03 拍板：**180 天**，配套每日定时清理
>   （分批删除，避免长事务）。详见 21.3.4。
> - **19.14（头像存储方式）** —— 2026-11-03 拍板：**仅预设头像，不做自定义上传**。
>   `avatar` 只存预设标识（如 `'bear'`），**服务器上不落任何头像文件**，
>   ⇒ 无头像目录、无备份项、无图片校验。原「本地磁盘 vs 对象存储」的取舍随之作废。
>   详见 22.2。
> - **19.23（商品券有效期）** —— 2026-11-03 拍板：**长期有效**，不设过期。
>   券一经发出永久可用（除非商品本身下架——但券存的是快照，不受影响）。
>   ⇒ 无过期定时任务、无「过期退分」逻辑，避免多一条积分变动路径。
> - **19.15（任务驳回后重提上限）** —— 2026-11-03 拍板：**3 次**。
>   计次口径 = 同一 `task_id` + 同一 `member_id` 的 `rejected` 提交数；
>   达上限后该成员不能再对同一任务提交。详见 20.3、20.5.15。

---

## 20. 任务（悬赏与确认）

### 20.1 目的与模型

任务不再是「家长单向发分」，而是**成员之间的积分转移**：

```
发布者冻结 N 分 → 任务 open → 有人提交 → 审核通过 → N 分从发布者转给完成者
                                              └─ 驳回 → 可重新提交（同一任务最多 3 次）
                    └─ 关闭 / 无人完成 → N 分退回发布者
```

**任何成员都可以发布任务**（含组长与普通成员），只要余额足够。
**管理员不能发布**——管理员不参与积分，没有可冻结的余额。

### 20.2 接取规则（公开认领）

**任务发布后，除发布者外任何成员都可以接取。** 不支持指定完成人。

| 项 | 规则 |
|---|---|
| 可接取者 | **家庭组全体成员 − 发布者** |
| 接取方式 | 主动点击「接取」，先到先得 |
| 已被接取 | 其他人**不能再接**（`assignee_id` 非空即锁定） |
| 放弃接取 | 接取者可放弃，任务回到 `open` 状态 |
| 提交完成 | **仅接取者本人**可提交 |
| 发布者取消 | 可随时关闭任务，冻结积分退回 |

**任务状态机**：

```
tasks.status:  open ──接取──> taken ──提交──> (等待投票)
                 ↑              │
                 └──放弃────────┘
                 └──发布者关闭──> closed
```

**提交状态机**：

```
task_submissions.status: submitted → approved / rejected / expired
```

> **为什么不允许指定完成人**：指定完成人会绕过「公开认领」的公平性，
> 也容易变成变相发分。公开接取让每个成员都有机会，且接取行为本身可被审计。

### 20.3 审核规则：全员投票，过半同意

**审核不是单人决定，而是投票制。**

| 项 | 规则 |
|---|---|
| 有投票权 | **家庭组成员 − 发布者**（完成者**可以**自投） |
| 通过条件 | **同意票 > 有投票权人数 ÷ 2**（严格过半） |
| 提前驳回 | 反对票达到「同意已不可能过半」时立即结束 |
| 投票期 | 提交后 **24 小时**，或任务截止时间（取较早者） |
| 超时未过半 | 状态置 `expired`，**不转移积分**，完成者可重新提交 |
| 一人一票 | `UNIQUE(submission_id, voter_id)` 保证 |
| 驳回后重提 | 同一任务最多 **3 次**（`rejected` 计数达 3 即锁定），`expired` **不计入**该上限 |
| 提交凭证 | **仅文字说明，且必填**（19.11 定：无图片凭证、无上传接口；空说明无凭证意义） |

**过半判定示例**（完成者计入投票人数，可自投）：

| 有账号成员数 | 有投票权（除发布者） | 通过所需同意票 | 说明 |
|---|---|---|---|
| 5 | 4 | **3**（> 2） | 完成者自投后还需 2 人 |
| 4 | 3 | **2**（> 1.5） | 完成者自投后还需 1 人 |
| 3 | 2 | **2**（> 1） | 完成者自投后还需 1 人 |
| 2 | 1 | **1**（> 0.5） | 完成者自投即通过 |
| 1 | 0 | — | **无法过半，禁止发布任务** |

> 所有成员都有账号，**分母就是家庭组成员数减 1**，不存在例外。

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
| 20.5.1 | **发布任务**（标题、悬赏分值、截止时间） | ✅ 任何成员可发，需余额充足；**不指定完成人** |
| 20.5.2 | **发布时冻结积分**（`score → frozen_score`） | ✅ 保证「悬赏必有兑付能力」 |
| 20.5.3 | **任务列表**（待接取 / 我已接取 / 已完成） | ✅ 除发布者外全员可见待接取任务 |
| 20.5.4 | **接取任务**（先到先得，锁定 `assignee_id`） | ✅ 条件更新保证并发下只有一个成功 |
| 20.5.5 | **放弃接取**（回到 `open`） | ✅ 仅接取者本人可放弃 |
| 20.5.6 | 提交完成（**附文字说明**） | ✅ **仅接取者本人**可提交；说明必填（无图片凭证，19.11 定） |
| 20.5.7 | **待投票列表**（除发布者外全员可见） | ✅ 任务详情返回提交与票数 |
| 20.5.8 | **投票（同意 / 反对）** | ✅ 一人一票，**提交后不可更改**（唯一索引兜底） |
| 20.5.9 | 实时票数展示（已同意 X / 需要 Y） | ✅ 投票接口直接返回票数与分母 |
| 20.5.10 | **达成过半 → 自动通过并转移积分** | ✅ 同事务：释放冻结 + 加给完成者 + 两条流水 + 关闭任务 |
| 20.5.11 | 反对票足以否决 → 自动驳回 | ✅ 判据：同意 + 剩余未投 ≤ 半数 |
| 20.5.12 | 超时未过半 → `expired`，冻结不动、任务回到待接取 | ✅ 惰性清扫（读路径上把过期提交置 expired） |
| 20.5.13 | 关闭任务 → 冻结积分退回发布者 | ✅ frozen → score，总资产不变 |
| 20.5.14 | 重复任务（每日/每周） | ⛔ 暂不实现，`repeat_rule` 预留 |
| 20.5.15 | **驳回后重提上限（3 次）** | ✅ `rejected` 数达 3 即锁定；`expired` 不计入 |

### 20.6 结算事务（2026-11-03 已实现，见 `server/internal/repo/task.go`）

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
| **发布者接取自己的任务** | **拒绝**（自己给自己悬赏无意义） |
| 任务已被他人接取 | 拒绝接取（`assignee_id` 非空） |
| 任务状态非 `open` | 拒绝接取 |
| 非接取者提交完成 | 拒绝（只有 `assignee_id` 本人能提交） |
| 接取者放弃后又想接回 | 允许（任务已回到 `open`） |
| **发布者参与投票** | **禁止**（发布者无投票权） |
| **完成者自投** | **允许**（计入同意票） |
| 同一人重复投票 | 拒绝（`UNIQUE(submission_id, voter_id)`） |
| 投票后再改票 | **不允许**，一人一票定终身（`UNIQUE(submission_id, voter_id)` + 拒绝二次写入） |
| 任务已关闭后提交 | 拒绝 |
| 同一任务重复提交 | 拒绝（已有 `submitted`） |
| **驳回后重提超过 3 次** | 拒绝（同一 `task_id` + `member_id` 的 `rejected` 数 ≥ 3 即锁定） |
| 提交时试图附图片 | **不支持**（无上传接口、无 `image_url` 字段）；需要举证时用文字描述 |
| 提交说明为空 | **拒绝**（无图片时说明是唯一凭证，不能空） |
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
| 成员邀请 / 踢出 | `member.invite` / `member.remove` | 是 |
| 规则增删改 | `rule.*` | 是 |
| 兑换 / 撤销兑换 | `item.redeem` / `item.revoke` | 是 |
| 积分赠送 / 撤销 | `transfer.send` / `transfer.revoke` | 是 |
| 任务审核 | `task.review` | 是 |
| 设备生成 / 吊销 | `device.*` | 是 |
| 登录 / 修改密码 | `auth.login` / `auth.password` | 否（仅记录事件） |

> **实现命名（2026-11-03）**：实际写入的 action 与上表略有出入，以代码为准：
> `item.save` / `item.delete` / `item.redeem` / `transfer.send` / `task.create` /
> `task.submit` / `task.vote` / `task.close` / `member.leave` / `member.kick` /
> `member.hidden` / `leader.transfer` / `family.rename` / `rule.save` /
> `screen_rule.save` / `device.create` / `device.revoke`。
> **未记**：登录、改密码（原计划「仅记录事件」，目前未落库）。

### 21.3 功能项

| # | 功能 | 状态 |
|---|---|---|
| 21.3.1 | 全量写入审计日志 | ✅ 已接入：兑换/赠送/任务(发布·提交·投票·关闭)/成员(踢出·退出·隐藏·转让)/规则/商品/设备/改组名 |
| 21.3.2 | Web 后台查询（按成员 / 动作 / 时间筛选） | ✅ `GET /api/admin/audit/list`，另支持 family_id 与分页 |
| 21.3.3 | 单条记录查看 before/after 差异 | ✅ 接口直接返回 `before` / `after` JSON（已脱敏） |
| 21.3.4 | 日志保留期与清理（**180 天**，每日分批清理） | ⚠️ 未做：需要定时任务，属部署阶段（19.13 已定 180 天） |
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
| 22.1.1 | 通知生成（任务待审 / 审核结果 / 兑换成功 / 收到赠送） | ✅ 已实现：收到赠送、审核结果；待审提醒与兑换成功待补 |
| 22.1.2 | 未读数角标 | ✅ 由 `/api/app/state` 的 `unread_count` 提供 |
| 22.1.3 | 通知列表与标记已读 | ✅ 列表（含 `unread=1` 过滤）、单条/全部已读 |
| 22.1.4 | 点击跳转到对应对象 | ✅ 通知带 `ref_type` / `ref_id`，跳转由客户端实现 |
| 22.1.5 | 系统推送（FCM / 厂商通道） | ⛔ 明确不做：国内需接多家通道，成本远高于收益 |

> **不做推送的取舍**：Android 端靠「打开 App 时拉取未读」即可满足家庭场景。
> 若日后确有实时性需求，再评估厂商通道。

### 22.2 成员头像

| # | 功能 | 状态 |
|---|---|---|
| 22.2.1 | 预设头像选择（内置一组，服务端只存标识） | ⬜ |
| 22.2.2 | 自定义上传 | ⛔ 明确不做（19.14 定：仅预设头像） |
| 22.2.3 | 图片校验（格式、尺寸 ≤ 2 MB、服务端二次校验） | ⛔ 随 22.2.2 作废：无上传即无校验 |
| 22.2.4 | 排行榜与记录列表展示头像 | ⬜ |

> **头像存储决策（19.14，2026-11-03）**：**只做预设头像**，`members.avatar` 存的是
> 预设标识（如 `'bear'`），前端按标识映射到内置资源。
> ⇒ **服务器上不落任何头像文件**：没有上传目录、没有静态托管、不进备份范围。
> 原「本地磁盘 + Nginx 托管 / 对象存储」的方案随自定义上传一并作废。
> 若日后确要放开自定义上传，必须同时补：图片二次校验、目录纳入备份、体积上限。

---

## 23. 验收要点

> **开发阶段验收范围**：本节的 **1–86、92–101** 条都要在开发阶段跑通；
> **87–91（部署组）在部署时验证**，开发阶段不阻塞交付。

**多租户隔离（最高优先级）**
1. A 组 admin 无法通过任何接口读写 B 组数据（逐接口测）。
2. 篡改请求体 / query 中的 `family_id` 无效——服务端只认 JWT。
3. 一个 username 无法同时存在于两个家庭组。
4. A 组修改睡觉时间点、连续奖励档位后，B 组的配置与结算结果完全不受影响。
5. 配置接口不接受「全局默认值」参数——任何配置写入都必须落到具体 `family_id`。

**账号与权限隔离**
6. 前台 token 调用任一 `/api/admin/*` 返回 **401**。
7. 后台 token 调用任一 `/api/app/*` 返回 **401**。
8. 普通成员调用组长专属接口（邀请成员、踢出成员、睡觉规则、商品管理）返回 **403**。
9. 管理员**不出现在**成员列表、排行榜、记录中。
10. 管理员调用「发布任务」被拒绝——管理员不参与积分，无冻结来源。
11. **发布者调用投票接口**返回 403（发布者无投票权）。
12. **发布者与完成者相同**的任务被拒绝。
13. 变更组长后，旧组长**立即失去**组长接口权限（不依赖重新登录）。
14. 变更组长后，**组内仍只有一名** `leader`。
15. **创建家庭组的用户只获得组长身份**，用其账号登录后台返回 401（家庭用户无后台账号）。
16. 后台管理员账号无法通过注册页创建（`/api/register` 不产生 `admins` 记录）。
17. 管理员 token 可跨组查询，但每次跨组操作**必须显式传 `family_id` 且写入审计日志**。
18. **默认管理员账号**由部署初始化产生，可用环境变量中的初始密码登录。
19. 默认管理员**首次登录未改密时**，除改密接口外所有后台接口返回 403。
20. 运营方可在后台新增 / 停用管理员账号；停用后该账号立即无法登录。
21. 家庭用户的账号（含组长）**无法通过任何接口获得后台权限**。
22. **邀请码超过 7 天后失效**，凭过期码加入返回 400。
23. 组长重置邀请码后，**旧码立即失效**。

**退出与踢出（19.2 已定）**
24. 成员被踢出后 `score` 与 `frozen_score` 均为 **0**。
25. 成员自行退出后 `score` 与 `frozen_score` 均为 **0**。
26. 被踢出 / 退出的成员**再次用同一邀请码或组长邀请加入该组**时被拒绝。
27. 被踢出 / 退出的成员**可以加入其他家庭组**（禁入只针对原组）。
28. 踢出时该成员有未结算任务 → 任务自动关闭，冻结积分**不退回**（余额已清零）。
29. 踢出后其历史记录**仍保留在原组**，不删除。

**积分不变量（核心账目）**
30. 任务通过前后，**家庭总积分不变**（发布者 -N，完成者 +N）。
31. 赠送前后，**家庭总积分不变**。
32. 睡觉奖励使家庭总积分**增加**。
33. 兑换使家庭总积分**减少**。
34. `frozen_score` **不计入可用余额**——冻结中的积分不能兑换、不能赠送、不能再发布任务。

**任务经济**
35. 发布任务时可用余额不足 → 拒绝，且不产生任务记录。
36. 发布成功后：发布者 `score -= N`、`frozen_score += N`。
37. 关闭任务后：发布者 `frozen_score -= N`、`score += N`（退回）。
38. 审核通过后：发布者 `frozen_score -= N`，完成者 `score += N`。
39. 同一提交被并发投票触发两次通过，只转移一次积分（条件更新受影响行数为 0 即返回成功）。
40. 悬赏分值 ≤ 0 时拒绝创建。

**两个榜单（19.12 已定）**
41. 积分榜按 `score + frozen_score` 降序；显示可用余额，有冻结时标注。
42. 得分榜只统计 `points > 0` 的记录，按 `occurred_at` 落在区间内聚合。
43. 兑换消耗**不影响**得分榜排名。
44. 送出赠送**不计入**得分榜（只有收到赠送计入）。
45. 补记的旧日期记录，落在其 `occurred_at` 所属周期，**不是写入时间所在周期**。
46. 同一成员在两个榜上的名次可以不同（资产高但本周没表现）。
47. **兑换后积分榜立即下降、得分榜不变**——两榜口径差异的最直观验证。

**商品券与核销（9.6）**
48. 兑换成功后**必然生成一张券**，状态 `unused`，`item_name`/`cost` 为快照。
49. 商品被删除或改名后，已发出的券**不受影响**。
50. 点击使用 → 状态 `pending`，写入 `vote_deadline`（+24h）。
51. **申请人没有投票权**，调用投票接口返回 403。
52. 同意票过半 → `redeemed`，写入 `settled_at`；券不可再用。
53. 驳回或超时 → 回到 `unused`，`reject_count + 1`，可再次发起。
54. **同一张券同时只能有一个 `pending`**，重复使用返回 409。
55. **有投票权为 0（1 人家庭）→ 直接 `redeemed`**，不进入投票。
56. 已核销的券再次使用返回 409。
57. 券的核销**不产生任何积分变动**（分在兑换时已扣）。

**任务接取（公开认领）**
58. 除发布者外任何成员都能看到并接取 `open` 状态的任务。
59. **发布者接取自己的任务**返回 403。
60. 任务被 A 接取后，B 再调用接取接口返回 409，且 `assignee_id` 仍为 A。
61. **非接取者提交完成**返回 403。
62. 接取者放弃后任务回到 `open`，其他成员可接取。
63. 接取者放弃后**可以再接回**（任务已回到 `open`）。

**任务投票（过半同意）**
64. 5 人家庭（发布者除外 4 人）：3 票同意即通过；2 票同意不通过。
65. 4 人家庭（发布者除外 3 人）：2 票同意即通过。
66. 3 人家庭（发布者除外 2 人）：需 2 票全票同意才通过。
67. **2 人家庭**：完成者自投 1 票即 1/1 过半，**直接通过**（不走降级）。
68. **有账号成员仅 1 人**时禁止发布任务（除发布者外无人可投）。
69. **投票分母 = 家庭组成员数 − 1**，与成员是否有其他角色无关。
70. 同一人重复投票被拒绝（`UNIQUE(submission_id, voter_id)`）。
71. 反对票达到「同意已不可能过半」时立即驳回，不必等满 24 小时。
72. 投票超时未过半 → 状态 `expired`，**积分不转移**，冻结仍归发布者。
73. 投票期内成员被踢出，其票作废并**重新计算过半线**。

**睡觉规则**
74. 配置「0 点前放下手机」（0:00–7:00、允许 0 分钟）：夜间使用 0 分钟 → 记 +N，连续 +1。
75. 夜间使用 45 分钟 → 记 0 分说明记录，连续天数清零。
76. 当日未上报 → 视为未达标，连续天数清零。
77. 0:00–07:00 的数据归属**前一天**，不归到当天。
78. 一次上报同时触发日总量与夜间两条记录；覆盖时两条都被正确回滚，无孤儿记录。
79. `repeat` 档在第 6、9 天再次触发，`once` 档只触发一次。
80. 修改睡觉时间点或奖励档位后，历史已结算日期**不追溯重算**。
81. 配置 `every = 0` 或 `bonus = 0` 时保存被拒绝；同组内 `every` 重复时保存被拒绝。
82. 多档同时命中时（如第 21 天既满 3 又满 7）两档都发放。

**已移除功能必须真的不存在**
83. `/api/admin/score/*`（记一笔）返回 **404**，不是 403 或静默成功。
84. 记录接口**不提供删除方法**（DELETE 返回 405）。
85. 不存在手动补录使用时长的接口（404）。
86. 后台与前台界面均**不出现**「记一笔」「手动录入」「删除记录」入口。

**并发与事务**
87. 同一成员并发兑换 10 次，余额不出现负数。
88. 同一份屏幕时间重复上报 10 次，积分只变一次。
89. A 与 B 并发互赠，不出现死锁（按 id 升序加锁）。
90. 撤销赠送后，双方积分与记录均正确回滚；转出方余额不足时撤销被拒绝。
91. **赠送直接到账**：发起后受赠方积分立即增加，不存在「待接受」状态。
92. **投票提交后不可更改**：同一人对同一提交第二次投票返回 409。
93. 事务中途失败时，积分、记录、结算行三者不会出现半更新状态。

**审计日志**
94. 能查到每一次积分变动、任务审核、赠送、配置修改的 before/after。
95. 审计日志中**不含任何 `pass_hash`**。
96. 审计日志写入失败时，主流程仍然成功。

**部署**
97. 服务器重启后所有服务自动拉起。
98. 备份文件能在**空库**上完整恢复（真做一次）。
99. **头像无文件落盘**：恢复后头像全部正常显示（因只存预设标识，无需恢复任何目录）。
100. HTTPS 证书自动续期正常。
101. MySQL 与 Go 端口不对外暴露。

**Android**
102. 未授权 `PACKAGE_USAGE_STATS` 时给出引导，不崩溃。
103. 杀后台后打开 App 能补齐遗漏日期。
104. 设备令牌吊销后立即 401。

**2026-11-03 四项决策（19.11 / 19.13 / 19.14 / 19.15）**
105. **任务提交只有文字说明且必填**：请求体不接收图片字段；`task_submissions` 表无 `image_url` 列；
    `note` 为空时返回 400。
106. 同一任务同一成员第 3 次被驳回后**仍可提交一次**，第 4 次提交返回 403 并提示已达上限。
107. `expired` 的提交**不计入**重提上限（超时后仍可再提交）。
108. 审计日志中早于 180 天的记录被定时任务删除，边界日（第 181 天）保留。
109. 日志清理**分批执行**，单批不影响主流程写入。
110. 头像接口只接受**预设标识**；传 URL 或非法标识返回 400。
111. 服务器上**不存在头像上传目录**，`members.avatar` 中无任何 URL 形态的值。
---

## 24. 开发起点与实施顺序（2026-11-03 新增）

> **背景**：用户拍板「**先开发，不管部署**」。19.5–19.7 已挂起（见 19.A），
> 本节给出「从现在开始写代码」所需的最小前提与推进顺序。

### 24.1 本地开发前提

| 需要 | 说明 |
|---|---|
| Go 1.22+ | 后端本体（Gin + sqlx + golang-jwt + bcrypt + slog） |
| MySQL 8 | **本机装一个，或 Docker 起一个即可**，不涉及服务器规格（19.5 挂起） |
| golang-migrate | 建表走 `migrations/0001_init.up.sql`，不用手工建库 |
| Android Studio | Kotlin + Compose（成员端），模拟器即可调试 |
| 前端工具链 | Web 后台前端**框架尚未选型**（见 24.4），不阻塞后端 |

**开发期明确不需要**：域名、备案、HTTPS 证书、服务器、Nginx、Docker Compose 编排 ——
这些全部属 16 节，**上线前再执行**。

### 24.2 实施顺序（按依赖推进，不要跳）

| 阶段 | 内容 | 完成判据（对应验收条目） |
|---|---|---|
| **P0 骨架** | `go.mod`、配置加载、MySQL 连接池、`migrations/0001`（全部表）、统一响应 `{ok,data,error}`、健康检查 | 服务能起、迁移能跑、表结构与第 3 节逐字段一致 |
| **P1 认证** | 注册（开放注册）、`/api/admin/login` 与 `/api/app/login` 分离、JWT（Access 2h + Refresh 30d）、`typ` 校验 | 6、7、16：两套 token 互打一律 401；后台账号**不能**通过注册页产生 |
| **P2 多租户** | `family_id` 只从 JWT 取、中间件注入、跨组返回 **404** | 1–5：逐接口篡改 `family_id` 无效 |
| **P3 家庭组与成员** | 创建组（成为组长）、邀请码 7 天、按用户名邀请、退出/踢出（积分清零 + `family_bans`）、组长变更 | 19.2 相关条目、邀请码过期与重置 |
| **P4 账目核心** | 记录写入、`score`/`frozen_score`、兑换（`FOR UPDATE`）、赠送（按 `members.id` 升序加锁）、撤销 | 积分不变量组、并发与事务组（83） |
| **P5 睡觉规则 + 屏幕时间** | `screen_rules` 配置、上报、夜间判定、连续奖励档位（`repeat`/`once`）、**覆盖回滚两条记录**、补算 | 64–72、18.3 的 12.x 条目 |
| **P6 任务与投票** | 发布冻结、公开接取、提交（`note` 必填）、全员投票过半、驳回 3 次上限、超时 `expired` | 48–63、95–97 |
| **P7 查询与榜单** | `/api/app/state` 分页（**不返回全量记录**）、记录筛选、积分榜 / 得分榜（`occurred_at` 聚合） | 74–82 |
| **P8 后台 + 审计 + 通知** | 管理员全量接口、审计 append-only（剔除 `pass_hash`、失败不阻断）、180 天清理、站内通知、预设头像 | 84–86、98–101 |
| **P9 客户端** | Android 15 界面（3 个组长专属）、Web 后台前端 | 92–94 及全部界面条目 |

**当前进度（2026-11-03）**：**P0 ✅ / P1 ✅ / P2 ✅ / P3 ✅ / P4 ✅ / P5 ✅ / P6 ✅ / P7 ✅**
（P1 19、P2 15、P3 33、P4 23、P5 37、P6 34、P7 25、P8a 19 项接口测试 + 计分单测全通过，
脚本见 `server/scripts/`）。
**P8 拆成两半**：P8a（审计 + 通知）✅、P8b（后台管理接口 + CSV 导出）✅ 均已完成
（P8a 19 项、P8b 36 项）。服务端 P0–P8 全部完成，**P9 客户端未开始**。
**未来升级（暂不实现）**：全服任务与家庭星已设计、暂缓（见 25 节），排在 P9 之后再说。
**撤销兑换 / 撤销赠送已按用户要求移除**（2026-11-03），接口返回 404，脚本里有断言兜住。
P3 未做的部分：**按用户名邀请（5.2.3）**需要「待确认邀请」状态机 + 新表，单独排到 P3.1。

**顺序理由**：P1/P2 是**所有接口的前置**，晚做就要回头改每个 handler；
P4 的账目模型定不下来，P5–P7 全部无法结算；审计（P8）放最后是因为它**旁路**，
不影响主流程正确性。

### 24.3 开发阶段的验证方式

- 仓库里**没有可运行的前端**（v1 已删），所以验证靠 **`go test` + `httptest` 接口测试**，
  按第 23 节条目逐条写用例，**不要靠「打开页面点一下」**。
- 每个阶段结束跑一次全量 `go test ./...`；**没测试的条目如实标注「未验证」**，不要写成已完成。
- 阶段完成即提交（含记忆），提交完停下汇报，不自行推进下一阶段。

### 24.4 前端选型

| 项 | 结论 | 备注 |
|---|---|---|
| **Web 后台前端框架** | ✅ **Vue 3 + Vite**（2026-11-03 定） | 只影响 P9；后端接口与它无关。目录建议 `web/`，与 `server/` 并列 |
| Android 最低支持版本 | ✅ **`minSdk 26`**（Android 8.0，2026-11-03 拍板） | `UsageStatsManager` 在 8.0 上稳定；覆盖绝大多数在用设备 |

### 24.5 开发库连接（**凭证不入库**）

开发期直连一台远端 MySQL（非本机），连接参数**只写进本地 `.env`**：

- `.env` 已在 `.gitignore` 中，**绝不提交**；仓库只提交 `.env.example`（占位符）
- 敏感值（主机、账号、口令）**不得**出现在 `docs/`、`.memory/`、提交信息里
- 建库与建表由 `server/cmd/migrate` 执行，**不要手工建表**（保证与 3 节一致）

**开发库账号权限**（2026-11-03 已补齐）：

```
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER ON `aabb`.* TO `abc`@`%`
```

- 现状：**ALTER / INDEX 可用**，迁移可以正常演进已有表（`go run ./cmd/migrate -check-ddl` 可复验）
- 沿革：权限最初只有 `CREATE/DROP`，导致 `0002` 的 `ALTER TABLE` 报 `Error 1142`、
  迁移表被打成 `dirty`，当时只能把字段并入 `0001` 并 `-reset` 重建。
  **教训：拿到新库先跑一次 `-grants` 和 `-check-ddl`**，别等到写迁移时才发现
- `-reset`（仅 dev）仍然保留，用于「确实要清库重来」的场景，**不再作为改表的唯一手段**

---

## 25. 未来升级：全服任务与家庭星（**已设计，暂不实现**）

> **2026-11-03 用户拍板**：这个功能**留着作为未来升级，先放下**。
> 本节是当次讨论的完整设计成果，**不进入当前实现范围**——
> 当前接口清单、表结构、验收条目均不含本节内容。
> 将来开工时，把本节内容按编号拆回 3 / 4 / 11 / 13 / 15 / 19 / 23 节即可。

**当时的讨论路径**（值得记住，避免重新争论）：
全服积分榜 → 被否（个人积分跨家庭不可比）→ 全服任务 → 被否（管理员没有积分、一任务一接取）
→ **家庭星**（独立计价）→ 设计完成 → 搁置。

### 25.1 搁置时已定的口径

- 家庭星**只用于排名**，不可消费（用户拍板）
- 认定方式**按任务可配**：自助 / 全服评分 / 平台审核（用户拍板）
- 下面三条是推定值，将来实现前可改：

| # | 事项 | 推定口径 | 改动成本 |
|---|---|---|---|
| 19.20 | 家庭是否默认上榜 | 默认上榜、可主动退出（`stars_public=1`） | 低 |
| 19.21 | `global_vote` 通过阈值 | 平均分 ≥ 3.5 且评分家庭 ≥ 3 | 低 |
| 19.22 | 榜单周期 | 总榜 + 月榜 | 中 |

> 19.20 的风险：家庭名可能含孩子姓名，默认公开等于默认暴露。

### 25.2 全服任务与家庭星（设计稿）

> **背景**：想要「全服排行榜」，但个人积分跨家庭不可比（见 25.3）；
> 想让管理员发任务，但**管理员不是家庭成员、没有积分可冻**（4 节第 2 条）。
> 解法是把两件事合成一件：**平台用「家庭星」这个独立计价单位发全服任务**。

#### 25.2.1 两本账，互不兑换

| | 个人积分 | 家庭星 |
|---|---|---|
| 存储 | `members.score` / `frozen_score` | `families.stars` |
| 定价 | **组长/管理员按组配置**（7.1–7.3） | **平台在发任务时统一设定** |
| 来源 | 睡觉达标（唯一生成入口） | **只有全服任务** |
| 用途 | 组内兑换、赠送、任务转移 | **只用于全服榜排名，不可消费** |
| 流水 | `records` | `family_star_logs` |
| 互兑 | — | **严禁兑换成个人积分** |

> ⚠️ **红线 1：家庭星不得兑换成个人积分。** 一旦能换，个人账本立刻通胀，
> `8.1` 的 `家庭总积分 = Σ(睡觉奖励) − Σ(兑换消耗)` 失效，P4/P5 的全部测试口径作废。
> ⇒ 不提供任何兑换接口，也不做「家庭星补贴孩子零花」这类功能。
>
> ⚠️ **红线 2：同一任务每家庭只计一次。** `UNIQUE(global_task_id, family_id)` 下沉到数据库，
> 否则 4 人家庭做 4 次拿 4 倍，榜单又变回「谁家人口多」。

#### 25.2.2 流程

```
管理员后台发布（标题 / 说明 / 家庭星 / 认定方式 / 起止时间）
        ↓
各组组长在「全服任务」列表看到（active 且在时间窗内）
        ↓
组长代表本家提交（附文字说明）—— 每家庭每任务仅一次
        ↓
按 verify_mode 认定 ──┬─ self        → 立即 approved
                     ├─ admin       → 管理员后台审核
                     └─ global_vote → 其他家庭组长评分，达阈值自动 approved
        ↓
同一事务：写 family_star_logs + families.stars += N + 回填 star_log_id
```

- **全服任务不发个人积分**，不写 `records`，`records.source` 枚举**不变**（5 个值）
- 与组内任务（20.1–20.7）**完全独立**：不占 `tasks` 表、不走组内投票、不冻结任何人积分
- 组长提交不消耗自己的分——家庭星是平台出的，不是家庭自付

#### 25.2.3 三种认定方式

| `verify_mode` | 谁认定 | 通过判据 | 适用场景 |
|---|---|---|---|
| `self` | 组长自助 | 点「已完成」即通过 | 无法验证的软性任务（「全家一起散步」） |
| `global_vote` | 其他家庭组长评分 | 平均分 **≥ 3.5** 且评分家庭数 **≥ 3** | 有成果可看的任务（手工作品、照片描述） |
| `admin` | 平台管理员 | 后台审核通过 | 标准明确、需要把关的任务 |

**`global_vote` 细则**：

- 评分人 = **其他家庭的组长**（家庭代表）。不让全员涌入：一是刷票面太大，二是评分质量不可控
- 每家庭对同一提交只能评一次：`UNIQUE(submission_id, rater_family_id)`
- **不能评自己家**的提交（列表里本就不返回）
- 1–5 分；到期时评分家庭数 **< 3 → 自动 `rejected`**（宁可不发，不发错）
- ⚠️ **提交说明对其他家庭可见**：提交页必须明确写「这段说明会公开给其他家庭看」，
  评分页只显示**家庭名**，不显示成员姓名

#### 25.2.4 发星事务

```sql
-- 三步同一事务，任一步失败全部回滚
UPDATE global_task_submissions
   SET status='approved', reviewed_at=NOW(3), star_log_id=?
 WHERE id=? AND status='submitted';          -- 条件更新，防重复审核

INSERT INTO family_star_logs (family_id, global_task_id, amount, note) VALUES (...);
-- uk_task_family 兜底：重复发放直接撞唯一键

UPDATE families SET stars = stars + ? WHERE id=?;
```

- 审核用**条件更新**（`AND status='submitted'`）：并发下只有一个请求能改成 approved
- `families.stars` 与 `SUM(family_star_logs.amount)` **必须恒等**，验收里专查
- 家庭星只增不减（不可消费），所以**不需要 frozen 概念**

#### 25.2.5 功能项

| # | 功能 | 状态 |
|---|---|---|
| 20.8.5.1 | 管理员发布 / 编辑 / 关闭全服任务 | ⬜ |
| 20.8.5.2 | 家庭端全服任务列表（仅 active 且在时间窗内） | ⬜ |
| 20.8.5.3 | 组长提交（附说明，每家庭限一次） | ⬜ |
| 20.8.5.4 | `self` 自助认定 | ⬜ |
| 20.8.5.5 | `admin` 后台审核（通过 / 驳回） | ⬜ |
| 20.8.5.6 | `global_vote` 跨家庭评分（每家庭一票，达阈值自动通过） | ⬜ |
| 20.8.5.7 | 发星事务（流水 + 余额 + 回填，同事务） | ⬜ |
| 20.8.5.8 | 全服家庭星榜（总榜 / 月榜 / 本家名次） | ⬜ |
| 20.8.5.9 | 家庭主页展示自己的家庭星 | ⬜ |
| 20.8.5.10 | 退出榜单（`stars_public=0`，仍累计） | ⬜ |

#### 25.2.6 边界规则

| 场景 | 处理 |
|---|---|
| 非组长提交 | 拒绝（家庭由组长代表） |
| 同一家庭重复提交 | 409（`UNIQUE(global_task_id, family_id)`） |
| 任务未开始 / 已结束 | 不在列表展示 / 拒绝提交 |
| 提交说明为空 | 拒绝（说明是唯一凭证） |
| 给自己的提交评分 | 拒绝 |
| 同一家庭重复评分 | 409 |
| 到期评分家庭数 < 3 | 自动 `rejected`，不发星 |
| 管理员驳回后重提 | **不允许**（每家庭只有一次机会，避免反复打扰评分人） |
| 家庭星试图兑换个人积分 | **不存在此接口**（红线 1） |
| 家庭解散 / 成员进出 | 家庭星留在家庭，不随成员走 |
| 已发星的任务被管理员删除 | 拒绝删除，只能 `closed`（保留流水） |

> **产品提醒（如实记录）**：家庭星**对孩子没有直接激励**——它不能换任何东西，
> 只影响家庭在全服的排名。所以全服任务的参与动力来自「家庭荣誉 + 家长推动」，
> 不要指望孩子像对兑换商城那样主动去做。若要给孩子动力，应由组长另发**组内任务**
> （用个人积分，走第 20 节原有流程）。

---

### 25.3 全服家庭星榜口径

| 项 | 口径 |
|---|---|
| 排序 | `families.stars` 降序；同分按 `families.id` 升序 |
| 周期 | 总榜（累计）+ 月榜（流水当月聚合） |
| 返回字段 | 名次、家庭名、家庭星、`is_mine`；**本家名次单独返回** |
| **禁止返回** | 成员列表、成员姓名、个人积分、`records`、明文 `family_id` |
| 退出 | `stars_public=0` 不上榜，仍累计 |

> **对账口径**：`families.stars` 必须恒等于 `SUM(family_star_logs.amount)`。

### 25.4 隔离例外（实现时新增 4.3 第 7 条）

跨家庭读取只有两个白名单入口：**全服家庭星榜**与**全服任务**。允许读的字段**穷举**：
家庭名、家庭星、任务模板、`global_vote` 模式下提交的说明文字、评分。
禁止读：成员列表、成员姓名、个人积分、`records`、明文 `family_id`。
⇒ 必须在 service 层用**专用查询**，不得复用组内 repo 方法。

### 25.5 数据库变更（4 张新表 + families 两列）

**families 加列**：`stars INT NOT NULL DEFAULT 0`、`stars_public TINYINT(1) NOT NULL DEFAULT 1`

```sql
CREATE TABLE global_tasks (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  title        VARCHAR(64)  NOT NULL,
  description  VARCHAR(500) NULL,
  stars        INT          NOT NULL,
  verify_mode  ENUM('self','global_vote','admin') NOT NULL DEFAULT 'self',
  start_at     DATETIME(3)  NOT NULL,
  end_at       DATETIME(3)  NOT NULL,
  status       ENUM('draft','active','closed') NOT NULL DEFAULT 'draft',
  created_by   BIGINT UNSIGNED NOT NULL,
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_status_time (status, end_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE global_task_submissions (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  global_task_id  BIGINT UNSIGNED NOT NULL,
  family_id       BIGINT UNSIGNED NOT NULL,
  submitted_by    BIGINT UNSIGNED NOT NULL,
  note            VARCHAR(255) NOT NULL,
  status          ENUM('submitted','approved','rejected') NOT NULL DEFAULT 'submitted',
  avg_score       DECIMAL(3,2) NULL,
  rater_count     INT NOT NULL DEFAULT 0,
  reviewed_by     BIGINT UNSIGNED NULL,
  reviewed_at     DATETIME(3) NULL,
  star_log_id     BIGINT UNSIGNED NULL,
  created_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_task_family (global_task_id, family_id),
  KEY idx_family (family_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE global_task_ratings (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  submission_id    BIGINT UNSIGNED NOT NULL,
  rater_family_id  BIGINT UNSIGNED NOT NULL,
  rater_member_id  BIGINT UNSIGNED NOT NULL,
  score            TINYINT UNSIGNED NOT NULL,
  created_at       DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_submission_family (submission_id, rater_family_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE family_star_logs (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id       BIGINT UNSIGNED NOT NULL,
  global_task_id  BIGINT UNSIGNED NOT NULL,
  amount          INT NOT NULL,
  note            VARCHAR(64) NOT NULL,
  created_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_task_family (global_task_id, family_id),
  KEY idx_family_time (family_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 25.6 接口（实现时登记进 15 节）

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| A1 | `/api/app/global-task/list` | GET | 全服任务列表（含本家提交状态） |
| A2 | `/api/app/global-task/detail` | GET | 详情（`global_vote` 返回待评分提交） |
| A3 | `/api/app/global-task/submit` | POST | 组长代表本家提交（每家庭每任务仅一次） |
| A4 | `/api/app/global-task/rate` | POST | 给其他家庭提交评分（1–5，每家庭一票） |
| A5 | `/api/app/stars/ranking` | GET | 家庭星榜（`period=all\|month`） |
| A6 | `/api/admin/global-task/list` | GET | 任务列表（含草稿） |
| A7 | `/api/admin/global-task/save` | POST | 发布 / 编辑 / 关闭 |
| A8 | `/api/admin/global-task/submissions` | GET | 全部家庭提交（含评分汇总） |
| A9 | `/api/admin/global-task/review` | POST | 审核通过 / 驳回 |

### 25.7 验收条目（实现时登记进 23 节，接 101 之后）

112. 全服任务完成后 **`records` 无新增行**、`members.score` 不变。
113. 同一家庭对同一全服任务重复提交返回 **409**；`family_star_logs` 只有一行。
114. 发星后 `families.stars` 与 `SUM(family_star_logs.amount)` **恒等**。
115. `global_vote` 不能给自己家的提交评分；同一家庭重复评分 **409**。
116. `global_vote` 到期评分家庭数 < 3 → 自动 `rejected`，不发星。
117. 全服榜响应中**不含**成员名、个人积分、`records`、明文 `family_id`。
118. `stars_public=0` 的家庭不上榜，但 `stars` 仍正常增长。

### 25.8 Android 界面（实现时登记进 13 节，界面数 15 → 17）

| # | 界面 | 关键内容 | 可见角色 |
|---|---|---|---|
| 13.16 | **全服任务** | 平台任务列表、本家提交、待评分 | 全部（提交仅组长） |
| 13.17 | **家庭星榜** | 总榜 / 月榜、本家名次、退出榜单 | 全部 |
