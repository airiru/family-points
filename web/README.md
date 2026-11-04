# 家庭积分 · Web 管理后台

Vue 3 + Vite，覆盖 `docs/features.md` 第 14 节的 12 个页面。**仅管理员使用**，不提供成员只读视图。

## 开发

```bash
cd web
npm install
npm run dev      # http://127.0.0.1:5173，/api 已代理到 127.0.0.1:8080
```

先起后端：

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
├── store.js          全局状态：当前管理员 + 当前家庭组（localStorage 记忆）
├── router.js         14 条路由 + 登录守卫
├── toast.js          轻量提示
├── components/
│   └── AppLayout.vue 侧边导航 + 顶部家庭组切换
└── pages/            12 个页面
```

## 与后端的关系

- 所有请求走 `/api/admin/*`，**只认 admin token**（成员 token 一律 401）
- 跨组操作必须带 `family_id`，由顶部「当前家庭组」下拉决定
- 只读页面（记录 / 审计 / 屏幕时间）**没有删除入口**——记录是 append-only
