// 构建后处理：为 Cloudflare Pages 生成高级模式入口 _worker.js
// （Pages 部署时由 _worker.js 处理 /api/*，其余请求回退到静态资源；
//   Workers 部署（wrangler deploy）则不依赖此文件）
import { copyFileSync, writeFileSync } from 'node:fs';

copyFileSync('worker/index.js', 'dist/worker-api.js');
writeFileSync(
  'dist/_worker.js',
  `import api from './worker-api.js';
export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.pathname.startsWith('/api/')) return api.fetch(request, env);
    return env.ASSETS.fetch(request);
  },
};
`
);
console.log('dist/_worker.js generated');
