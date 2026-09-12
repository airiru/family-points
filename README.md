# 家庭积分榜（Vue 3 + Cloudflare Workers 版）

多成员积分管理系统：自定义计分规则（如"按时睡觉 +1"、"晚睡半小时 -3"，可配连续打卡奖励）、积分排行榜、自定义兑换商城、账号权限、历史记录。前端为 **Vue 3 + Vite**，数据接口由 **Cloudflare Worker** 提供，数据保存在 **Cloudflare KV**，多成员访问共享同一份数据。

## 项目结构

```
├── index.html          # Vite 入口 HTML
├── src/                # Vue 前端
│   ├── main.js         # 应用入口
│   ├── App.vue         # 根组件：hash 路由（积分榜/后台设置）+ 登录门控 + 全局弹窗/提示
│   ├── pages/          # HomePage（主页只读）、AdminPage（后台管理）、LoginPage（登录/初始化）
│   ├── store.js        # 共享状态与全部业务操作
│   └── style.css       # 全局样式
├── worker/index.js     # Cloudflare Worker：/api/* 数据接口（KV 读写、登录鉴权、连续打卡奖励）
├── wrangler.jsonc      # Cloudflare Workers 部署配置（构建产物目录、KV 绑定）
├── dev-server.mjs      # 本地联调服务器（托管 dist/ + 模拟 /api，仅开发用）
└── vite.config.js
```

## 部署（Cloudflare Workers）

1. **安装依赖并登录 Cloudflare**

   ```bash
   npm install
   npx wrangler login
   ```

2. **创建 KV 命名空间并填入 ID**

   ```bash
   npx wrangler kv namespace create KV
   ```

   命令会输出一个 `id`，把它填进 `wrangler.jsonc` 里 `kv_namespaces[0].id`（这是唯一需要手动填的一处配置）。

3. **构建并部署**

   ```bash
   npm run deploy   # = vite build + wrangler deploy
   ```

   部署完成后会输出 `https://family-points.<你的子域>.workers.dev`，打开即可使用。

4. **首次使用**：打开网页会要求创建管理员账号（一次性初始化），之后所有账号在「后台设置 → 账号」里管理，无需再改任何配置。

> 也可以在 Cloudflare 控制台把 Worker 连接到 GitHub 仓库（Workers Builds），推送代码自动构建部署；构建命令同样用 `npm run build`。

## 本地开发

```bash
npm install
npm run dev           # Vite 开发服务器（页面可看，API 需另外起服务）
npx wrangler dev      # 或：构建后在真实 Workers 运行时本地联调（自动模拟 KV）
node dev-server.mjs   # 或：轻量 Node 联调服务器（托管 dist/ 并模拟 /api，数据存内存）
```

## 功能说明

- **主页（积分榜）**：按积分实时排名（🥇🥈🥉），点击成员查看全部记录；🔥 显示连续打卡天数；最近记录列表可按成员筛选
- **后台设置**（仅管理员）：成员管理（增删/清零）、计分规则、记一笔、兑换商城、账号管理
- **计分规则**：自定义名称和 ± 分值；可选"连续打卡奖励"——该规则每连续打卡 N 天自动额外奖 M 分（中断重新计数，一天多次打卡不重复计天，按东八区）
- **记一笔**：选成员点规则一步计分；也支持自定义事项 ± 分值
- **兑换商城**：管理员可代任何成员兑换；账号绑定成员后，成员可在主页用自己的积分自助兑换
- **账号与权限**：管理员（全部权限）/ 查看者（只读 + 自助兑换），账号在页面里增删改，密码加盐哈希存 KV，登录凭证 30 天有效

## 注意事项

- Cloudflare KV 写入后需要短暂时间同步到全球节点，家庭使用场景下延迟可忽略。
- KV 最终一致：极小概率下两人同时写会有一次写入被覆盖，日常家庭使用几乎不会遇到。
- 从阿里云 ESA 版迁移：数据格式兼容，把旧 KV `jifen` 空间里 `state` 键的值导入新 KV 命名空间即可保留原有积分数据（`wrangler kv key put state --path=state.json`）。
