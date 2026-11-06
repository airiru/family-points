# 家庭积分榜 · v3（自建服务器）

多成员家庭积分系统：睡觉规则自动计分、任务悬赏与全员投票、积分赠送、兑换商城与商品券、
屏幕时间自动采集上报、榜单与趋势。三端自建，**没有云函数、没有第三方托管数据库**。

> **功能与架构的唯一事实源是 [`docs/features.md`](docs/features.md)**（数据模型、业务规则、
> 接口清单、验收要点）。本文件只负责「怎么跑起来」和「去哪查」，不复述业务细节。
> 项目处于**开发阶段**：不兼容任何旧数据，数据库可随时重建（见「开发约定」）。
> 部署方案（features 16 节）与待决策项（19 节）**仍挂起**，不影响本地开发。

## 一眼看懂

**数据流**：`web/`（开发期由 Vite 把 `/api` 代理到 `:8080`）与 `android/`（`API_BASE_URL` 直连）
都只连 `server/`；**只有 `server/` 访问 MySQL**，所有业务判定都在服务端。
管理后台给**运营方管理员**用，家庭成员只用 Android App，两者 token 互不通用。

```
MySQL 8  ──sqlx──  server/ Go+Gin :8080  ──/api 代理──  web/ Vue3 :5173
                            │
                            └──HTTP（API_BASE_URL）──  android/ Compose
```

| 层 | 目录 | 技术栈 | 说明 |
|---|---|---|---|
| 服务端 | `server/` | Go 1.25 + Gin + sqlx + golang-migrate | 21 张表，双前缀鉴权，全部业务判定在服务端 |
| 管理后台 | `web/` | Vue 3 + Vite 6 + vue-router | 14 个 .vue 页面，仅管理员使用 |
| 客户端 | `android/` | Kotlin 2.0 + Jetpack Compose + Hilt + WorkManager | 16 个界面，屏幕时间自动采集上报 |
| 文档 | `docs/features.md` | — | 25 节，功能 / 接口 / 验收唯一事实源 |
| 记忆 | `.memory/` | memory-handoff 协议 | 提炼层 `MEMORY.md` + 按日追加日志 |
| 原型 | `design/` | HTML | Android 15 高保真界面原型 |

## 环境要求

| 项 | 版本 / 说明 |
|---|---|
| Go | 1.25+（国内构建建议设 `GOPROXY=https://goproxy.cn,direct`、`GOSUMDB=off`） |
| MySQL | 8.0+（需要建库或已存在库的写权限；迁移会执行 DDL） |
| Node.js | 18+（Vite 6） |
| JDK / Android SDK | JDK **17**（AGP 8.7 不接受更高版本）、platform 35、Gradle 8.9 走 wrapper |
| 验收脚本 | bash + curl + Python 3 |

## 快速开始

### 1. 配置

`.env` 放在**仓库根目录**（`server/` 下启动时会自动读 `../.env`），已被 gitignore：

```bash
cp .env.example .env
# 必填：DB_HOST / DB_PORT / DB_USER / DB_PASSWORD / DB_NAME / JWT_SECRET / ADMIN_PASSWORD
```

| 变量 | 用途 |
|---|---|
| `APP_ENV` / `HTTP_ADDR` | `dev` / `:8080` |
| `DB_*` | MySQL 连接；`DB_NAME` 缺省 `family_points` |
| `JWT_SECRET` | token 签名密钥 |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | **仅初始化默认管理员时用一次** |
| `ACCESS_TTL` / `REFRESH_TTL` | 默认 `2h` / `720h`（30 天） |

### 2. 建表 + 初始化管理员（在 `server/` 下）

```bash
cd server
go run ./cmd/migrate             # 建库并升级到最新
go run ./cmd/migrate -verify     # 核对表齐全 + 关键约束（改过表结构必跑）
go run ./cmd/migrate -seed-admin # 幂等：已有管理员则跳过
```

`ADMIN_USERNAME` 首次登录**强制改密**，未改密前除改密接口外后台一律 403。
其它可用参数：`-version`、`-down N`、`-list`、`-grants`、`-check-ddl`、`-force N`、
`-reset`（**仅 `APP_ENV=dev`，删光全部表重建**）。

### 3. 起后端

```bash
cd server
go run ./cmd/api                 # http://127.0.0.1:8080
curl http://127.0.0.1:8080/healthz   # {"ok":true,"data":{"env":"dev","db":"up"}}
```

`/healthz` 会真的 Ping 一次库，能区分「服务活着」与「库挂了」。

### 4. 起管理后台

```bash
cd web
npm install
npm run dev                      # http://127.0.0.1:5173，/api 已代理到 8080
```

### 5. 构建 Android（可选）

```bash
cd android
export JAVA_HOME=/path/to/jdk-17
export ANDROID_HOME=/path/to/Android/Sdk
./gradlew assembleDebug          # app/build/outputs/apk/debug/app-debug.apk
```

后端地址由 `build.gradle.kts` 的 `API_BASE_URL` 决定：模拟器默认 `http://10.0.2.2:8080/`；
真机推荐 `adb reverse tcp:8080 tcp:8080` 配合 `-PapiBaseUrl=http://127.0.0.1:8080/`。
详见 [`android/README.md`](android/README.md) 与 [`web/README.md`](web/README.md)。

## 门禁与验收

| 检查 | 命令 | 判据 |
|---|---|---|
| 服务端编译 | `cd server && go build ./... && go vet ./...` | 无输出即通过 |
| 表结构 | `cd server && go run ./cmd/migrate -verify` | 「结构核对全部通过」 |
| 接口对账 | `cd server && python scripts/api_audit.py` | **双向零漂移**（见下） |
| Web 构建 | `cd web && npm run build` | 产物在 `web/dist/` |
| Android 构建 | `cd android && ./gradlew assembleDebug` | `BUILD SUCCESSFUL` |
| 接口回归 | `cd server && for s in scripts/p*_check.sh; do bash "$s"; done` | 每个脚本「失败 0 项」 |

接口回归脚本需要先起好 API（默认打 `127.0.0.1:8080`），依赖 `lib_admin.sh` 统一的管理员登录
（脚本会改管理员密码，写死密码会集体假红）。`p5` / `p7` 等脚本还会用 `python` 解析 JSON。
脚本用 `curl` 直连本机：**若 shell 里设了 `HTTP_PROXY`，需 `export NO_PROXY=127.0.0.1,localhost`**，
否则请求会被代理拦成 502。`p12` 的过期断言会调 `cmd/devtool -expire-invitation` 做时间旅行
（仅 `APP_ENV=dev` 可用），不依赖定时等待。

> **`api_audit.py` 是硬门禁**：`docs/features.md` 第 15 节登记的接口必须和
> `server/internal/router/router.go` **双向一一对应**。
> 新增接口要同轮登记 15.2 / 15.3；**未实现的功能只在功能章节写状态，不要在接口清单里写死路径**。

## 接口约定（速查）

- 响应统一 `{ ok: bool, data?: any, error?: string }`
- `/api/admin/*` 只认 `typ=admin` 的 token；`/api/app/*` 只认 `typ=member`；两者互不通用
- 跨组操作 `family_id` **只能走路径参数**（后台）、前台一律取自 token 主体
- 公开接口：`/api/register`、两个 `login`、两个 `refresh`；`/api/app/logout` 只需有效 access
- 屏幕时间上报用**成员登录态**，设备令牌机制已于 2026-11-05 整体删除
- **账号字段口径**：`username` 是登录凭证、全局唯一、注册后永不可改；`nickname` 是显示名、可改。
  `members` / `admins` 两张表都有，API / DTO 一律用 `nickname`（`records.member_name` 是历史快照列，保留旧名）
- 完整清单：features 15 节；鉴权细节：features 4 节；P3.1 待确认邀请：features 5.2.3

## 文档索引

| 想查什么 | 去哪 |
|---|---|
| 架构 / 技术选型 / 表结构 | features 1–3 节（`server/migrations/0001_init.up.sql` 是 DDL 事实源） |
| 角色权限 / 账号 / 家庭组 | features 4–6 节 |
| 睡觉规则 / 积分流转 / 商城 / 赠送 / 榜单 | features 7–11 节 |
| 屏幕时间（含多设备、后台设备明细） | features 12 节 |
| Android 界面 / Web 后台页面 | features 13 / 14 节 |
| 接口清单 / 对账 | features 15 节 + `scripts/api_audit.py` |
| 部署 / 待决策（**挂起**） | features 16 / 19 节 |
| 审计 / 通知 / 验收要点 | features 21–23 节 |
| 实施顺序 / 未来设计（未实现） | features 24 / 25 节 |
| 跨会话上下文 | `.memory/MEMORY.md` + `.memory/YYYY-MM-DD.md`（memory-handoff 协议） |

## 开发约定

1. **不兼容旧数据**：开发阶段不为旧版本 / 旧字段写兼容层，`-reset` 重建数据库是日常手段。
2. **文档先行**：改了表结构就跑 `-verify`，增删接口就跑 `api_audit.py`；文档是事实源，代码向它对齐。
3. **前端不做业务判定**：积分结算、夜间判定、投票结果等一律以服务端结果为准。
4. **按阶段提交**：先对账 → 只 `add` 本阶段路径 → 提交；临时产物（点检脚本 / 截图）不入库。
5. **记忆只留线索**：`.memory/` 日志按日追加，凡能从磁盘验证的结论，动手前先复核。

## 历史

v1（Vue 3 + Cloudflare Workers + KV）的代码已于 2026-11-03 按决策**全部删除**，
需要对照时可用 `git show 46ff29e:worker/index.js` 取回。当前仓库从 v3 起，
不含任何 v1 兼容代码。
