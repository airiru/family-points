// 本地联调服务器（仅开发用，不部署）：托管 dist/ 并把 /api/* 转给 worker/index.js
import http from 'node:http';
import { readFile } from 'node:fs/promises';
import { extname, join } from 'node:path';

globalThis.EdgeKV = class {
  static shared = {};
  async get(k) { return EdgeKV.shared[k]; }
  async put(k, v) { EdgeKV.shared[k] = v; }
};
const worker = (await import('./worker/index.js')).default;
const env = { KV: new EdgeKV() };

const MIME = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml' };

http.createServer(async (req, res) => {
  const url = new URL(req.url, 'http://localhost');
  if (url.pathname.startsWith('/api/')) {
    const chunks = [];
    for await (const c of req) chunks.push(c);
    const r = await worker.fetch(new Request('http://localhost' + req.url, {
      method: req.method,
      headers: req.headers,
      body: chunks.length ? Buffer.concat(chunks) : undefined,
    }), env);
    res.writeHead(r.status, { 'content-type': r.headers.get('content-type') });
    res.end(await r.text());
    return;
  }
  let p = url.pathname === '/' ? '/index.html' : url.pathname;
  try {
    const buf = await readFile(join('dist', p));
    res.writeHead(200, { 'content-type': MIME[extname(p)] || 'application/octet-stream' });
    res.end(buf);
  } catch {
    res.writeHead(404); res.end('not found');
  }
}).listen(8787, () => console.log('http://localhost:8787'));
