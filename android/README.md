# 家庭积分 · Android 客户端

Kotlin + Jetpack Compose 原生实现，覆盖 `docs/features.md` 第 13 节的 16 个界面。

## 构建

```bash
cd android
./gradlew assembleDebug          # 产物：app/build/outputs/apk/debug/app-debug.apk
```

需要先设置两个环境变量（`local.properties` 只放 `sdk.dir`，不要提交）：

```bash
export JAVA_HOME=/path/to/jdk-17      # AGP 8.7 不接受 JDK 25
export ANDROID_HOME=/path/to/Android/Sdk
```

wrapper 已提交（`gradlew` / `gradlew.bat` / `gradle-wrapper.jar`），**不需要**本机装 Gradle。
`gradle-wrapper.properties` 的 `distributionUrl` 指向华为云镜像，国内下载更稳；
换到海外网络时可改回 `services.gradle.org`。

**环境要求**

| 项 | 版本 |
|---|---|
| JDK | 17 |
| Gradle | 8.9 |
| AGP | 8.7.3 |
| Kotlin | 2.0.21 |
| minSdk | **26**（Android 8.0） |
| targetSdk / compileSdk | 35 |

## 连接后端

后端地址写在 `app/build.gradle.kts` 的 `buildConfigField("API_BASE_URL", ...)`：

| 运行环境 | 地址 |
|---|---|
| Android 模拟器 | `http://10.0.2.2:8080/`（默认，指向宿主机） |
| 真机（同一局域网） | `http://<你的电脑 IP>:8080/` |

Manifest 已开 `usesCleartextTraffic`，开发期可直接用 HTTP。

## 已实现的完整链路

### 账号与家庭组
- 注册（创建家庭组 / 稍后加入）、登录、退出
- 未入组时进入「输入邀请码」页
- Token 持久化到 DataStore；**access 过期自动用 refresh 换新并重放原请求**（OkHttp Authenticator）

### 积分
- **积分榜**（`score + frozen_score` 净额）与**得分榜**（区间正分毛额）双榜切换，得分榜支持周/月
- **趋势折线**（2026-11-06）：积分榜页用 `Canvas + drawLine` 画 earned 实线 / net 虚线，周/月切换联动；
  只画服务端 `/api/app/stats/trend` 的结果，客户端不做业务判定（零新增依赖）
- 成员详情、全量记录流水
- 赠送积分（选人 + 金额 + 附言，实时校验余额）
- 商城兑换（余额校验 + 二次确认）

### 任务
- 发布（冻结积分）、接取、放弃、提交（说明必填）
- 全员投票：实时票数进度（同意 X / 需要 Y）、发布者无投票权、投票后不可改
- 关闭任务（冻结退回）

### 屏幕时间
- `UsageStatsManager.queryEvents` 配对 `ACTIVITY_RESUMED/PAUSED` 自行累加（**不用 `INTERVAL_DAILY`**，避免按自然日切桶）
- 排除桌面、系统 UI、设置与本应用自身
- 区分总时长与夜间时段时长（支持跨零点）
- **明细视图**（2026-11-05）：今日 24 小时分布（点柱子看该小时分钟数、标出峰值小时）+
  近 7 天每日时长列表。一次 `queryEvents` 覆盖 7 天、按「天 × 小时」分桶，
  逐天各查一次会重复扫描事件。**小时桶只在本地算、不上传**（服务端只存日总量）
- 夜间时段取**本机登录成员自己的规则**（`screen_rules`），不是写死 0–7
- WorkManager 每晚 21:00 上报「昨天」；**打开 App 时补算一次**（国产 ROM 杀后台的兜底）
- 权限检测 + 跳转系统设置；未授权时 Worker 直接成功返回，不无限重试

### 组长工具
- 成员管理：邀请码查看/重置、踢出（二次确认，提示清零+禁入）、设组长、隐藏
- 睡觉规则：按成员配置睡觉时间点、起床点、允许分钟数
- 商品管理：增删改

## 结构

```
app/src/main/java/com/familypoints/app/
├── FamilyPointsApp.kt        Application + WorkManager 工厂
├── MainActivity.kt           单 Activity + Compose 入口
├── data/
│   ├── api/                  DTO / Retrofit 接口 / OkHttp 客户端
│   └── local/                TokenStore（DataStore）
├── screen/                   用量采集 + 上报 Worker + 调度
└── ui/
    ├── AppViewModel.kt       全部业务动作与会话状态
    ├── Components.kt         通用组件
    ├── nav/AppNav.kt         导航 + 底部 Tab
    └── screens/              16 个界面（按功能分 7 个文件）
```

## 已知未实现

| 项 | 原因 |
|---|---|
| ~~商品券与核销投票~~ | ✅ **已实现**（2026-11-03 后端 + 客户端同步落地，见 9.6）：我的券三态、用券发起核销、跨家庭投票 |
| 头像自定义上传 | 决策为「仅预设头像」（19.14），当前直接渲染 emoji |
| ~~趋势图~~ | ✅ **已实现**（2026-11-06）：积分榜页双线折线；顺带修掉 `TrendPoint` DTO 的 `label/value` 与后端 `date/earned/net` 错配 |
| 通知点击跳转 | 目前只做已读，未做深链到具体任务 |

## 验证状态

**已通过真实构建**，两台机器各一次：

| 日期 | JDK | Gradle | 结果 |
|---|---|---|---|
| 2026-11-03 | 21.0.2 | 8.9 | `BUILD SUCCESSFUL in 54s` |
| 2026-11-04 | 17.0.21（Microsoft） | 8.9（wrapper） | `BUILD SUCCESSFUL in 16m 16s`（首次含依赖下载），增量 50s |

产物：`app/build/outputs/apk/debug/app-debug.apk`，**19 MB**，
`package: com.familypoints.app.debug`，`minSdk 26` / `targetSdk 35`。

> **平台 35 可自动补齐**：本机 SDK 原本只有 platform 34，AGP 8.7 在构建时自动下载了
> `platforms/android-35`（`licenses/android-sdk-license` 已接受即可），无需手工装 cmdline-tools。

### 构建过程中修掉的 3 个错误（2026-11-03）

1. **`ApiService.kt` 未闭合注释** —— KDoc 里写了 `/api/app/*`，Kotlin 支持嵌套块注释，`/*` 被当成新的注释开始，导致注释永不闭合。改为 `/api/app/...`。
2. **`TokenStore` 缺 `@ApplicationContext`** —— 裸 `Context` 无法被 Hilt 解析，Dagger 报 MissingBinding。
3. **`AppViewModel` 的 suspend 调用位置** —— `runCatching{}.onFailure{}` 里读 DataStore，改为显式 `try/catch`。

### 真机验证（2026-11-04，HUAWEI ANA-AN00 / Android 12 / API 31）

`adb install` + 真机启动 + 真登录**全部通过**，走的是 `adb reverse` 隧道：

```bash
adb reverse tcp:8080 tcp:8080          # 设备的 127.0.0.1:8080 → 本机 8080
./gradlew assembleDebug -PapiBaseUrl=http://127.0.0.1:8080/
adb install -r app/build/outputs/apk/debug/app-debug.apk
adb shell am start -n com.familypoints.app.debug/com.familypoints.app.MainActivity
```

已验证的界面（截图逐一核对）：登录 → 积分榜（4 名成员、真实余额、冻结额正确）
→ 任务（可接取为空符合「排除自己发布」）→ 商城（余额不足置灰、我的券入口）
→ 我的（组长工具三项按角色显示）。

**踩到的坑**：华为设备在**密码框获得焦点时 `adb screencap` 返回空文件**（安全截屏保护）。
先 `adb shell input keyevent 4` 收起键盘再截即可。

### 屏幕时间链路（2026-11-04 真机验证通过）

`UsageStatsManager` 采集 → WorkManager 上报 → 服务端结算，全链路跑通：

```
POST /api/device/screen/report  200 (58-byte body，含 night_min)
screen_reports  revision 1 → 2 → 3（覆盖重算）
night_ok = true，连续第 3 天命中「满 3 天 +5」档位 → 当天 +7 / -3
```

> ⚠️ 上面这行是 **2026-11-04 当时的历史路径**。设备令牌机制已于 2026-11-05 整体删除，
> 现在上报走 `POST /api/app/screen/report`（成员登录态 + `device_key` / `device_model`），
> 详见 features 12 节与 15.4 节。此处保留原始记录，不改写历史。

**过程中修掉的 4 个 bug**（都是「编译能过、跑起来才发现」）：

1. **设备令牌没有任何入口**：`TokenStore.deviceToken` 只读不写，Worker 拿不到令牌直接 return，
   而页面还显示「已与服务器同步」。现在屏幕时间页有配置卡片，保存后立刻补算一次。
2. **`ScreenRuleList` 字段名错**：后端返回 `screen_rules`，客户端读 `items` → 永远空列表，
   夜间配置拿不到（且被 `runCatching` 静默吞掉）。
3. **`nightMin` 被省略**：kotlinx 默认 `encodeDefaults=false`，`Int` 默认值 0 不会编码，
   服务端当成「未判定」，夜间永远不加分。改为 `Int?`。
4. **规则取错了人**：原来 `firstOrNull()` 随便抓一条，多成员家庭会算错夜间判定；
   改为按本机登录成员匹配。

### 仍未验证

- **token 自动刷新**：refresh 过期后重放请求的路径没触发过（access 2h，测试期间没到期）
- 商品券核销投票的完整链路（需要多个账号轮流操作）
- 已知性能问题：`/api/app/state` 1.1s、`/api/app/stats/ranking` 1.2s。
  **榜单的 N+1 已修**（2026-11-05）：原来每个成员单独查一次连续天数，
  4 人家庭实测 2.0–4.7s；改为一次查全组、内存分组计算后降到 1.0–1.6s（约 2.7 倍）。
  剩下的 ~1.2s 是**远端 MySQL 的两次往返 RTT**（榜单 + 连续天数各一条 SQL），
  不是算法问题——要继续压只能做缓存或把数据库搬到应用同机
