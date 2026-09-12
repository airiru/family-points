/**
 * 成员积分管理系统 — 阿里云 ESA 边缘函数
 *
 * 部署步骤见 README.md。数据保存在 ESA 边缘 KV 中（存储空间名称需与下方 KV_NAMESPACE 一致）。
 * 整个系统的状态以一个 JSON 存储，多成员访问共享同一份数据。
 */

const KV_NAMESPACE = 'jifen'; // 控制台创建的 KV 存储空间名称
const STATE_KEY = 'state';
const MAX_RECORDS = 500; // 历史记录上限，超出后丢弃最旧的
const PASSCODE = ''; // 访问口令。留空 = 不启用鉴权；设置后打开网页会要求输入口令

function html() {
  return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>家庭积分榜</title>
<style>
  :root { --p:#4f6ef7; --bg:#f4f6fb; --card:#fff; --txt:#2b2f3a; --sub:#8a90a0; --ok:#22a05c; --bad:#e05252; }
  * { box-sizing:border-box; margin:0; padding:0; }
  body { font-family:system-ui,-apple-system,"PingFang SC","Microsoft YaHei",sans-serif; background:var(--bg); color:var(--txt); }
  .wrap { max-width:640px; margin:0 auto; padding:16px 16px 80px; }
  h1 { font-size:20px; padding:8px 0 16px; text-align:center; }
  .tabs { display:flex; gap:6px; margin-bottom:16px; background:#e6e9f4; border-radius:12px; padding:4px; }
  .tabs button { flex:1; border:0; background:transparent; padding:9px 0; border-radius:9px; font-size:14px; cursor:pointer; color:var(--sub); }
  .tabs button.on { background:var(--card); color:var(--txt); font-weight:600; box-shadow:0 1px 3px rgba(0,0,0,.08); }
  .card { background:var(--card); border-radius:14px; padding:16px; margin-bottom:14px; box-shadow:0 1px 4px rgba(30,40,90,.06); }
  .row { display:flex; align-items:center; gap:10px; padding:10px 0; border-bottom:1px solid #eef0f6; }
  .row:last-child { border-bottom:0; }
  .row .grow { flex:1; min-width:0; }
  .name { font-size:15px; font-weight:600; }
  .clickable { cursor:pointer; }
  .sub { font-size:12px; color:var(--sub); margin-top:2px; word-break:break-all; }
  .score { font-size:20px; font-weight:700; color:var(--p); }
  .pts { font-weight:700; }
  .plus { color:var(--ok); } .minus { color:var(--bad); }
  .btn { border:0; border-radius:8px; padding:7px 13px; font-size:13px; cursor:pointer; background:var(--p); color:#fff; }
  .btn.ghost { background:#eef1fb; color:var(--p); }
  .btn.del { background:#fdeeec; color:var(--bad); }
  .btn:active { opacity:.8; }
  .form { display:flex; gap:8px; margin-top:12px; flex-wrap:wrap; }
  input, select { border:1px solid #dfe3ee; border-radius:8px; padding:9px 10px; font-size:14px; outline:none; background:#fbfcff; }
  input:focus, select:focus { border-color:var(--p); }
  input.n { width:80px; }
  .empty { text-align:center; color:var(--sub); font-size:13px; padding:24px 0; }
  .toast { position:fixed; left:50%; bottom:90px; transform:translateX(-50%); background:#333a4d; color:#fff; padding:9px 18px; border-radius:20px; font-size:13px; opacity:0; transition:.25s; pointer-events:none; z-index:10; }
  .toast.show { opacity:1; }
  .modal-bg { position:fixed; inset:0; background:rgba(0,0,0,.4); display:none; align-items:center; justify-content:center; z-index:9; }
  .modal-bg.show { display:flex; }
  .modal { background:#fff; border-radius:14px; padding:18px; width:88%; max-width:400px; max-height:70vh; overflow:auto; }
  .modal h3 { font-size:16px; margin-bottom:12px; }
  .rule-btn { width:100%; text-align:left; border:1px solid #e3e7f3; background:#fafbff; border-radius:10px; padding:11px 13px; margin-bottom:8px; cursor:pointer; font-size:14px; }
  .time { font-size:11px; color:var(--sub); }
  .desc { font-size:12px; color:var(--sub); margin-bottom:10px; line-height:1.6; }
</style>
</head>
<body>
<div class="wrap">
  <h1>🏆 家庭积分榜</h1>
  <div class="tabs" id="tabs"></div>
  <div id="view"></div>
</div>
<div class="modal-bg" id="modalBg"><div class="modal" id="modal"></div></div>
<div class="toast" id="toast"></div>
<script>
let S = null; // { members, rules, items, records }
let tab = 'members';
let recordFilter = ''; // 记录页按成员筛选，空 = 全部
const $ = id => document.getElementById(id);
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const fmt = t => new Date(t).toLocaleString('zh-CN', { month:'2-digit', day:'2-digit', hour:'2-digit', minute:'2-digit' });
function toast(m) { const t = $('toast'); t.textContent = m; t.classList.add('show'); setTimeout(() => t.classList.remove('show'), 1600); }
function modal(htmlStr) { $('modal').innerHTML = htmlStr; $('modalBg').classList.add('show'); }
function closeModal() { $('modalBg').classList.remove('show'); }
$('modalBg').addEventListener('click', e => { if (e.target === $('modalBg')) closeModal(); });

async function api(path, body) {
  const opt = body ? { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify(body) } : {};
  const r = await fetch('/api/' + path, opt);
  let j;
  try { j = await r.json(); } catch { j = { ok:false, error:'网络错误' }; }
  if (r.status === 401) {
    const code = prompt("请输入访问口令");
    if (code) { localStorage.setItem('fp_code', code); return api(path, body); }
    throw new Error('需要口令');
  }
  if (!j.ok) { toast(j.error || '操作失败'); throw new Error(j.error); }
  S = j.state;
  render();
}

const TABS = [ ['members','成员'], ['rules','计分规则'], ['score','记一笔'], ['items','兑换商城'], ['records','记录'] ];

function render() {
  $('tabs').innerHTML = TABS.map(([k, label]) => \`<button class="\${tab===k?'on':''}" onclick="tab='\${k}';render()">\${label}</button>\`).join('');
  $('view').innerHTML = { members:vMembers, rules:vRules, score:vScore, items:vItems, records:vRecords }[tab]();
}

function rank(list) { return [...list].sort((a,b) => b.score - a.score); }
function memberRecords(id) { return S.records.filter(r => r.memberId === id).sort((a,b) => b.time - a.time); }

function vMembers() {
  if (!S.members.length) return '<div class="card"><div class="empty">还没有成员，先添加一个吧</div></div>';
  return rank(S.members).map((m, i) => \`
    <div class="card"><div class="row">
      <div style="font-size:18px;width:26px;text-align:center">\${['🥇','🥈','🥉'][i] || (i+1)}</div>
      <div class="grow clickable" onclick="showMember('\${m.id}')"><div class="name">\${esc(m.name)} ›</div><div class="sub">\${memberRecords(m.id).length} 条记录</div></div>
      <div class="score">\${m.score} 分</div>
      <button class="btn ghost" onclick="pickRule('\${m.id}')">记一笔</button>
      <button class="btn del" onclick="delMember('\${m.id}')">删</button>
    </div></div>\`).join('')
  + \`<div class="card"><div class="form">
      <input id="nm" placeholder="成员名字" style="flex:1" onkeydown="if(event.key==='Enter')addMember()">
      <button class="btn" onclick="addMember()">添加成员</button></div></div>\`;
}

function showMember(id) {
  const m = S.members.find(x => x.id === id); if (!m) return;
  const recs = memberRecords(id);
  modal(\`<h3>\${esc(m.name)} · 当前 \${m.score} 分</h3>
    <div style="margin-bottom:12px"><button class="btn del" onclick="if(confirm('将「\${esc(m.name)}」的积分清零？')){closeModal();api('member/reset',{id:'\${m.id}'});}">积分清零</button></div>
    \${recs.length ? recs.slice(0, 50).map(r => \`<div class="row">
      <div class="grow"><div class="name" style="font-size:14px;font-weight:500">\${esc(r.title)}</div><div class="time">\${fmt(r.time)}</div></div>
      <div class="pts \${r.points>=0?'plus':'minus'}">\${r.points>=0?'+':''}\${r.points} 分</div></div>\`).join('')
      + (recs.length > 50 ? \`<div class="empty">仅显示最近 50 条，共 \${recs.length} 条</div>\` : '')
      : '<div class="empty">暂无记录</div>'}\`);
}

function vRules() {
  return \`<div class="card"><div class="desc">计分规则可自定义，正数为加分，负数为减分。例如：按时睡觉 +1、晚睡半小时 -3。</div>\${
    S.rules.map(r => \`<div class="row"><div class="grow"><div class="name">\${esc(r.name)}</div></div>
      <div class="pts \${r.points>=0?'plus':'minus'}">\${r.points>=0?'+':''}\${r.points} 分</div>
      <button class="btn del" onclick="delRule('\${r.id}')">删</button></div>\`).join('')
    || '<div class="empty">还没有规则</div>'
  }<div class="form">
    <input id="rn" placeholder="规则名称，如：按时睡觉" style="flex:1" onkeydown="if(event.key==='Enter')addRule()">
    <input id="rp" class="n" type="number" placeholder="±分值" onkeydown="if(event.key==='Enter')addRule()">
    <button class="btn" onclick="addRule()">添加规则</button></div></div>\`;
}

function vScore() {
  if (!S.members.length) return '<div class="card"><div class="empty">请先在「成员」页添加成员</div></div>';
  const memberSel = \`<select id="sm" style="flex:1">\${S.members.map(m => \`<option value="\${m.id}">\${esc(m.name)}（\${m.score} 分）</option>\`).join('')}</select>\`;
  const custom = \`<div class="card"><div class="desc">自定义记一笔：不在规则里的临时事项，直接填名称和分值。</div>
    <div class="form">
      <input id="ct" placeholder="事项，如：主动做家务" style="flex:1;min-width:140px">
      <input id="cp" class="n" type="number" placeholder="±分值">
      <button class="btn" onclick="applyCustom()">记一笔</button></div></div>\`;
  if (!S.rules.length) return \`<div class="card"><div class="form" style="margin:0">\${memberSel}</div></div>\` + custom
    + '<div class="card"><div class="empty">还没有计分规则，可先在「计分规则」页添加，或用上面的自定义记一笔</div></div>';
  return \`<div class="card"><div class="form" style="margin:0 0 12px">\${memberSel}</div><div id="sr"></div></div>\`
    + rulePickerHtml('applyScore') + custom;
}
function rulePickerHtml(fn) {
  return S.rules.map(r => \`<button class="rule-btn" onclick="\${fn}('\${r.id}')">
    <b>\${esc(r.name)}</b>
    <span class="pts \${r.points>=0?'plus':'minus'}" style="float:right">\${r.points>=0?'+':''}\${r.points} 分</span></button>\`).join('');
}
function pickRule(memberId) {
  tab = 'score'; render();
  const sel = $('sm'); if (sel) sel.value = memberId;
  window.scrollTo({ top: 0, behavior: 'smooth' });
}
function currentMember() { const m = S.members.find(x => x.id === $('sm').value); if (!m) toast('请选择成员'); return m; }
function applyScore(ruleId) {
  const m = currentMember(); if (!m) return;
  const r = S.rules.find(x => x.id === ruleId);
  api('score/add', { memberId: m.id, ruleId }).then(() => toast(\`\${m.name} \${r.points>=0?'加':'减'} \${Math.abs(r.points)} 分 ✓\`));
}
function applyCustom() {
  const m = currentMember(); if (!m) return;
  const title = $('ct').value.trim(); const points = parseInt($('cp').value, 10);
  if (!title || isNaN(points) || points === 0) return toast('请填写事项和不为 0 的分值');
  api('score/custom', { memberId: m.id, title, points }).then(() => {
    toast(\`\${m.name} \${points>0?'加':'减'} \${Math.abs(points)} 分 ✓\`); $('ct').value = ''; $('cp').value = '';
  });
}
function vItems() {
  return \`<div class="card"><div class="desc">自定义可兑换的物品和所需积分，成员用积分兑换。</div>\${
    S.items.map(it => \`<div class="row"><div class="grow"><div class="name">🎁 \${esc(it.name)}</div><div class="sub">需要 \${it.cost} 积分</div></div>
      <button class="btn ghost" onclick="redeem('\${it.id}')">兑换</button>
      <button class="btn del" onclick="delItem('\${it.id}')">删</button></div>\`).join('')
    || '<div class="empty">还没有物品</div>'
  }<div class="form">
    <input id="in" placeholder="物品名称，如：冰淇淋" style="flex:1" onkeydown="if(event.key==='Enter')addItem()">
    <input id="ic" class="n" type="number" placeholder="积分" onkeydown="if(event.key==='Enter')addItem()">
    <button class="btn" onclick="addItem()">添加物品</button></div></div>\`;
}
function vRecords() {
  const all = [...S.records].sort((a,b) => b.time - a.time)
    .filter(r => !recordFilter || r.memberId === recordFilter);
  return \`<div class="card">
    <select id="rf" style="width:100%;margin-bottom:10px" onchange="recordFilter=this.value;render()">
      <option value="">全部成员</option>
      \${S.members.map(m => \`<option value="\${m.id}" \${recordFilter===m.id?'selected':''}>\${esc(m.name)}</option>\`).join('')}
    </select>
    \${all.length ? all.map(r => \`<div class="row">
      <div class="grow"><div class="name" style="font-size:14px;font-weight:500">\${esc(r.name)} · \${esc(r.title)}</div>
      <div class="time">\${fmt(r.time)}</div></div>
      <div class="pts \${r.points>=0?'plus':'minus'}">\${r.points>=0?'+':''}\${r.points} 分</div></div>\`).join('') : '<div class="empty">暂无记录</div>'}</div>\`;
}

function addMember() { const v = $('nm').value.trim(); if (!v) return toast('请输入名字'); api('member/add', { name: v }); $('nm').value = ''; }
function delMember(id) { const m = S.members.find(x => x.id === id); if (confirm(\`删除成员「\${m.name}」及其全部记录？\`)) api('member/del', { id }); }
function addRule() {
  const n = $('rn').value.trim(); const p = parseInt($('rp').value, 10);
  if (!n || isNaN(p)) return toast('请填写规则名称和分值');
  api('rule/add', { name: n, points: p }); $('rn').value = ''; $('rp').value = '';
}
function delRule(id) { if (confirm('删除该计分规则？')) api('rule/del', { id }); }
function addItem() {
  const n = $('in').value.trim(); const c = parseInt($('ic').value, 10);
  if (!n || isNaN(c) || c <= 0) return toast('请填写物品名称和正数积分');
  api('item/add', { name: n, cost: c }); $('in').value = ''; $('ic').value = '';
}
function delItem(id) { if (confirm('删除该物品？')) api('item/del', { id }); }
function redeem(itemId) {
  const it = S.items.find(x => x.id === itemId);
  modal(\`<h3>兑换「\${esc(it.name)}」</h3>\`
    + (S.members.length ? S.members.map(m => \`<button class="rule-btn" onclick="doRedeem('\${m.id}','\${itemId}')">
        \${esc(m.name)} <span style="float:right" class="pts \${m.score < it.cost ? 'minus':''}">当前 \${m.score} 分</span></button>\`).join('')
      : '<div class="empty">暂无成员</div>'));
}
function doRedeem(memberId, itemId) {
  closeModal();
  api('item/redeem', { memberId, itemId }).then(() => toast('兑换成功 🎉'));
}

api('state').catch(e => { if (e.message !== '需要口令') $('view').innerHTML = '<div class="card"><div class="empty">加载失败，请刷新重试</div></div>'; });
</script>
</body>
</html>`;
}

// ---------- 数据与操作 ----------

const emptyState = () => ({ members: [], rules: [], items: [], records: [] });
const uid = () => Date.now().toString(36) + Math.random().toString(36).slice(2, 7);

async function getState(kv) {
  const raw = await kv.get(STATE_KEY);
  try {
    const s = JSON.parse(raw);
    return { members: [], rules: [], items: [], records: [], ...s };
  } catch { return emptyState(); }
}

async function saveState(kv, state) {
  state.records = state.records.slice(-MAX_RECORDS);
  await kv.put(STATE_KEY, JSON.stringify(state));
}

const json = (data, status = 200) =>
  new Response(JSON.stringify(data), { status, headers: { 'content-type': 'application/json; charset=utf-8' } });

// 每个操作：接收 (state, body) => 同步修改 state
const actions = {
  'member/add': (s, b) => {
    const name = (b.name || '').trim();
    if (!name) throw new Error('请输入成员名字');
    if (s.members.some(m => m.name === name)) throw new Error('该成员已存在');
    s.members.push({ id: uid(), name, score: 0 });
  },
  'member/del': (s, b) => {
    s.members = s.members.filter(m => m.id !== b.id);
    s.records = s.records.filter(r => r.memberId !== b.id);
  },
  'member/reset': (s, b) => {
    const m = s.members.find(x => x.id === b.id);
    if (!m) throw new Error('成员不存在');
    m.score = 0;
  },
  'rule/add': (s, b) => {
    const name = (b.name || '').trim();
    const points = parseInt(b.points, 10);
    if (!name || isNaN(points)) throw new Error('请填写规则名称和分值');
    s.rules.push({ id: uid(), name, points });
  },
  'rule/del': (s, b) => { s.rules = s.rules.filter(r => r.id !== b.id); },
  'score/add': (s, b) => {
    const m = s.members.find(x => x.id === b.memberId);
    const r = s.rules.find(x => x.id === b.ruleId);
    if (!m || !r) throw new Error('成员或规则不存在');
    m.score += r.points;
    s.records.push({ time: Date.now(), memberId: m.id, name: m.name, title: r.name, points: r.points });
  },
  'score/custom': (s, b) => {
    const m = s.members.find(x => x.id === b.memberId);
    const points = parseInt(b.points, 10);
    const title = (b.title || '').trim();
    if (!m || isNaN(points) || !title) throw new Error('参数不完整');
    m.score += points;
    s.records.push({ time: Date.now(), memberId: m.id, name: m.name, title, points });
  },
  'item/add': (s, b) => {
    const name = (b.name || '').trim();
    const cost = parseInt(b.cost, 10);
    if (!name || isNaN(cost) || cost <= 0) throw new Error('请填写物品名称和正数积分');
    s.items.push({ id: uid(), name, cost });
  },
  'item/del': (s, b) => { s.items = s.items.filter(i => i.id !== b.id); },
  'item/redeem': (s, b) => {
    const m = s.members.find(x => x.id === b.memberId);
    const it = s.items.find(x => x.id === b.itemId);
    if (!m || !it) throw new Error('成员或物品不存在');
    if (m.score < it.cost) throw new Error(`${m.name} 的积分不够（需要 ${it.cost}，当前 ${m.score}）`);
    m.score -= it.cost;
    s.records.push({ time: Date.now(), memberId: m.id, name: m.name, title: `兑换「${it.name}」`, points: -it.cost });
  },
};

export default {
  async fetch(request) {
    const kv = new EdgeKV({ namespace: KV_NAMESPACE });
    const url = new URL(request.url);

    if (!url.pathname.startsWith('/api/')) {
      return new Response(html(), { headers: { 'content-type': 'text/html; charset=utf-8' } });
    }

    // 可选口令校验：设置 PASSCODE 后，所有 API 请求需携带匹配的 x-passcode 头
    if (PASSCODE && request.headers.get('x-passcode') !== PASSCODE) {
      return json({ ok: false, error: '口令错误' }, 401);
    }

    const path = url.pathname.slice(5); // 去掉 /api/

    try {
      if (request.method === 'GET' && path === 'state') {
        return json({ ok: true, state: await getState(kv) });
      }
      if (request.method === 'POST') {
        const handler = actions[path];
        if (!handler) return json({ ok: false, error: '未知操作' }, 404);
        const body = await request.json().catch(() => ({}));
        const state = await getState(kv);
        handler(state, body);
        await saveState(kv, state);
        return json({ ok: true, state });
      }
      return json({ ok: false, error: '不支持的请求' }, 405);
    } catch (e) {
      return json({ ok: false, error: e.message || '服务器错误' }, 400);
    }
  },
};
