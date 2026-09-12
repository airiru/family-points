// 前端 API 层：与边缘函数交互，统一处理口令鉴权和错误提示
let onError = () => {};

export function setApiErrorHandler(fn) { onError = fn; }

export async function api(path, body) {
  const opt = body
    ? { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
    : {};
  const r = await fetch('/api/' + path, opt);
  let j;
  try { j = await r.json(); } catch { j = { ok: false, error: '网络错误' }; }
  if (r.status === 401) {
    const code = prompt('请输入访问口令');
    if (code) { localStorage.setItem('fp_code', code); return api(path, body); }
    throw new Error('需要口令');
  }
  if (!j.ok) { onError(j.error || '操作失败'); throw new Error(j.error); }
  return j.state;
}
