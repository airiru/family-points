# 家庭积分 · Android 客户端

Kotlin + Jetpack Compose 原生实现，覆盖 `docs/features.md` 第 13 节的 16 个界面。

## 构建

```bash
# 首次：用 Android Studio 打开 android/ 目录，它会自动补齐 gradle wrapper
# 或本地已装 Gradle 8.9+：
cd android
gradle wrapper          # 生成 gradlew
./gradlew assembleDebug
```

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
    └── screens/              16 个界面（按功能分 6 个文件）
```

## 已知未实现

| 项 | 原因 |
|---|---|
| **商品券与核销投票** | 后端 `coupons` / `coupon_votes` 两张表与 3 个接口**尚未实现**（文档 9.6 已设计），客户端无法对接 |
| 头像自定义上传 | 决策为「仅预设头像」（19.14），当前直接渲染 emoji |
| 趋势图 | 接口 `stats/trend` 已接，界面暂未画折线 |
| 通知点击跳转 | 目前只做已读，未做深链到具体任务 |

## 验证状态

**已通过真实构建**（2026-11-03）：

```
BUILD SUCCESSFUL in 54s
app/build/outputs/apk/debug/app-debug.apk   19 MB
package: com.familypoints.app.debug  minSdk 26  targetSdk 35
```

构建环境（本机已就绪）：

| 组件 | 版本 / 路径 |
|---|---|
| JDK | **21.0.2**（`D://tools//jdk-21.0.2`）——AGP 8.7 不接受 JDK 25 |
| Gradle | 8.9（`D://tools//gradle-8.9`） |
| Android SDK | `D://Android//Sdk`（platform 35 / build-tools 35.0.0） |

构建命令：

```bash
cd android
JAVA_HOME=/d/tools/jdk-21.0.2 /d/tools/gradle-8.9/bin/gradle assembleDebug
```

### 构建过程中修掉的 3 个错误

1. **`ApiService.kt` 未闭合注释** —— KDoc 里写了 `/api/app/*`，Kotlin 支持嵌套块注释，`/*` 被当成新的注释开始，导致注释永不闭合。改为 `/api/app/...`。
2. **`TokenStore` 缺 `@ApplicationContext`** —— 裸 `Context` 无法被 Hilt 解析，Dagger 报 MissingBinding。
3. **`AppViewModel` 的 suspend 调用位置** —— `runCatching{}.onFailure{}` 里读 DataStore，改为显式 `try/catch`。

### 尚未验证

- **未在真机 / 模拟器上运行过**，只验证了编译与打包
- 屏幕时间采集、WorkManager 定时上报、token 自动刷新等运行时行为**未经实测**
- 后端接口联调未做（需要先起 MySQL + Go 服务）
