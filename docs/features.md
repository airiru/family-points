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
- ⇒ Android **不再承载后台管理界面**，共 **17 个界面**（其中 3 个仅组长可见，见 13 节）

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
  -- 家庭码（2026-11-06）：8 位、唯一且**永不可更改**、不过期，建组时自动生成。
  -- 「7 天有效期 + 可重置」的邀请码语义已废除（19.22 作废）。
  invite_code     CHAR(8)      NOT NULL UNIQUE,
  -- 隐藏 = 搜不到 + 凭码申请也拒绝（2026-11-07 定稿），对外等价于码不存在
  code_hidden     TINYINT(1)   NOT NULL DEFAULT 0,
  -- 不设 owner_member_id：组长身份唯一来源是 members.role='leader'（见设计要点 6），
  -- 两处都存必然分叉
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 被踢出 / 自行退出后的禁入名单（同组不可再次加入）
CREATE TABLE family_bans (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id  BIGINT UNSIGNED NOT NULL,
  member_id  BIGINT UNSIGNED NOT NULL,
  reason     ENUM('kicked','left') NOT NULL,
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_family_member (family_id, member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 待确认状态机（P3.1，5.2.3）：两个方向共用——
-- direction='invite' 组长邀请（inviter=组长）；direction='request' 凭家庭码申请（inviter=invitee=申请人）
CREATE TABLE family_invitations (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id       BIGINT UNSIGNED NOT NULL,
  inviter_id      BIGINT UNSIGNED NOT NULL,   -- invite=组长 / request=申请人自己
  invitee_id      BIGINT UNSIGNED NOT NULL,   -- 要入组的那个人（两个方向都是他）
  direction       ENUM('invite','request') NOT NULL DEFAULT 'invite',
  status          ENUM('pending','accepted','rejected','cancelled','expired') NOT NULL DEFAULT 'pending',
  expires_at      DATETIME(3)  NOT NULL,      -- 7 天（邀请/申请本身的有效期；家庭码永不过期）
  decided_at      DATETIME(3)  NULL,
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  -- 生成列：只有 pending 行参与唯一约束，其余状态为 NULL 不冲突。
  -- 同组同一人最多一条待确认，历史 accepted/rejected/cancelled 可留多条追溯。
  -- 与 members.leader_family_id 同构。
  pending_invitee BIGINT UNSIGNED GENERATED ALWAYS AS (IF(status = 'pending', invitee_id, NULL)) STORED,
  UNIQUE KEY uk_pending_invitee (family_id, pending_invitee),
  KEY idx_invitee_status (invitee_id, status),
  KEY idx_family_status (family_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 后台管理员账号（与前台成员完全独立，不参与积分）
CREATE TABLE admins (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  username   VARCHAR(20)  NOT NULL UNIQUE,  -- 登录凭证，注册后永不可改（5.1.6）
  nickname   VARCHAR(32)  NOT NULL DEFAULT '',  -- 显示名，可改（5.4.9）；新建时回填为 username
  pass_hash  VARCHAR(60)  NOT NULL,          -- bcrypt
  must_change_password TINYINT(1) NOT NULL DEFAULT 1,  -- 首次登录强制改密（含默认账号）
  disabled   TINYINT(1)   NOT NULL DEFAULT 0,          -- 停用后无法登录（19.19）
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
  -- 注意：管理员是「网站运营者」，不属于任何家庭组，可跨组查看与管理。
  -- 家庭用户（含组长）永远没有后台账号；但管理员同样是「用户」，也有昵称。
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 成员（账号即成员：username 非空即可登录前台）
CREATE TABLE members (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id       BIGINT UNSIGNED NULL,       -- 注册时为空；被组长邀请或自行创建家庭组后才有值
  nickname        VARCHAR(32)  NOT NULL,      -- 显示名，可改（1–32 字）；**不是登录凭证**
  score           INT          NOT NULL DEFAULT 0,
  frozen_score    INT          NOT NULL DEFAULT 0,   -- 因发布任务被冻结的积分，不可动用
  hidden          TINYINT(1)   NOT NULL DEFAULT 0,
  avatar          VARCHAR(64)  NULL,          -- **预设头像标识**（如 'bear'），不是 URL；无自定义上传
  username        VARCHAR(20)  NOT NULL UNIQUE,  -- 登录凭证，注册后永不可改；全局唯一 ⇒ 一人只属一个家庭组
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

-- 兑换单（P4 已实现，与 0001_init 迁移一致）
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
  night_rule_id    BIGINT UNSIGNED NULL,    -- 达标时计入的睡觉规则（rules.id）；由后端推导，前端不必传
  -- 未按时睡觉的扣分口径（2026-11-05）
  night_penalty_unit_min INT NOT NULL DEFAULT 30,  -- 单位：30 或 60 分钟
  night_penalty_points   INT NOT NULL DEFAULT 0    -- 每单位扣 N 分（0 = 不扣分）
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
  UNIQUE KEY uk_member_date (member_id, report_date),
  KEY idx_family_date (family_id, report_date)                -- 榜单批量算连续天数用（全组一次扫描）
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 成员设备（无令牌，仅用于区分与展示）
CREATE TABLE member_devices (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id     BIGINT UNSIGNED NOT NULL,
  member_id     BIGINT UNSIGNED NOT NULL,
  device_key    VARCHAR(64)  NOT NULL,
  device_model  VARCHAR(64)  NOT NULL,
  first_seen_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  last_seen_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_member_device (member_id, device_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 按设备的当日原始上报（多设备各一行，汇总后才进 screen_reports 结算）
CREATE TABLE screen_device_reports (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id    BIGINT UNSIGNED NOT NULL,
  member_id    BIGINT UNSIGNED NOT NULL,
  device_key   VARCHAR(64)  NOT NULL,
  device_model VARCHAR(64)  NOT NULL,
  report_date  DATE         NOT NULL,
  total_min    INT          NOT NULL,
  night_min    INT          NULL,
  reported_at  DATETIME(3)  NOT NULL,
  updated_at   DATETIME(3)  NULL,
  UNIQUE KEY uk_member_device_date (member_id, device_key, report_date)
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
   **列名保留 `member_name` 不随 `nickname` 改名**：它是流水里的历史快照列，
   改列名等于要求迁移生产库的历史数据，收益为零（见 5.1.6）。
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
8. **登录凭证与显示名彻底分离（P3.1）** —— `username` 是登录凭证、全局唯一、注册后永不可改；
   `nickname` 是显示名、可改、1–32 字符。`members` 与 `admins` **两张账号表都有这两个字段**。
   API / DTO 一律用 `nickname`，不再出现 `name` 表示人。
   早期 `members.name` 语义上本来就是昵称，只是名字没对齐，本阶段统一改名（不是新增能力）。
9. **邀请的「同组同一人最多一条 pending」下沉到数据库** —— 用生成列
   `pending_invitee = IF(status='pending', invitee_id, NULL)` + `UNIQUE(family_id, pending_invitee)`，
   与组长唯一约束同构。应用层不做「先查再插」的竞态判断，撞 1062 直接转业务错误 409。
   `expired` **不主动落库**：不设定时任务，读路径按 `expires_at` 惰性判定（与任务投票清扫同一取舍）。
   唯一例外是**同组再次邀请同一人**：`Create` 会在同事务里把该人已过期的 pending 落为 `expired`
   再插新行。否则生成列只看 `status`，一条过期的 pending 会永久占住 `uk_pending_invitee`，
   而列表又不显示过期条目，组长连「取消」都拿不到 ID —— 这个人就再也邀不动了。

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
| **查看家庭码** | ✅（后台只读展示） | ✅ | ✅ |
| **隐藏 / 显示家庭码** | ❌ | ✅ | ❌ |
| **配置睡觉时间与奖励规则** | ✅ | ✅ | ❌ |
| **商城商品管理** | ✅ | ✅ | ❌ |
| 审计日志 | ✅ | ❌ | ❌ |
| **解散家庭组**（需输入组名确认） | ✅（15.2.40） | ❌ | ❌ |

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
7. **业务判定只在后端**（2026-11-05 定）：前端（Android / Web）**只负责发送配置、展示服务端结果**，
   **不得自己判断规则**。已经为此改掉的四处：
   - 夜间是否达标 → 读 `screen_reports.night_ok`（原本前端算 `nightMinutes <= nightMax`）
   - 商品能否兑换 → 读 `items[].affordable`（原本前端比 `score >= cost`）
   - 赠送上限 → 读 `state.transfer_max`（原本前端拿 `score` 当上限）
   - 密码/用户名长度 → 只由后端校验并返回错误（原本三处各写一套：4 位 / 6 位 / 8 位）
   ⇒ 前端可以做的只有：**空值提示、两次输入一致性、输入格式（纯数字）**这类纯表单问题。
   判据：**同一规则如果在两处实现，迟早会漂 —— 只留后端一处。**
8. **登出必须吊销 refresh**（2026-11-05 定）：refresh 是无状态凭证，
   前端「把 token 从本地删掉」**不等于**服务端失效——拿到过 refresh 的人仍能换出 access。
   ⇒ `POST /api/admin/logout` / `/api/app/logout` 按主体**全量吊销**，
   语义是「该账号所有会话下线」。前端登出按钮必须先调接口，再清本地。
   **两端都已落实**（2026-11-05）：Web `AppLayout` / `ChangePassword`、
   Android `AppViewModel.logout()` 均先 `runCatching { api.logout() }` 再清本地
   （网络失败放行本地登出，否则断网就退不出去）。
   真机实测：登出后该成员 `refresh_tokens` 全部 `revoked_at` 非空。

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
| 5.1.6 | **username 与 nickname 分离（P3.1）** | ✅ `username` 登录凭证、全局唯一、**注册后永不可改**；`nickname` 显示名、可改、1–32 字。`members` / `admins` 两张账号表都有，新建时不填昵称则回填为 `username`。所有改资料接口显式拒绝请求体里带 `username` |

> **口径（2026-11-06 用户明确）**：原来 `members.name` 就是「昵称」、本来就可改，
> 只是字段命名混着「名称 / 昵称」两套叫法；`username` 本来也**没有任何 UPDATE 路径**。
> 本阶段做的是**统一命名为 `nickname`** + 给 `admins` 补昵称，不是新造一套身份体系。
> `records.member_name` 是历史流水快照列，**列名保留不动**。

> ⚠️ **创建家庭组 ≠ 获得后台管理员**。后台是给**网站运营者**用的，
> 家庭用户（包括组长）**永远没有后台账号**，也没有任何后台入口。

### 5.2 加入家庭组：家庭码 / 待确认邀请 / 凭码申请

> **家庭码（2026-11-06 起）**：8 位、全局唯一、建组时自动生成，**永不可更改、不过期、不可重置**；
> 原「邀请码 7 天有效期 + 组长重置」语义已废除。组长可把它设为**隐藏**：
> **隐藏 = 搜不到 + 凭码申请也拒绝（2026-11-07 定稿）**，对外等价于码不存在。
> 凭家庭码加入必须经组长审批（`direction='request'`），不存在绕过审批的直接入组接口。

| # | 功能 | 状态 |
|---|---|---|
| 5.2.1 | **查看本组家庭码 / 组长设置是否可被搜到** | ✅ 全员可读（15.3.52）；隐藏开关仅组长（15.3.34）。隐藏后搜索与凭码申请**都**拒绝 |
| 5.2.2 | **未入组者按家庭码搜索家庭** | ✅ 隐藏的码返回 `found=false`，与不存在不可区分（15.3.53） |
| 5.2.2a | **凭家庭码申请加入（需组长审批）** | ✅ 校验：码存在且未隐藏、本人未入组、未被该组禁入；同组已有 pending → 409（15.3.55）。**凭码直接入组的旧接口已删除** |
| 5.2.2b | **未入组者创建家庭组** | ✅ 建组者成为组长（仅前台身份），系统生成家庭码（15.3.54） |
| 5.2.3 | **App 侧按用户名发起「待确认邀请」** | ✅ **P3.1 已实现**：`family_invitations` 状态机 + 被邀请人确认/拒绝；与家庭码申请并存。后台 15.2.3 的「按用户名直拉入组」语义不变、独立保留 |
| 5.2.4 | 加入后 `family_id` 设为该组，`role='member'` | ✅ 三条入组路径（接受邀请 / 申请审批通过 / 后台直拉）共用同一段入组事务 `joinTx` |
| 5.2.5 | **~~邀请码有效期 7 天~~** | ♻️ **已废除**：家庭码永不过期、不可重置；7 天有效期只适用于**邀请与申请本身**（见 5.2.6） |
| 5.2.6 | **邀请 / 申请有效期 7 天** | ✅ **惰性判定**：不设定时任务，读列表与接受/审批时按 `expires_at` 判断；`expired` 不主动落库，仅在同组再次邀请/申请时由写路径顺手落地（见下） |
| 5.2.7 | **同组同一人最多一条 pending** | ✅ 生成列 + `uk_pending_invitee` 兜底；重复邀请返回 **409**，不产生第二条 |
| 5.2.8 | **被邀请人未入组也能查收 / 处理** | ✅ 三个接口挂 `/api/app` 而非 `RequireFamily()` 的 `inFamily` 组；`family_id` 一律取服务端邀请行，不接受请求体指定 |
| 5.2.9 | **接受后其它组发来的 pending 一并作废** | ✅ 与入组在同一事务；避免一人多组。接受 / 拒绝 / 取消都写 `decided_at` |

> **为什么前台「按用户名邀请」必须待确认，后台 15.2.3 却直接拉入组**：
> 后台管理员是运营方、可跨组管理，直拉是管理动作；组长只是家庭用户，
> 不能替别人决定归属。两条路径共用同一段 `joinTx` 入组 SQL，差别只在「谁触发」。
>
> **状态机**：`pending → accepted / rejected / cancelled`（落库），`expired` 默认只在读路径判定；
> 同组再次邀请同一人时，写路径会先把该人**已过期的 pending** 落为 `expired` 再插新行，
> 以释放 `uk_pending_invitee`（否则过期行会永久占用唯一键，且列表看不到它、无法取消）。
> 发起校验顺序：用户名存在 → 不能邀自己 → 对方未入组。
> **邀请不检查禁入名单**（2026-11-07 定稿：封禁只挡「本人申请」，不挡「组长邀请」）；
> 接受校验：邀请属于本人 → 仍 pending 且未过期 → 本人未入组；
> **接受成功时同事务清除本人对本组的封禁**（避免「人在组里、名单里还被禁」的自相矛盾）。
> 凭码申请（`direction='request'`）则相反：码不存在或已隐藏、已被禁入都会被拒（403/400）；
> 审批通过走与接受邀请相同的 `joinTx`。
> 并发都以 `FOR UPDATE` + 条件 UPDATE 兜住。
> 通知与审计都是**业务成功后的旁路**：失败只记日志，不阻断邀请。

### 5.3 退出与踢出（积分清零 + 禁入）

| # | 功能 | 状态 |
|---|---|---|
| 5.3.1 | 成员自行退出家庭组 | ✅ **必须二次确认**（前端弹窗 + 后端 `confirm=true`） |
| 5.3.2 | 组长踢出成员 | ✅ 同样二次确认；不能踢自己 |
| 5.3.3 | **退出 / 踢出后积分清零** | ✅ `score=0`、`frozen_score=0`；积分不带走、不退还 |
| 5.3.4 | **禁止再次加入该家庭组** | ✅ 写入 `family_bans`，`UNIQUE(family_id, member_id)`。**封禁只挡「本人凭码申请」（403），不挡「组长邀请」**；接受邀请时自动清除封禁（2026-11-07 定稿） |
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
| 5.4.2 | **默认账号首次登录强制改密** | ✅ 未改密前除改密接口外全部拒绝；Web 14.1a 承接 |
| 5.4.3 | 运营方可在后台新增 / 停用管理员账号 | ✅ `/api/admin/accounts`（列表/新建/停用）+ Web 14.13 |
| 5.4.4 | 后台登录 / 退出 | ✅ 登录 `/api/admin/login`；退出 `POST /api/admin/logout`（**吊销该管理员全部 refresh**） |
| 5.4.5 | 修改密码 | ✅ `POST /api/admin/password` + Web 14.1a |
| 5.4.6 | Token 刷新（Access 2h + Refresh 30d） | ✅ `/api/admin/refresh`、`/api/app/refresh`；refresh 存哈希、可吊销、轮换 |
| 5.4.7 | **可管理所有家庭组的所有内容** | ✅ 成员、规则、任务、商品、设备、数据、审计（14 节 13 个页面） |
| 5.4.8 | 管理员操作全部写审计日志 | ✅ 调用点以 **21.2 表**为唯一事实源（不写死计数），跨组操作额外带 `audit_family_id` |
| 5.4.9 | **管理员也有昵称，且可改** | ✅ `admins.nickname`；`POST /api/admin/me/profile` 只改昵称，Web 14.13 提供「我的昵称」卡片。`username` 同样永不可改，请求体带非空 `username` 直接 **400** |

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
| 5.5.2 | 解散家庭组 | ✅ **2026-11-07**：仅后台运营者可执行（15.2.40），**必须输入当前组名逐字确认**。级联语义见下 |
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

### 8.4 余额可以为负数（2026-11-06 用户明确的全局不变量）

> **`members.score` 允许为负，且没有下限**。这是刻意的「负债」语义，不是并发 bug，
> 也不是漏掉的护栏。实现时**不要**给自动扣分路径补 `score >= 0` 检查。

| 场景 | 规则 |
|---|---|
| **自动扣分**（屏幕时间日总量超标、夜间惩罚、覆盖重算的回滚） | 直接 `score = score + delta`（`delta` 可为负）；**余额已经是负数也继续扣**，扣多少记多少 |
| **主动支出**（兑换 9 节、赠送 10.5、发布任务冻结 20.5） | **仍要求余额充足**：事务内 `score < 需要分值` 即 `ErrInsufficient` 拒绝；前端按后端 `affordable` 置灰。负余额下这些操作一律不可用 |
| 流水 | 每次变动仍必须写 `records`（8.1 铁律）；**负分流水是合法记录**，不得因「余额会变负」而截断分值或跳过写入 |

代码落点：`members.score` 是**有符号 `INT`** 且无 `CHECK`；`scoring.DayPoints` / `NightPenalty`
返回负值；`repo/screen.go` 的 `Settle` 与 `rollbackReport` 都只做加减、不做下限判断。
相对地，`repo/ledger.go`（兑换 / 赠送）与 `repo/task.go`（冻结）三处 `FOR UPDATE` 后
的余额校验**必须保留**——那是主动支出的门槛，与自动扣分的下限无关。

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
>
> 「余额不足 → 拒绝」只约束**主动支出**；屏幕时间的自动扣分不受此限，
> 余额可以扣成负数并继续扣（8.4）。两者不要混为一谈。

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
| 认证 | **成员登录态**（2026-11-05 起）；设备令牌机制**已删除**，手动上传仍不允许 |
| 数据来源 | 只有 `auto`（设备自动上报） |
| **余额下限** | **无**：扣分可把 `score` 扣成负数，负余额后继续扣分仍生效（8.4 全局不变量）；只有兑换 / 赠送 / 任务冻结才要求余额充足 |

> **覆盖示例**（对应 19.3 的结论）：
> 第一次上报「当天累计 10 分钟」→ 判达标，记 +2；
> 第二次上报「当天累计 30 分钟」→ **回滚 +2，按 30 分钟重新结算**，若超阈值则改记 -3。
> 最终状态与「一开始就上报 30 分钟」完全一致。

### 12.2 功能项

> 状态于 2026-11-06 按 `android/app/src/main/java/.../screen/` 与 `ui/screens/ProfileScreens.kt`
> 逐条复核：早期整片 ⬜ 是采集链路落地前的标记，现已按实际实现更新。

| # | 功能 | 状态 |
|---|---|---|
| 12.2.1 | 权限引导（`PACKAGE_USAGE_STATS`） | ✅ `hasPermission()` 用 AppOps 判定，未授权时页面跳 `Settings.ACTION_USAGE_ACCESS_SETTINGS` |
| 12.2.2 | 采集当日总时长（`queryEvents` 配对 RESUMED/PAUSED） | ✅ `UsageStatsReader.read`，**不用 `INTERVAL_DAILY`**。排除名单：本应用 / 桌面 / 系统 UI / 设置（12.2 顶部）——**桌面时间不计入（2026-11-07 用户确认）**，桌面久坐不代表「玩手机」 |
| 12.2.3 | 排除系统应用与息屏时段 | ✅ 排除本应用 / 系统 UI / 设置 / 常见桌面；息屏由 `ACTIVITY_PAUSED` / `ACTIVITY_STOPPED` 结算，**无独立 `SCREEN_OFF` 监听** |
| 12.2.4 | 定时上报（WorkManager） | ✅ **2026-11-07 定稿**：`ReportScheduler` 每天 **10:00 / 18:00** 各一次（两个 unique periodic work，升级时自动取消旧版 21:00 单任务；联网约束） |
| 12.2.5 | 打开 App 补算最近 7 天 | ✅ **2026-11-07**：`ScreenReportWorker` 每次运行上报 **D-7..D-1 有事件的天**（逐日结算，服务端幂等跳过未变值）+ 今天快照。**事件数为 0 的天跳过**——可能是系统没保留那么久的事件，拿 0 覆盖会毁掉真实结算 |
| 12.2.6 | 失败重试（指数退避） | ✅ Worker 返回 `Result.retry()`，用 WorkManager 默认指数退避（未显式设 `setBackoffCriteria`）；服务端按「成员×日」幂等，重试不会重复计分 |
| 12.2.7 | 上报状态展示 | ✅ 屏幕时间页「上报状态」卡片。**2026-11-07 起显示真实情况**：Worker 每次成败写入本地（`ReportStatusStore`），卡片显示 上次成功时间 / 尚未上报 / 上报异常 + 最近失败原因（如 `HTTP 401`、连不上后端）；不再无条件显示「正常」。权限卡同时做**功能性探测**（部分 ROM AppOps 已 allow 但系统层未放行，`queryEvents` 会抛安全异常 → 显示未授权） |
| 12.2.7a | **上报时机与刷新策略**（2026-11-07 用户定稿） | ✅ 打开 App 补算一次（`runNow()`）+ 每天 10:00/18:00 定时 + 屏幕页**手动上报按钮**。屏幕页**不做任何自动刷新/上传**：进页只读服务端数据，ON_RESUME 仅重查权限状态；「刷新数据」按钮已更名「手动上报」 |
| 12.2.8 | 国产 ROM 保活引导 | ✅ **2026-11-07**：小米等 ROM 的进程被杀成 stopped 后 WorkManager 不再运行（真机实案），屏幕时间页按厂商显示引导卡（小米：应用管理开「自启动」+ 省电策略「无限制」+「打开应用信息」入口）；重新打开 App 仍会立即补算 |
| 12.2.9 | **采集并上报夜间时段使用分钟数**（`night_min`） | ✅ 服务端已支持；Android 端 `ScreenReportWorker` + `UsageStatsReader` 已实现（含跨零点切分、按本机登录成员取规则） |
| 12.2.10 | **设备绑定与解绑**（2026-11-07） | ✅ 一台设备全局只绑一个账号（`member_devices.device_key` 唯一）：上报时未绑定 → 自动绑定当前账号，绑定他人 → **409**；解绑=组长直接解绑 / 成员申请审批（15.5，含通知与审计）；解绑后设备可在任何账号下次上报时重新绑定。「我的 → 设备管理」页展示绑定列表（标记本机）、解绑入口、撤回申请；组长的设备管理页内联审批待审批申请 |

### 12.3 结算事务（MySQL 版）

核心是 **upsert + 重算**：同一天已有结算行时，先回滚旧积分与旧记录，再按新值结算。

```sql
BEGIN;
  -- 1. 校验成员 JWT，得到 family_id / member_id；客户端自报 device_key + device_model
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
| 保存规则时未传 `night_rule_id` | **由后端自动取本组唯一那条规则**（每组只有一条），取不到才报错 |
| `night_penalty_unit_min` 不是 30 / 60 | 400 拒绝（前端只提供这两个选项） |
| `night_penalty_points < 0` | 400 拒绝 |

#### （六）未按时睡觉的扣分（2026-11-05 新增）

达标只给奖励、不达标**不扣分**的旧口径太粗：夜里用 1 分钟和用 5 小时结果完全一样。
现在改成**按单位阶梯扣**：

```
超出 = max(0, night_min − night_max_min)
单位数 = ceil(超出 ÷ unit)          // 不满一个单位也算一个单位
扣分   = −(单位数 × points)
```

| 参数 | 取值 | 说明 |
|---|---|---|
| `night_penalty_unit_min` | **30 或 60** | 前端只给「每半小时 / 每小时」两个选项 |
| `night_penalty_points` | ≥ 0 | 每单位扣 N 分；**0 = 不扣分**（退回旧口径：不加分 + 连续清零） |

示例（`night_max_min = 0`、单位 30、每单位 2 分）：

| 夜间使用 | 超出 | 单位数 | 扣分 |
|---|---|---|---|
| 0 分钟 | 0 | 0 | **0（达标，正常加分）** |
| 1 分钟 | 1 | 1 | −2 |
| 30 分钟 | 30 | 1 | −2 |
| 31 分钟 | 31 | 2 | −4 |
| 100 分钟（12 点 30 + 1 点 50 + 2 点 20） | 100 | 4 | **−8** |
| 同上但单位 60 | 100 | 2 | −4 |

**判定权在后端**：前端（Android / Web）只负责①把配置发上来 ②展示服务端算好的
`night_ok` 与流水。**任何前端都不得自己判断达标或算扣分** —— 否则两端口径会漂移。

流水标题形如：`未按时睡觉：夜间使用 100 分钟，超 100 分钟（4 × 30 分钟）`，
积分为负值；达标时标题为 `按时睡觉（夜间使用 0 分钟，上限 0）`，积分为正（含连续档位奖励）。

---

### 12.5 客户端明细展示（2026-11-05 新增）

App 的「屏幕时间」页除了今日总时长，还要能看到**每小时**与**每天**的明细：

| 视图 | 内容 | 数据来源 |
|---|---|---|
| 今日每小时 | 24 根柱（0–23 点），夜间时段用蓝色区分；点柱子看该小时分钟数，另显示峰值小时 | **设备本地**（`UsageStatsManager.queryEvents`） |
| 近 7 天每日 | 每天一行：日期（今天/昨天/月-日）+ 时长条 + 具体时长 | 同上，一次查询按「天 × 小时」分桶 |

**口径与上报完全一致**：同一套 `queryEvents` 配对逻辑、同样排除本应用/桌面/系统界面。
一次 `queryEvents` 覆盖 7 天，把每段前台时间按小时边界切开分别累加 ——
逐天各查一次会重复扫描事件。

**2026-11-06 起改为上传**（推翻上一版「不上传」的结论）：
小时桶写入 `screen_hourly_reports`，只用于**展示**，**不参与判定或计分**。
原因很直接——不上传就看不到**别人**的每小时，而「看家庭成员的详细时间」是明确需求；
几百字节的上报体换一个可用功能，这个取舍成立。
⇒ 计分口径不变（仍是 `screen_reports` 的「成员×日」一行）；小时数据是纯附加信息，
可以随时丢弃重算，不影响任何分数。

> 夜间时段取**本机登录成员自己的规则**（`screen_rules`），不是写死的 0–7 ——
> 多成员家庭里小学生和初中生的睡觉时间点本来就可以不同（25 条）。

### 12.7 逐小时明细与成员详情（2026-11-06 新增）

**表**：`screen_hourly_reports`（`member_id` + `device_key` + `report_date` + `hour` 唯一），
一行 = 某成员某设备某小时的使用分钟数。后报覆盖（同一键先删后插，事务内完成）。

**两条写入路径**：

| 路径 | 时机 | 是否结算 |
|---|---|---|
| `POST /api/app/screen/report` | Worker 每天 21:00 上报「昨天」 | ✅ 结算、发分 |
| `POST /api/app/screen/snapshot` | 打开 App 时上报「今天」的实时快照 | ❌ **不结算、不发分** |

> 快照单独开一条路径，是因为家人想看的「今天」必须实时；若复用 `report`，
> 会在一天内反复重算当天积分，分数早中晚各变一次，流水会很脏。

**读取**：`GET /api/app/screen/member/:member_id?date=YYYY-MM-DD`（`member_id=0` 为自己），
返回：总时长（跨设备相加）、夜间、判定与积分（**只认结算行**，未结算为 `null` + `settled=false`）、
24 小时桶（跨设备相加）、各设备明细（含各自的 24 桶与近 7 天）、近 7 天合计（固定 7 条，缺的补 0）。

**一致性的硬约束**：客户端上传 24 个桶时，服务端以**桶之和**作为 `total_min`，
所以「总时长」与「逐小时加起来」永远相等，不存在两个数字打架。

**界面**（13.12）：顶部总览 + 设备切换（全部 / 单台）+ 每小时 + 近 7 天 + 各设备；
下方「其他成员」卡片可点，进入同一套组件的只读详情页。
**列表不含自己**——自己的数据已经在页面顶部，重复列一遍是噪音；
只有一个人的家庭整段不显示。

---

### 12.6 后台设备明细（2026-11-06 新增）

> ⚠️ 本节与 12.5 **不是一回事**：12.5 是设备本地按小时算的、不入库的明细；
> 本节是服务端 `screen_device_reports` 里**已经上报**的「每台设备每天一行」原始值。

Web 后台「屏幕时间」页除结算行外，另有一张「设备原始上报」表：

| 内容 | 数据来源 | 口径 |
|---|---|---|
| 设备原始上报列表 | `screen_device_reports` | 一行 = 某成员 × 某设备 × 某天；`total_min` 是该设备自报的**当天累计总时长** |
| 已登记设备 | `member_devices` | 上报过就会登记，`last_seen_at` 为最近一次；**当天没上报的设备也在列表里** |

筛选：`date=YYYY-MM-DD`（省略为不限日期）、`member_id=0` 全组、`limit`（默认 50，上限 200）。
`date` 格式不合法返回 400。**只读**：不重新汇总、不重新判定，
结算仍按「同一成员当天全部设备相加 → 一条 `screen_reports`」（12.4）。

---

## 13. Android App（Kotlin 原生）

> 面向**家庭成员**（组长与成员），**不含后台管理**。
> **有注册入口**——用户自助注册（见 5.1），**管理员不能代建账号**（决策 24）。

| # | 界面 | 关键内容 | 可见角色 |
|---|---|---|---|
| 13.1 | 启动页 | 检查前台 token，有效则直进主页 | 全部 |
| 13.2 | 登录 / 注册 | 登录；注册创建账号（用户名 + 密码 + 昵称），可顺带创建家庭组（成为组长） | 全部 |
| 13.3 | **积分榜 / 得分榜** | 顶部切换两个榜；积分榜看余额，得分榜看周/月表现；下方**积分趋势折线**（全组、周/月、`earned`/`net` 双线，2026-11-06 补） | 全部 |
| 13.4 | 成员详情 | 该成员记录（分页） | 全部 |
| 13.5 | 最近记录 | 分页、按成员筛选 | 全部 |
| 13.6 | 我的兑换 | 商城、余额校验、确认兑换 | 全部 |
| 13.7 | **我的券** | 未使用 / 待核销 / 已核销三态；**点击使用**发起核销投票 | 全部 |
| 13.8 | 赠送积分 | 选受赠成员、金额与附言、赠送历史 | 全部 |
| 13.9 | **任务** | 发布任务（分值、截止时间）、**接取**、放弃接取、提交、我的任务 | 全部 |
| 13.10 | **任务投票** | 待投票列表、同意/反对、实时票数（X/Y） | 全部（**发布者除外**） |
| 13.11 | 通知中心 | 未读列表、点击跳转、标记已读 | 全部 |
| 13.12 | 屏幕时间（底部 tab，在积分榜前） | 权限状态、总时长、**设备切换（全部/单台）**、每小时（可点看详情）、近 7 天、**其他成员详情（点卡片进入）**、上报状态 | 全部 |
| 13.13 | **个人设置**（点「我的」头像进入） | 改**昵称**（`username` 只读展示、永不可改）、选头像（预设 emoji）、改密码 | 全部 |
| 13.14 | **家庭管理**（2026-11-06 由「成员管理」更名） | **改家庭组名**、**家庭码展示 + 隐藏开关**、**按用户名发待确认邀请**、**本组已发邀请列表 / 取消**、**待审批入组申请 / 同意 / 拒绝**、踢出成员 | **仅组长** |
| 13.15 | **睡觉规则配置** | 睡觉时间点、允许时长、连续奖励档位 | **仅组长** |
| 13.16 | **商品管理** | 商品增删改 | **仅组长** |
| 13.17 | **设备管理**（2026-11-07） | 绑定设备列表（标记**本机**、绑定/最近上报时间）、解除绑定（组长直接 / 成员发申请）、撤回申请；**组长另有本组待审批解绑申请** | 全部 |

**共 17 个界面**，其中 3 个（13.14–13.16）仅组长可见。

> **未入组态（2026-11-06 改版）**：`family_id` 为空时保留完整底部 tab 外壳（屏幕/积分榜/任务/商城
> 四个 tab 显示「去加入家庭」占位，不打需要家庭上下文的接口；「我的」照常可用），
> 「所在家庭」进入 **FamilyScreen**（不占界面编号，是 13.1 启动分流后的一个状态分支）：
> - **搜索家庭**：输入 8 位家庭码 → 搜索（隐藏的码提示「不存在或已隐藏」）→ 申请加入（需组长审批）；
> - **我的申请**：本人发出的申请与状态（pending / accepted / rejected）；
> - **创建家庭**：建组者成为组长，自动生成家庭码。
> 已入组时 FamilyScreen 显示家庭名称、家庭码（可复制 + 隐藏状态 Pill）、成员列表（含积分）、
> 退出家庭（组长改为「需先转让」说明卡）。
> 入组/建组成功后客户端必须重新拉 `/api/app/me`、刷新本地 `familyId` 并切到主界面。
> 查收邀请走 `/api/app/invitations`、查申请走 `/api/app/family/requests/mine`（**都不要求已入组**）。

> **导航改版（2026-11-05）**：底部 tab 改为 **屏幕 / 积分榜 / 任务 / 商城 / 我的**，
> 屏幕时间提到第一位并作为独立 tab；**通知从 tab 移入「我的」**（带未读角标）。
> 未来升级还会加 2 个界面（全服任务、家庭星榜，见 25 节），当前**不做**。

> **组长的管理能力放在前台**，不需要登录 Web 后台——这是本次权限重划的核心：
> 组长管「家庭内的事务」（成员、睡觉规则、商品），管理员管「系统与安全」（设备、审计、家庭组）。

> **原型（2026-11-04 更新）**：两份，都在 `design/` 下，浏览器直接打开：
> - [`android-app.html`](../design/android-app.html) —— **可交互版**（推荐）：点按钮真的会变，
>   登录/兑换/投票/赠送全流程走内存态，含商品券核销
> - [`android-prototype.html`](../design/android-prototype.html) —— 静态设计板（15 屏，早于商品券）
>
> 原型里的字段与本节、第 3 节表结构一一对应，**不是自由发挥的视觉稿**——
> 若原型与本文档冲突，以本文档为准并修原型。

---

## 14. Web 管理后台

> **仅管理员使用**（`admins` 账号）。不提供成员只读视图——成员看数据请用 Android。

| # | 页面 | 对应功能 |
|---|---|---|
| 14.0 | **总览** | 全站计数（家庭组 / 成员 / 记录）+ 家庭组列表（含**隐藏状态标记**），点「进入」切当前组 |
| 14.1 | 登录 | **仅登录，无注册入口**；管理员由部署时环境变量初始化（见 5.4） |
| 14.1a | **修改初始密码** | 首次登录强制改密（5.4.2）；未改密前其余接口全 403，故必须单独有页 |
| 14.2 | 成员管理 | 查看成员 `username`（不可改）/ `nickname` 与头像（**仅预设**，改资料在 App 端 22.2）、隐藏、设组长、踢出、**按用户名直接拉入组**（15.2.3；后台管理动作，无确认环节） |
| 14.3 | 睡觉规则 | 睡觉时间点、允许时长、连续奖励档位（与组长同权） |
| 14.4 | 任务管理 | **只读**：查看全部任务与提交；审核走组内全员投票，后台**没有代审核接口** |
| 14.5 | 兑换商城 | 商品管理；兑换撤销已按 9.4 移除 |
| 14.6 | 赠送管理 | **只读**赠送记录；撤销赠送已按 10.2.6 移除 |
| 14.7 | 记录 | **只读**：分页、筛选、搜索、导出 CSV（**无删除**） |
| 14.8 | 统计 | 积分榜、得分榜（周/月）、积分趋势图 |
| 14.9 | 审计日志 | 按成员/动作/时间筛选，查看 before/after |
| 14.10 | 屏幕时间 | 查看上报历史 + **设备原始上报明细 / 已登记设备**（12.6；**只读，无手动录入**） |
| 14.12 | 家庭组设置 | 组名改名、家庭码只读展示（**含隐藏状态**）、**封禁名单（只读）**、**全部邀请与凭码申请（只读）**（15.2.38/39）、**解散家庭组**（15.2.40，危险区，输入组名确认）；**移交管理员不在本期范围**（管理员是运营方账号，与家庭组无归属关系） |
| 14.13 | **管理员账号** | 列表（显示 `nickname`）、新建（首次登录强制改密）、停用（5.4.3）、**改我的昵称**（5.4.9） |

**共 14 个页面**（12 个功能页 + 登录页 + 改密页）。设备管理页已随「去掉设备令牌」删除。相比早期版本删除了「记一笔」页与「手动录入」入口。

> **实现状态（2026-11-06 核对）**：`web/`（Vue 3 + Vite）共 14 个页面（`src/pages/` 14 个 .vue，路由 **15** 条含 redirect；
> 口径坑：登录守卫 `beforeEach` 里也有一处 `path: '/login'`，用 `grep -c 'path:'` 会多数一条），
> 直接对接 `/api/admin/*`。三处**有意只读**，不提供写操作：
> **任务管理**（代审核会绕开「一人一票」，不做）、**赠送管理**（撤销已按 10.2.6 移除）、
> **屏幕时间**（12 节：无手动录入入口）。记录与审计同样只读，记录是 append-only。
>
> 启动：`cd web && npm install && npm run dev`（`/api` 已代理到 `127.0.0.1:8080`）。
>
> **逐页点检（2026-11-04）**：用 Edge 无头浏览器真实登录并遍历全部页面，
> 全量页面零控制台错误、零失败请求。过程中修掉 4 个真问题（详见 `.memory/2026-11-04.md`）：
> ① 缺改密页导致默认管理员**登录后死锁**；② `.pad` 只定义了 `.card.pad`，
> 16 处卡片内区块全部贴边；③ 任务页用不存在的 `status==='done'` 统计「已完成」，
> 实际要用 `completed_by` 区分已完成/已关闭；④ 按钮在 flex 行里被压成竖排单字。

---

## 15. 接口清单

响应统一 `{ ok: bool, data?: any, error?: string }`。
**两套前缀，两套鉴权，互不通用**：

- `/api/admin/*` —— 仅接受 `typ='admin'` 的 token
- `/api/app/*` —— 仅接受 `typ='member'` 的 token（组长与成员同池，靠 `role` 区分）
- ~~`/api/device/*`~~ —— **已删除**（2026-11-05）；上报改走 `/api/app/screen/report`，用成员登录态

### 15.1 公开接口（无需鉴权）

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.1.1 | `/api/register` | POST | 注册账号（`username` / `password` / `nickname`，昵称可省略，缺省回填为用户名）；可传 `createFamily` 顺带创建家庭组（**只成为组长，不产生后台身份**） |
| 15.1.2 | `/api/admin/login` | POST | 后台登录 |
| 15.1.3 | `/api/app/login` | POST | 前台登录 |
| 15.1.3a | `/api/app/logout` | POST | 前台登出（吊销该成员全部 refresh；需 access，不要求已入组） |
| 15.1.4 | `/api/admin/refresh` | POST | 后台 token 刷新 |
| 15.1.5 | `/api/app/refresh` | POST | 前台 token 刷新 |

### 15.2 后台接口（`typ='admin'`）

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.2.1 | `/api/admin/password` | POST | 改密码 |
| 15.2.1a | `/api/admin/logout` | POST | 后台登出（吊销该管理员全部 refresh） |
| 15.2.3 | `/api/admin/family/:family_id/invite` | POST | 按用户名把**已注册且未入组**的用户拉进该组（14.2） |
| 15.2.5 | `/api/admin/family/:family_id/member/kick` | POST | 踢出成员（组长不可直接踢，须先换组长） |
| 15.2.22 | `/api/admin/family/:family_id/member/hidden` | POST | 隐藏 / 显示成员 |
| 15.2.23 | `/api/admin/family/:family_id/member/leader` | POST | 后台指定新组长（同事务降旧升新） |
| 15.2.24 | `/api/admin/family/:family_id/rename` | POST | 改家庭组名 |
| 15.2.29 | `/api/admin/family/:family_id/rule` | GET | 本组睡觉规则（未配置返回 `null`） |
| 15.2.30 | `/api/admin/family/:family_id/items` | GET | 本组商品列表 |
| 15.2.31 | `/api/admin/family/:family_id/tasks` | GET | 本组全部任务（含已关闭，顺手把超时提交置 `expired`） |
| 15.2.32 | `/api/admin/family/:family_id/transfers` | GET | 赠送记录（`limit`，默认 50） |
| 15.2.33 | `/api/admin/family/:family_id/screen-reports` | GET | 屏幕时间上报历史（只读，`member_id` / `limit`） |
| 15.2.34 | `/api/admin/family/:family_id/stats/ranking` | GET | 积分榜 / 得分榜（`mode=points\|score`、`range=week\|month`） |
| 15.2.35 | `/api/admin/family/:family_id/stats/trend` | GET | 按天趋势（`days` 默认 30、`member_id` 0=全组） |
| 15.2.36 | `/api/admin/family/:family_id/records` | GET | 本组流水（只读、分页、`member_id` / `source` / `keyword` / `from` / `to`） |
| 15.2.37 | `/api/admin/family/:family_id/screen-device-reports` | GET | 设备原始上报 + 已登记设备（只读，`date` / `member_id` 0=全组 / `limit`；见 12.6） |
| 15.2.38 | `/api/admin/family/:family_id/bans` | GET | **封禁名单（只读）**：成员 + 原因（kicked/left）+ 时间。**不提供解除接口**——唯一解封路径是组长邀请入组时自动清除（5.3.4） |
| 15.2.39 | `/api/admin/family/:family_id/invitations` | GET | **全部邀请与凭码申请（只读）**：两个方向、含已处理 / 已过期（`limit` 默认 50，≤200） |
| 15.2.40 | `/api/admin/family/:family_id/dissolve` | POST | **解散家庭组**（5.5.2，高危）：`confirm_name` 必须与当前组名逐字一致，否则 400。事务内级联——成员积分清零 + `family_id` 置 NULL + 组长身份收回；删除 families / 睡觉规则 / 屏幕规则 / 商品 / 禁入名单；未完结任务关闭、pending 邀请与解绑申请作废；**流水 / 上报 / 审计 / 通知保留**（family_id 悬空属预期）。解散前给全组发通知（注意：成员解散后未入组前读不到组内通知，见 15.3.46 已知限制） |
| 15.2.6 | `/api/admin/family/:family_id/rule/save` | POST | 睡觉规则与连续奖励档位（与组长同权） |
| 15.2.7 | `/api/admin/family/:family_id/item/save` | POST | 商品增改（与组长同权；撤销兑换已按 9.4 移除） |
| 15.2.9 | `/api/admin/audit/list` | GET | 审计日志查询 |
| 15.2.10 | `/api/admin/export/records` | GET | 导出 CSV |
| 15.2.12 | `/api/admin/state` | GET | 后台初始化数据 |
| 15.2.13 | `/api/admin/accounts` · `/api/admin/accounts/disable` | GET/POST | 管理员账号列表 / 新增 / 停用（不能停用自己） |
| 15.2.14 | `/api/admin/families` | GET | 所有家庭组列表（运营方视角） |
| 15.2.15 | `/api/admin/family/:family_id` | GET | 跨组查看单个家庭组（**family_id 只能走路径参数**） |
| 15.2.16 | `/api/admin/family/:family_id/members` | GET | 跨组查看成员（含被隐藏的） |
| 15.2.19 | `/api/admin/me` | GET | 当前管理员信息（含 `username` / `nickname` / `must_change_password`） |
| 15.2.19a | `/api/admin/me/profile` | POST | **只改当前管理员的昵称**（5.4.9）；请求体带非空 `username` 直接 400（用户名永不可改） |

> **已移除**：`/api/admin/score/*`（记一笔）、`/api/admin/screen/report-manual`（手动录入）。
> 接口层**直接删除，返回 404**，不是隐藏。
>
> **接口清单清理（2026-11-06）**：以下旧编号不再登记路径，避免与路由对账时产生「文档有、代码无」：
>
> | 旧编号 | 去向 |
> |---|---|
> | 15.2.2 `/api/admin/family` | 早期聚合写法；已拆为 15.2.15（查看）与 15.2.24（改名） |
> | 15.2.4 `/api/admin/family/transfer-admin` | **移交管理员不在本期范围**（14.12）：管理员是运营方账号，与家庭组无归属关系；跨组指定组长走 15.2.23 |
> | 15.2.8 `/api/admin/task/*` | 代审核会绕开「一人一票」，**明确不做**（14.4）；后台只有只读的 15.2.31 |
> | 15.2.11 `/api/admin/device/*` | 设备令牌机制 2026-11-05 **整体删除**，上报改走 15.4 的成员登录态 |
> | 15.3.22 `/api/app/member/avatar` | 已并入 15.3.42 `/api/app/me/profile`（名称 + 预设头像标识一次提交） |
> | 15.3.32 `/api/app/member/invite` | 早期写成「组长直接拉入组」，该语义**明确不做**；P3.1 以「待确认邀请」新语义重新登记为 **15.3.43**（5.2.3）。旧编号作废，后台直拉仍是 15.2.3 |
> | 15.1.6 `/api/app/family/join` | 家庭码体系（2026-11-07）下**凭码直接入组绕过审批，已整体删除**；入组只走两条路：待确认邀请（15.3.43/47）与凭码申请审批（15.3.55/58） |
> | 15.3.34 `/api/app/family/invite-code` | 邀请码改为**不可变家庭码**：查看重新登记为 15.3.52（`GET /family/code`），重置能力**取消**（家庭码永不重置、无有效期） |
>
> 原则：**未实现的功能留在功能章节写清状态，不在接口清单里写死路径**。

### 15.3 前台接口（`typ='member'`）

**所有成员可用**

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.3.1 | `/api/app/password` | POST | 改自己的密码 |
| 15.3.2 | `/api/app/state` | GET | 本组概览（不含全量记录）；含 `transfer_max`（单笔可赠送上限，后端给） |
| 15.3.3 | `/api/app/records` | GET | 记录分页 / 筛选 / 搜索（**只读**） |
| 15.3.4 | `/api/app/item/list` | GET | 商城列表（含 `affordable`：后端按当前可用积分算的「能不能换」） |
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
| 15.3.21 | `/api/app/stats/trend` | GET | 趋势图（Android 积分榜页 13.3 画 earned / net 双线） |
| 15.3.23 | `/api/app/family/leave` | POST | 自行退出家庭组（积分清零 + 写入 `family_bans`） |
| 15.3.24 | `/api/app/family` | GET | 本组信息（**family_id 取自 token**，不接受任何参数） |
| 15.3.25 | `/api/app/family/members` | GET | 本组成员（过滤被隐藏的） |
| 15.3.26 | `/api/app/family/rename` | POST | 改家庭组名（**组长**） |
| 15.3.27 | `/api/app/member/rename` | POST | 改昵称（本人或**组长**） |
| 15.3.28 | `/api/app/records` | GET | 本组流水分页（`limit` / `offset`） |
| 15.3.29 | `/api/app/me` | GET | 当前成员信息（`score` / `frozen_score` / `role` / `family_id`） |
| 15.3.30 | `/api/app/screen/reports` | GET | 结算历史（本人；组长可查他人） |
| 15.3.49 | `/api/app/screen/snapshot` | POST | **今日实时快照**：落设备 + 当日合计 + 24 小时桶，**不结算、不发分**（12.7） |
| 15.3.50 | `/api/app/screen/member/:member_id` | GET | **某成员某天明细**：总时长 / 24 桶 / 各设备（含各自 24 桶与近 7 天）/ 近 7 天；`member_id=0` 为自己（12.7） |
| 15.3.31 | `/api/app/task/detail` | GET | 任务详情：提交列表 + 每条的票数与票 |
| 15.3.46 | `/api/app/invitations` | GET | **我收到的邀请**（待确认 + 历史处理记录）。**挂在 `app` 组、`family_id` 为空也能调**（5.2.8），不经过 `RequireFamily()` |
| 15.3.47 | `/api/app/invitation/accept` | POST | **接受邀请**（`invitation_id`）：同事务置 `accepted` + 走 `joinTx` 入组 + 取消本人其它 pending；返回邀请对象 |
| 15.3.48 | `/api/app/invitation/reject` | POST | **拒绝邀请**（`invitation_id`）：置 `rejected` + `decided_at`；返回邀请对象 |
| 15.3.53 | `/api/app/family/search` | GET | **按家庭码搜索家庭**（需未入组；`?code=`）。隐藏的码返回 `found=false`，与不存在不可区分 |
| 15.3.54 | `/api/app/family/create` | POST | **未入组者建组**（`name`），创建者成为组长（只产生前台组长身份） |
| 15.3.55 | `/api/app/family/request` | POST | **凭家庭码申请加入**（`code`）：隐藏的码一并拒绝；同组已有 pending 返回 **409**；被禁入返回 **403** |
| 15.3.56 | `/api/app/family/requests/mine` | GET | **我发出的入组申请**（需未入组也能调，挂 `app` 组） |

**仅组长（`role='leader'`）**

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.3.33 | `/api/app/member/remove` | POST | 踢出成员（积分清零 + 写入 `family_bans`；**需 `confirm=true`**） |
| 15.3.52 | `/api/app/family/code` | GET | **查看本组家庭码** + 隐藏状态（**全员可读**，不限于组长） |
| 15.3.34 | `/api/app/family/code/hidden` | POST | 组长设置家庭码是否可被搜到（`hidden`）。**隐藏 = 搜不到 + 凭码申请也拒绝**（15.3.53 / 15.3.55 都等价于码不存在） |
| 15.3.35 | `/api/app/rule` · `/api/app/rule/save` | GET/POST | 睡觉规则与连续奖励档位（保存限**组长**） |
| 15.3.36 | `/api/app/item/save` · `/api/app/item/delete` | POST | 商品增改 / 删除（**组长**） |
| 15.3.37 | `/api/app/member/hidden` | POST | 隐藏 / 显示成员 |
| 15.3.38 | `/api/app/family/transfer-leader` | POST | **组长转让**（同事务降旧升新） |
| 15.3.39 | `/api/app/screen-rule` · `/api/app/screen-rule/save` | GET/POST | 每成员屏幕时间规则（保存限**组长**） |
| 15.3.40 | `/api/app/coupons/pending` | GET | 本组待我核销的券（投票入口） |
| 15.3.41 | `/api/app/coupon/detail` | GET | 券详情 + 核销投票明细（`coupon_id`） |
| 15.3.42 | `/api/app/me/profile` | POST | 改自己的资料（**昵称** + **预设**头像标识；不接受 URL，不接受 `username`） |
| 15.3.43 | `/api/app/member/invite` | POST | **按用户名发起待确认邀请**（5.2.3，`username`）；不直接入组；同组同一人已有 pending 返回 **409** |
| 15.3.44 | `/api/app/member/invitations` | GET | 本组已发出的邀请（含被邀请人昵称 / 状态 / 有效期）；过期 pending 不显示且不落库 |
| 15.3.45 | `/api/app/member/invite/cancel` | POST | 取消本组一条 pending 邀请（`invitation_id`）；非本组或非 pending 一律 **404** |
| 15.3.57 | `/api/app/family/requests` | GET | 本组待审批的入组申请（`direction='request'` 的 pending） |
| 15.3.58 | `/api/app/family/request/decide` | POST | **审批入组申请**（`invitation_id` + `accept`）：通过时同事务走 `joinTx` 入组，与邀请接受共用同一段 SQL |

### 15.4 屏幕上报接口（成员登录态，2026-11-05 起无需令牌）

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.4.1 | `/api/app/screen/report` | POST | 上报当天累计时长（`total_min`）与夜间时长（`night_min`）+ `device_key` / `device_model`；**按设备后报覆盖**，`report_date` 缺省按归属日推算 |
| 15.4.2 | `/api/app/screen/family` | GET | 全组某天屏幕时间：每人汇总 + 各设备明细（`?date=YYYY-MM-DD`） |
| 15.4.3 | `/api/app/screen/devices` | GET | 本人上报过的设备列表 |

### 15.5 设备绑定与解绑（2026-11-07）

> **绑定语义**：一台设备（`device_key`）全局只能绑定一个账号（`member_devices.device_key` 全局唯一）。
> 上报时未绑定 → **自动绑定到当前登录账号**；已绑定他人 → **409「该设备已绑定其他账号」**，
> 换绑必须先解绑。解绑后设备可在下次上报时绑定到当时登录的账号。
> **解绑语义**：组长可直接解绑本组任何设备（含自己的）；普通成员解绑自己的设备需**发起申请 → 组长审批**
> （一人同时最多一条 pending）。设备被组长直接解绑时，其 pending 申请自动落为 approved。
> 上报仍全部走 15.4.1 / 15.4.2（携带 device_key 时执行绑定检查）；15.4.3 的设备列表即「我的已绑定设备」。

| # | 接口 | 方法 | 说明 |
|---|---|---|---|
| 15.5.1 | `/api/app/device/unbind/request` | POST | **成员发起解绑申请**（`device_key`，须绑定在自己名下）；一人一条 pending（409）；组长收到 `unbind_requested` 通知 |
| 15.5.2 | `/api/app/device/unbind` | POST | **组长直接解绑**（`device_key`，本组任何成员的设备）；成员调用 403；被解绑者的 pending 自动 approved |
| 15.5.3 | `/api/app/device/unbind/mine` | GET | 我发出的解绑申请（含已处理，最新在前） |
| 15.5.4 | `/api/app/device/unbind/requests` | GET | 本组待审批的解绑申请（**组长**） |
| 15.5.5 | `/api/app/device/unbind/decide` | POST | 组长审批（`request_id` + `approve`）；**同意即立即解绑**；结果通知申请人（`unbind_decided`） |
| 15.5.6 | `/api/app/device/unbind/cancel` | POST | 成员撤回自己的 pending 申请（`request_id`） |

> **越权测试基线**：前台 token 打任一 `/api/admin/*` 必须 401；
> 后台 token 打任一 `/api/app/*` 必须 401；
> 普通成员打 15.3.21–15.3.25（组长专属）必须 403；
> 发布者调用投票接口必须 403；非本组成员投票必须 404。

---

## 16. 部署方案——**2026-11-07 已部署（仍是开发阶段，非生产上线）**（见 16.5 实录）

> **2026-11-07 用户明确：当前仍是开发阶段，服务器部署只是给开发/体验用的环境，不是生产。**
> 因此：① 线上库（aabbcc）可随时 `migrate -reset`，丢数据不算事故；② 16.2/16.3 的
> 加固项（备份、防火墙、证书、密钥轮换）是**生产化 checklist**，进入生产前再逐项补齐，
> 不阻塞当前开发；③ 入口 `https://fam.airiru.site`（阿里 CDN 回源本机 80）。

2026-11-07 首次部署实录：**未用 Docker**（服务器是 960MB 小内存的 Debian 12 面板机，
nginx/MySQL 已由面板管理，直接复用）。当前拓扑与 16.1 一致。

### 16.1 拓扑

```
公网 80/443 → Nginx → (静态 / → Web dist) + (/api → Go :8080) → MySQL :3306
```

**只暴露 80/443**，Go 与 MySQL 仅监听 `127.0.0.1`。

### 16.2 组件清单

> 16.2.1 修订：实际部署**不用 Docker Compose**——直接 systemd + 复用面板的 nginx/MySQL
> （小内存机器再起一套容器化 MySQL 不现实）。16.2.5 / 16.2.6 / 16.2.9 / 16.2.10 已随上线完成。

| # | 事项 | 状态 |
|---|---|---|
| 16.2.1 | ~~Docker Compose~~ → **systemd 单进程**（`familypoints.service`，ExecStart=/opt/familypoints/bin/api，MemoryMax=256M） | ✅ 2026-11-07 |
| 16.2.2 | Nginx 配置（fam.airiru.site vhost：SPA 静态 + `/api` 反代 127.0.0.1:8080 + `/healthz` 直通；复用面板 ssl.conf） | ✅ 2026-11-07 |
| 16.2.3 | HTTPS 证书 | ✅ 由阿里 CDN 终结（源站复用面板自签证书，2058 年到期）；Let's Encrypt 待域名直连时再换 |
| 16.2.4 | 数据库迁移 | ✅ 独立步骤：`bin/migrate`（-reset/-seed-admin/-verify）；**注意 5.7 兼容**：建库排序规则用 utf8mb4_general_ci，库已存在时跳过 |
| 16.2.5 | 环境变量管理 | ✅ `/opt/familypoints/.env`（600），DB 复用面板 MySQL 的 `aabbcc` 库 |
| 16.2.6 | 健康检查接口 `/healthz` | ✅（上线首日即有） |
| 16.2.7 | 日志落盘与轮转 | ✅ journald（systemd 托管，`journalctl -u familypoints`） |
| 16.2.8 | 防火墙（仅开 22/80/443） | ⚠️ 面板机自带 14623 SSH 端口；80/443/3306 已对外——3306 对外属面板既有风险，待加固 |
| 16.2.9 | **初始化默认管理员账号** | ✅ `-seed-admin` 已跑；凭据在服务器 `/root/.fp_admin_password`（600） |
| 16.2.10 | **首次登录强制改密** | ✅（上线验收时实测走通：改密 → 重登） |

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

### 16.5 部署实录（2026-11-07，备忘；开发阶段环境，非生产）

| 项 | 值 |
|---|---|
| 服务器 | Debian 12 / x86_64 / 2C / **960MB 内存** / 40G 盘（kvm-airiru，SSH 14623） |
| 已有服务 | 面板（/xp/panel，占 39280）、nginx（/xp/server/nginx，80/443）、MySQL 5.7.43（/xp/server/mysql，3306 **对公网开放**） |
| 部署目录 | `/opt/familypoints/`（bin/api、bin/migrate、web/、.env） |
| 数据库 | 复用面板 MySQL，库 `aabbcc`（用户 aabbcc 仅有此库权限，migrate 据此跳过建库） |
| 入口 | `https://fam.airiru.site`（DNS→阿里 CDN 163.181.214.24，回源本机 80；主站 vhost 的 server_name 已摘除 fam） |
| 备份 | `/xp/panel/vhost/nginx/airiru.site.conf.bak-*` |
| 待加固（**生产化 checklist，进入生产前补齐**） | MySQL 3306 公网暴露、私钥已入对话需轮换、备份 16.3 全表 ⬜ |
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
| 19.5 | 服务器规格与操作系统 | ✅ 2026-11-07：Debian 12 / 2C / 960MB（见 16.5）；因此放弃 Docker 改 systemd |
| 19.6 | 是否公开注册 | **先按「开放注册」实现**（注册语义见 5.1）。限流与验证码**不实现**，但路由层要留中间件挂载点，日后加一行即可 |
| 19.7 | 域名与备案 | ✅ 2026-11-07：fam.airiru.site（阿里 CDN，源站 kvm-airiru） |

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
> - **19.22（邀请码有效期）** —— 2026-11-02 拍板「7 天」；**2026-11-06 随家庭码体系废除**：
>   家庭码 8 位、唯一、**永不可更改、不过期、不可重置**；组长只能切换「是否可被搜到」。
>   凭码加入必须经组长审批（5.2.2a），隐藏 = 搜不到 + 凭码申请也拒绝（2026-11-07 定稿）。
>   详见 5.2、15.3.52–58。
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

**以 `server/internal/handler/*` 里的 `recordAudit(...)` 调用为唯一事实源**（2026-11-06 核对）：

| 分类 | 实际写入的 action |
|---|---|
| 管理员账号 | `admin.create` · `admin.disable` |
| 家庭组 / 成员 | `member.invite` · `member.kick` · `member.leave` · `member.hidden` · `leader.transfer` · `family.rename` |
| 待确认邀请（P3.1） | `invite.create` · `invite.accept` · `invite.reject` · `invite.cancel` |
| 规则 | `rule.save` · `screen_rule.save` |
| 商品 / 积分流转 | `item.save` · `item.delete` · `item.redeem` · `transfer.send` |
| 任务 | `task.create` · `task.close` · `task.submit` · `task.vote` |
| 导出 | `records.export` |
| 认证 / 管理员资料 | `auth.login` · `auth.password`（2026-11-06 补齐） · `auth.profile`（P3.1：管理员改昵称） |

> 调用约定：**业务成功后才写**，失败路径不记；写入失败由 `audit.Writer` 吞掉，不影响主流程。
>
> **待确认邀请的审计口径（P3.1）**：`invite.create` / `invite.cancel` 是组长在组内操作，
> 直接走 `recordAudit` 自动取主体；`invite.accept` / `invite.reject` 时主体**还没入组**（JWT 里
> `family_id` 为 NULL），必须用 `recordAuditAs` 显式传入**服务端邀请行**的 `family_id`，
> 不能从请求体取，也不能留空。这是对 21.4 第 4/5 条的补充而非放宽。
>
> **认证审计的口径（2026-11-06）**：只记**成功**事件——失败登录不改任何状态，按上面的调用约定不记。
> `before` / `after` 只放 `username` 与 `relogin_required` 标记，**绝不写明文密码或哈希**。
> 登录是公开接口、JWT 中间件还没注入主体，由 handler 用刚认证通过的结果经
> `recordAuditAs` **显式传主体**，`family_id` 取服务端查到的归属（管理员为 `NULL`）；
> 改密是受保护接口，直接走 `recordAudit` 自动取主体。
> 管理员 `operator_type=system`，成员 `operator_type=member`。
> `before` 只有少数写入点提供（如 `member.leave` / App 侧 `leader.transfer` 的旧值），
> 其余多为 `after` 快照——需要完整前后对比时以 `audit_logs` 里实际存的 JSON 为准。
>
> **早期规划但不存在**的 action（勿再引用）：`record.delete` / `member.reset`（记录 append-only、
> 积分清零发生在 leave/kick 流程里）、`item.revoke` / `transfer.revoke`（撤销能力已移除）、
> `task.review`（审核走组内全员投票，见 14.4）、`device.create` / `device.revoke`
> （设备令牌 2026-11-05 整体删除）。
> **`auth.login` / `auth.password` 已于 2026-11-06 实现**，移入上表「认证」行，不再是缺口。

### 21.3 功能项

| # | 功能 | 状态 |
|---|---|---|
| 21.3.1 | 全量写入审计日志 | ✅ 已接入：兑换/赠送/任务(发布·提交·投票·关闭)/成员(踢出·退出·隐藏·转让)/规则/商品/改组名/登录·改密/邀请(发起·接受·拒绝·取消)（设备令牌已整体删除，无此事件） |
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
5. **公开接口（登录 / 注册）不得从请求体取主体**：中间件此时没有注入，必须用
   `recordAuditAs` 传入**服务端认证通过**的主体，否则会写出 `operator_id` 为空的
   日志，甚至允许伪造操作者。

---

## 22. 站内通知与成员头像

### 22.1 站内通知

| # | 功能 | 状态 |
|---|---|---|
| 22.1.1 | 通知生成（任务待审 / 审核结果 / 兑换成功 / 收到赠送 / 收到邀请 / 邀请被处理） | ✅ 全部已实现：`task_submitted` · `task_decided` · `coupon_pending` · `coupon_decided` · `transfer_in` · `redeem_success` · `kicked` · `invite_received` · `invite_decided`（2026-11-06 补齐兑换成功与 P3.1 邀请通知） |
| 22.1.2 | 未读数角标 | ✅ 由 `/api/app/state` 的 `unread_count` 提供 |
| 22.1.3 | 通知列表与标记已读 | ✅ 列表（含 `unread=1` 过滤）、单条/全部已读 |
| 22.1.4 | 点击跳转到对应对象 | ✅ 通知带 `ref_type` / `ref_id`，跳转由客户端实现 |
| 22.1.5 | 系统推送（FCM / 厂商通道） | ⛔ 明确不做：国内需接多家通道，成本远高于收益 |

> **不做推送的取舍**：Android 端靠「打开 App 时拉取未读」即可满足家庭场景。
> 若日后确有实时性需求，再评估厂商通道。
>
> **单一写入路径（2026-11-06）**：通知统一由 `internal/notify.Service` 从 **service 层**发出。
> 早期 `handler/notification.go` 里还有一条直写 repo 的 `notify(...)`，与 service 层
> 各发一条，导致**赠送 / 任务审核结果各推两条重复消息**（p8 查旧类型、p11 查新类型，
> 两边都绿但用户看到重复）。旧链路连同 `SetNotifier` / `notifier` / handler 里的 `itoa`
> 已整体删除；**新增通知必须在 `notify.Service` 加语义方法，不要回到 handler 直写 repo**。
> `redeem_success` 发给兑换者本人，`ref_type=redemption` / `ref_id` 为兑换单 ID
> （券与扣分同事务生成，repo 未在返回值里带 coupon ID，不为通知改 repo 返回结构）。
>
> **P3.1 邀请通知**：`invite_received` 只发给被邀请人、`invite_decided` 只发给发起邀请的组长，
> `ref_type=invitation` / `ref_id` 为邀请 ID；取消邀请**不发**通知。
> ⚠️ **已知限制**：通知列表 `/api/app/notification/list` 挂在 `inFamily` 组，
> 被邀请人**尚未入组时看不到** `invite_received`；未入组侧的可靠入口是 15.3.46 邀请列表
> 与 13 节的「我收到的邀请」卡片。该通知是入组后的历史留痕，不承担触达职责
> （不为此放开通知接口的鉴权——通知内容带 `family_id`，边界不能放宽）。

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
22. **家庭码永不过期、不可重置**；隐藏后搜索返回 `found=false`、凭码申请返回 400（等价于码不存在）。
23. 凭家庭码加入**必须经组长审批**：申请 → 组长同意 → 入组；同组已有 pending 返回 409。

**退出与踢出（19.2 已定）**
24. 成员被踢出后 `score` 与 `frozen_score` 均为 **0**。
25. 成员自行退出后 `score` 与 `frozen_score` 均为 **0**。
26. 被踢出 / 退出的成员**凭家庭码再次申请加入原组被拒绝**（403）；**组长按用户名邀请不受封禁限制**，且接受邀请入组时自动清除封禁。
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
104. 上报**不再需要设备令牌**：登录成员即可上报；未登录 401，缺 `device_key` 400。
105. **多设备相加后结算**：同一成员同一天两台设备（30 + 20）→ 结算行 `total_min = 50`，只产生 1 条结算行。
106. 家庭屏幕详情返回**全组成员**（没上报的也在，total=0），每人的 `devices` 列出各设备型号与时长。
107. 个人资料可改名称/头像；空名称+空头像 400，名称超 32 字 400。
108. 站内通知在业务节点真实产生：任务提交→其他成员收到 `task_submitted`；任务出结果→完成者收到 `task_decided`；
    券核销发起→其他成员收到 `coupon_pending`；核销出结果→申请人收到 `coupon_decided`；赠送到账→受赠方收到 `transfer_in`。

**2026-11-03 四项决策（19.11 / 19.13 / 19.14 / 19.15）**
105. **任务提交只有文字说明且必填**：请求体不接收图片字段；`task_submissions` 表无 `image_url` 列；
    `note` 为空时返回 400。
106. 同一任务同一成员第 3 次被驳回后**仍可提交一次**，第 4 次提交返回 403 并提示已达上限。
107. `expired` 的提交**不计入**重提上限（超时后仍可再提交）。
108. 审计日志中早于 180 天的记录被定时任务删除，边界日（第 181 天）保留。
109. 日志清理**分批执行**，单批不影响主流程写入。
110. 头像接口只接受**预设标识**；传 URL 或非法标识返回 400。
111. 服务器上**不存在头像上传目录**，`members.avatar` 中无任何 URL 形态的值。

**2026-11-05 夜间扣分（12.4（六））**
112. `night_min ≤ night_max_min` 时**不扣分**（正常加分，含连续档位）。
113. 单位 30、每单位 2 分、允许 0 分钟、夜间 100 分钟 → 扣 **8** 分（4 个单位，向上取整）。
114. 同上但单位改 60 → 扣 **4** 分（2 个单位）。
115. 夜间 1 分钟（不满一个单位）也按 **1 个单位**扣分。
116. `night_penalty_points = 0` 时**不扣分**，流水积分仍为 0（只不加分 + 连续清零）。
117. `night_penalty_unit_min` 传 45 → **400**；`night_penalty_points` 传负数 → **400**。
118. 保存规则**不传 `night_rule_id`** 时，后端自动取本组唯一规则；本组尚无规则 → **400** 且提示先配档位。

**2026-11-06 后台设备明细与趋势折线（12.6 / 13.3）**
119. `/api/admin/family/:family_id/screen-device-reports` 按路径里的 `family_id` 隔离；`date` 非 `YYYY-MM-DD` 返回 **400**；跨组数据不出现在结果里。
120. 同一成员同一天两台设备（30 / 20）在后台明细里是 **2 行**且各自 `total_min` 保持原值；结算行仍只有 **1 行**且为相加值 50。
121. 返回的 `devices` 列出该组已登记设备（含 `device_key` / `device_model` / `last_seen_at`）；当天没上报的设备仍在列表中。
122. Android 积分榜页的趋势 DTO 与后端对齐为 `date` / `earned` / `net`（旧 `label` / `value` 会静默取默认值）；周/月切换后曲线随服务端数据变化，空数据显示空态不崩溃。

**2026-11-06 余额可为负（8.4 / 12.1；p5 第 10 节）**
123. 屏幕时间自动扣分把 `score` 扣成**负数**：配置「阈值 0 / 超标扣 1000 / 不加分」后上报一次，余额 = 原余额 − 1000 且小于 0。
124. **余额已经是负数时继续扣分仍生效**：另起一天再超标上报，余额精确再减 1000，不截断、不拒绝、不归零。
125. 两次扣分各写一条**负分流水**（`source=screen`、`points=-1000`），不因余额为负而跳过写入（8.1 铁律）。
126. 负余额状态下**主动支出仍被拒绝**：赠送返回 400 且余额不变；兑换 / 发布任务冻结同理由事务内余额校验把关（8.4）。
127. 自动扣分路径（`Settle` / `rollbackReport`）**不得引入 `score >= 0` 护栏**；主动支出的三处 `FOR UPDATE` 后校验**必须保留**。

**2026-11-06 通知与认证审计（22.1 / 21.2；p8 第 8 节、p11 第 2/4 节）**
128. 兑换成功后**兑换者本人恰好收到 1 条** `redeem_success` 通知，组内其他成员不收；余额不足的失败兑换不产生通知。
129. 通知只有 `internal/notify.Service` 一条写入路径：赠送 / 任务审核结果不再产生旧的 `transfer` / `task_review`，每个事件恰好 1 条。
130. 成员登录成功后落 `auth.login`（`operator_type=member`，`operator_id` 与 `family_id` 均正确）；管理员登录落 `auth.login`（`operator_type=system`、`family_id` 为空）。
131. 改密成功后落 `auth.password`；登录 / 改密的**失败路径不写审计**。
132. 认证审计的 `before` / `after` 不含任何密码、哈希或 token 字段；审计写入失败不阻断登录与改密。

**2026-11-06 P3.1：username 不可改 / 昵称统一 / 按用户名待确认邀请（5.1.6 / 5.2.3 / 5.4.9 / 15.3.43–48；p12）**
133. `members` 与 `admins` 均有 `nickname`、`members.name` 已清除；`family_invitations.pending_invitee` 为 STORED 生成列且 `uk_pending_invitee` 存在；`go run ./cmd/migrate -verify` 全绿。
134. 注册不传 `nickname` 时回填为 `username`；注册 / `/api/app/me` / 成员列表 / 榜单的显示名一律是 `nickname`，响应里不再出现 `name`；`records.member_name` 列名保留且改名不污染历史快照。
135. `POST /api/app/me/profile`、`POST /api/app/member/rename`、`POST /api/admin/me/profile` 带 `username` 一律 **400**，库里 `username` 不变；管理员改昵称成功后 `/api/admin/me` 返回新昵称。
136. 发起邀请校验：用户名不存在 **400**、邀自己 **400**、邀已入组用户 **409**、普通成员发起 **403**；失败路径不写 `invite.create` 审计。
137. 正常发起返回裸邀请对象（`status=pending` + `family_name` + `invitee_nickname`），**不直接入组**；同组同一人重复发起 **409**，本组列表只有 1 条 pending。
138. 未入组用户调 `GET /api/app/invitations` 返回 **200**（挂 `app` 组而非 `inFamily`），只看到发给自己的邀请、带邀请人昵称；看不到别人的邀请；组内通知接口对未入组仍 **403**（22.1 已记限制）。
139. 拒绝：`status=rejected`、写 `decided_at`、仍未入组；重复拒绝 / 再接受同一邀请 **404**；组长列表显示 `rejected` 并收到 `invite_decided`。
140. 接受：`status=accepted`、`family_id` 变为该组、`role=member`；其它组发来的 pending 同事务置 `cancelled`，再接受 **404**；审计 `invite.accept` 的 `family_id` 取服务端邀请行而非 NULL。
141. 取消：组长可查本组已发邀请并取消 pending；重复取消 / 跨组取消 **404**；取消后对方看到的是 `cancelled` 历史行，不再是 pending。
142. 过期：把 `expires_at` 改到过去后，两条列表都不显示该 pending、接受返回 **400**「邀请已过期」；**同组再次邀请同一人仍成功**（写路径先把过期 pending 落为 `expired` 释放唯一键），旧行在组长列表显示 `expired`。
143. 审计 `invite.create` / `invite.accept` / `invite.reject` / `invite.cancel` 只在业务成功后写；通知 `invite_received` 只给被邀请人、`invite_decided` 只给发起组长，取消不发通知。
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
| **P3 家庭组与成员** | 创建组（成为组长，自动生成家庭码）、家庭码搜索 / 隐藏 / 凭码申请审批、按用户名邀请、退出/踢出（积分清零 + `family_bans`）、组长变更 | 19.2 相关条目、5.2 / 5.3、p3 / p12 |
| **P4 账目核心** | 记录写入、`score`/`frozen_score`、兑换（`FOR UPDATE`）、赠送（按 `members.id` 升序加锁）、撤销 | 积分不变量组、并发与事务组（83） |
| **P5 睡觉规则 + 屏幕时间** | `screen_rules` 配置、上报、夜间判定、连续奖励档位（`repeat`/`once`）、**覆盖回滚两条记录**、补算 | 64–72、18.3 的 12.x 条目 |
| **P6 任务与投票** | 发布冻结、公开接取、提交（`note` 必填）、全员投票过半、驳回 3 次上限、超时 `expired` | 48–63、95–97 |
| **P7 查询与榜单** | `/api/app/state` 分页（**不返回全量记录**）、记录筛选、积分榜 / 得分榜（`occurred_at` 聚合） | 74–82 |
| **P8 后台 + 审计 + 通知** | 管理员全量接口、审计 append-only（剔除 `pass_hash`、失败不阻断）、180 天清理、站内通知、预设头像 | 84–86、98–101 |
| **P9 客户端** | Android **16** 界面（3 个组长专属）+ 商品券核销 ✅、Web 后台 14 页面 ✅ | 92–94 及全部界面条目 |

**当前进度（2026-11-03）**：**P0 ✅ / P1 ✅ / P2 ✅ / P3 ✅ / P4 ✅ / P5 ✅ / P6 ✅ / P7 ✅**
（P1 19、P2 15、P3 33、P4 23、P5 37、P6 34、P7 25、P8a 19 项接口测试 + 计分单测全通过，
脚本见 `server/scripts/`）。
**P8 拆成两半**：P8a（审计 + 通知）✅、P8b（后台管理接口 + CSV 导出）✅ 均已完成
（P8a 19 项、P8b 36 项）。服务端 P0–P8 全部完成。
**P9 已完成（2026-11-04）**：Android 客户端（`android/`，16 个界面，含设备自动上报与夜间时长采集）
**已真实构建通过** —— `./gradlew assembleDebug` → `app-debug.apk` 19 MB（JDK 17 + Gradle 8.9）；
商品券与核销投票（9.6）前后端均已落地；Web 后台 14 个页面已上线（`web/`）。
数据库已按「开发阶段不兼容旧数据」重建，迁移 squash 为单一 `0001_init`；
**表清单以 `go run ./cmd/migrate -verify` 为唯一事实源**（该命令会逐项核对生成列与唯一键），不在正文写死表数。
**真机验证（2026-11-04）**：在 HUAWEI ANA-AN00（Android 12 / API 31）上 `adb install` 成功，
登录、积分榜（4 名成员真实余额与冻结额）、任务、商城（余额不足置灰 + 我的券入口）、
我的（组长工具按角色显示）**逐屏截图核对通过**；链路走 `adb reverse tcp:8080`。
**屏幕时间链路已真机验证（2026-11-04）**：`UsageStatsManager` 采集 → Worker 上报 → 服务端结算
全跑通（上报 200、`revision` 1→2→3 覆盖重算、夜间判定 `night_ok=true`、连续 3 天命中 +5 档位）。
**仍未验证**：token 自动刷新、券核销投票全链路。
**未来升级（暂不实现）**：全服任务与家庭星已设计、暂缓（见 25 节），排在 P9 之后再说。
**撤销兑换 / 撤销赠送已按用户要求移除**（2026-11-03），接口返回 404，脚本里有断言兜住。
**P3.1 已完成（2026-11-06）**：按用户名「待确认邀请」（5.2.3）落地，含 `family_invitations` 状态机、
被邀请人确认 / 拒绝、7 天惰性过期与唯一键释放；同时统一 `username`（永不可改）/ `nickname`（可改）命名，
并给 `admins` 补昵称（5.1.6 / 5.4.9）。验收见 `server/scripts/p12_invite_check.sh` 与 23 节 133–143。

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
| **Web 后台前端框架** | ✅ **Vue 3 + Vite**（2026-11-03 定，**2026-11-04 已落地**） | 目录 `web/`，与 `server/` 并列；无 UI 框架，样式手写以复用设计语言 |
| Android 最低支持版本 | ✅ **`minSdk 26` / `targetSdk 35`**（已落地并构建通过） | `UsageStatsManager.queryEvents` 在 8.0 上稳定 |

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

### 24.6 开发阶段的数据与迁移策略（2026-11-04 用户明确）

> **现在是开发阶段：不兼容旧数据、旧代码、旧版本。可以完全推倒重来，数据库可以重新设计。**

| 场景 | 开发阶段的做法 | 不要做的事 |
|---|---|---|
| 加字段 / 改字段 | 直接改 `0001`，或 `-reset` 后重跑；必要时 squash 迁移链 | 为「已执行的迁移不能改」而将就设计 |
| 改表关系 | 重新设计表结构，`-reset` 重建 | 写数据迁移脚本、双写窗口、回滚预案 |
| 已有测试数据 | 丢了就丢了，重跑脚本再生成 | 为了保住测试数据写兼容代码 |
| 旧版本代码 | 直接删或改，不留兼容分支 | 保留「兼容 v1」的兜底逻辑 |

**仍然必须做的两件事**（「不兼容旧数据」不等于「不写文档」）：

1. `go run ./cmd/migrate -verify` —— 重建后表结构必须与第 3 节一致
2. `python scripts/api_audit.py` —— 接口清单与路由必须零漂移

> 判据：**任何为了「保住已有数据」而增加的复杂度，在当前阶段都是错的。**

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
119. **登出后，登出前签发的 refresh token 重放必须返回 401**（后台与前台各一条）。
120. 登出只吊销该主体的 refresh，**不影响其他账号**；同账号其他设备的 refresh 一并失效。
121. 登出后旧 access token 在到期前**仍可用**（JWT 无状态，这是已知且接受的代价）。
122. 未带 token 调 logout 返回 **401**；前台 token 打后台 logout 返回 **401**。
123. 登出后重新登录可正常使用，账号不被锁死。
124. **Web 后台 access 过期时自动刷新并重放原请求**，用户不被打断；
     并发多个请求同时 401 时**只发起一次刷新**（并发去重）。

### 25.8 Android 界面（实现时登记进 13 节，界面数 15 → 17）

| # | 界面 | 关键内容 | 可见角色 |
|---|---|---|---|
| 13.16 | **全服任务** | 平台任务列表、本家提交、待评分 | 全部（提交仅组长） |
| 13.17 | **家庭星榜** | 总榜 / 月榜、本家名次、退出榜单 | 全部 |
