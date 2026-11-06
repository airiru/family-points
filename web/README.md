# 家庭积分 · Web 管理后台

Vue 3 + Vite，覆盖 `docs/features.md` 第 14 节的 14 个 `.vue`（12 个功能页 + 登录页 + 强制改密页）。
**仅管理员使用**，不提供成员只读视图。

## 开发

```bash
cd web
npm install
npm run dev      # http://127.0.0.1:5173，/api 已代理到 127.0.0.1:8080
```

先起后端（从仓库根目录）：

```bash
cd server && go run ./cmd/api
```

管理员账号由 `go run ./cmd/migrate -seed-admin` 初始化（取 `.env` 里的 `ADMIN_USERNAME` / `ADMIN_PASSWORD`），
首次登录必须改密。

## 构建

```bash
npm run build    # 产物在 dist/
```

## 结构

```
src/
├── api.js            统一 fetch 封装：token、401 跳登录、CSV 下载
│                     （localStorage 里存的是登录 username，不是昵称）
├── store.js          全局状态：当前管理员 + 当前家庭组（localStorage 记忆）
├── router.js         15 条路由（14 页面 + 1 条 catch-all redirect）+ 登录守卫
├── toast.js          轻量提示
├── components/
│   └── AppLayout.vue 侧边导航 + 顶部家庭组切换
└── pages/            14 个 .vue（12 个功能页 + 登录 + 改密）
```

## 账号口径（2026-11-06 P3.1）

- 管理员和成员一样有 `username` + `nickname`；**`username` 注册后永不可改**，
  localStorage 里存的正是它（改密后要用它重新登录，不能用昵称）。
- `管理员账号` 页有「我的昵称」卡片，`POST /api/admin/me/profile` 只改 `nickname`；
  请求体带 `username` 后端直接 400。成员列表显示 `nickname`，表头标注用户名不可改。
- 成员改名走 App 端；后台成员页是只读展示，不提供改昵称入口。

## 与后端的关系

- 所有请求走 `/api/admin/*`，**只认 admin token**（成员 token 一律 401）
- 跨组操作必须带 `family_id`，由顶部「当前家庭组」下拉决定
- 只读页面（记录 / 审计 / 屏幕时间 / 任务管理 / 赠送管理）**没有写操作入口**：
  记录是 append-only；任务审核走组内全员投票，**后台无代审核接口**；兑换撤销、赠送撤销已按
  features 9.4 / 10.2.6 移除；屏幕时间无手动录入
