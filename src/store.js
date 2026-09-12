// 共享状态与操作：页面组件都从这里读写
import { reactive, ref, computed } from 'vue';

export const state = reactive({ members: [], rules: [], items: [], records: [] });
export const loaded = ref(false);

const TOKEN_KEY = 'fp_token';
const authHeaders = () => {
  const t = localStorage.getItem(TOKEN_KEY);
  return t ? { 'x-auth': t } : {};
};
export const needLogin = ref(false); // 已初始化但当前无有效登录凭证
export const needSetup = ref(false); // 首次使用，需要创建管理员
export const myMemberId = ref(''); // 当前登录成员的 id（用于主页自助兑换）
export const user = ref(null); // { username, role: 'admin' | 'member' }
export const isAdmin = computed(() => !needLogin.value && (!user.value || user.value.role === 'admin'));

// token 形如 username.exp.hmac，角色以登录响应为准；这里从 token 解析出用户名和过期时间
function parseToken(token) {
  const [username, exp] = token.split('.');
  if (!username || Number(exp) < Date.now()) return null;
  return { username };
}

export async function login(username, password) {
  const r = await fetch('/api/login', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  });
  const j = await r.json().catch(() => ({ ok: false, error: '网络错误' }));
  if (!j.ok) throw new Error(j.error || '登录失败');
  localStorage.setItem(TOKEN_KEY, j.token);
  needLogin.value = false;
  user.value = { username: j.username, role: j.role };
  await refresh(); // 拉取完整 state（含 myMemberId、账号列表等）
}
// 首次使用：创建管理员账号并自动登录
export async function setup(username, password) {
  const r = await fetch('/api/setup', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  });
  const j = await r.json().catch(() => ({ ok: false, error: '网络错误' }));
  if (!j.ok) throw new Error(j.error || '初始化失败');
  localStorage.setItem(TOKEN_KEY, j.token);
  needSetup.value = false;
  needLogin.value = false;
  user.value = { username: j.username, role: 'admin' };
  if (j.state) Object.assign(state, j.state);
  loaded.value = true;
}
export function logout() {
  localStorage.removeItem(TOKEN_KEY);
  user.value = null;
  needLogin.value = true;
}
async function refresh() {
  const opt = { headers: authHeaders() };
  const r = await fetch('/api/state', opt);
  const j = await r.json().catch(() => ({ ok: false, error: '网络错误' }));
  if (r.status === 401) {
    needLogin.value = true;
    needSetup.value = !!j.needSetup;
    throw new Error('未登录');
  }
  if (!j.ok) { toast(j.error || '加载失败'); throw new Error(j.error); }
  Object.assign(state, j.state);
  // 以后端返回的角色为准（避免本地缓存的旧角色与最新配置不一致）
  user.value = { username: j.username ?? user.value?.username ?? '', role: j.role ?? 'member' };
  myMemberId.value = j.myMemberId ?? '';
  loaded.value = true;
}
export function loadState() {
  const t = localStorage.getItem(TOKEN_KEY);
  if (t) user.value = parseToken(t);
  refresh().catch(e => { if (e.message !== '未登录') toast('加载失败，请刷新重试'); });
}

export const toastMsg = ref('');
let toastTimer;
export function toast(m) {
  toastMsg.value = m;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (toastMsg.value = ''), 1600);
}

// 弹窗：{ title, html }，内容由调用方拼接（内部已转义用户输入）
export const modal = ref(null);
export function openModal(title, html) { modal.value = { title, html }; }
export function closeModal() { modal.value = null; }

async function request(path, body) {
  const opt = body
    ? { method: 'POST', headers: { 'Content-Type': 'application/json', ...authHeaders() }, body: JSON.stringify(body) }
    : { headers: authHeaders() };
  const r = await fetch('/api/' + path, opt);
  if (r.status === 401) { needLogin.value = true; throw new Error('未登录'); }
  const j = await r.json().catch(() => ({ ok: false, error: '网络错误' }));
  if (!j.ok) { toast(j.error || '操作失败'); throw new Error(j.error); }
  Object.assign(state, j.state);
}

// 调用后端操作，done 为成功后的额外回调（如清空表单）
export function call(path, body, done) {
  return request(path, body).then(() => { if (done) done(); });
}

export const fmt = t => new Date(t).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
export const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

export function memberRecords(id) {
  return state.records.filter(r => r.memberId === id).sort((a, b) => b.time - a.time);
}
export const sortedRecords = () => [...state.records].sort((a, b) => b.time - a.time);
export const ranked = () => [...state.members].sort((a, b) => b.score - a.score);

// 连续打卡展示：按成员使用各带奖励规则的计分次数计（被相关减分规则扣分后重新计数，与 worker 口径一致）
export function memberStreak(id) {
  const streakRules = state.rules.filter(r => (r.streaks?.length || r.streak));
  if (!streakRules.length) return 0;
  let best = 0;
  for (const r of streakRules) {
    const m = state.members.find(x => x.id === id);
    const after = (m?.streakResetAt && m.streakResetAt[r.name]) || 0;
    const n = state.records.filter(x => x.memberId === id && x.title === r.name && x.time > after).length;
    if (n > best) best = n;
  }
  return best;
}

// ---------- 业务操作 ----------
export function addMember(name, username, password, role) { return call('member/add', { name, username, password, role }); }
export function setMemberLogin(id, username, password, role) { return call('member/setLogin', { id, username, password, role }); }
export function setMemberHidden(m, hidden) { return call('member/hide', { id: m.id, hidden }); }
export function delMember(m) { if (confirm(`删除成员「${m.name}」及其全部记录？`)) return call('member/del', { id: m.id }); }
export function resetScore(m) {
  if (confirm(`将「${m.name}」的积分清零？`)) return call('member/reset', { id: m.id }).then(() => closeModal());
}
export function showMember(m) {
  const recs = memberRecords(m.id);
  openModal(`${m.name} · 当前 ${m.score} 分`, recs.length
    ? recs.slice(0, 50).map(r => `<div class="row"><div class="grow"><div class="name" style="font-size:14px;font-weight:500">${esc(r.title)}</div><div class="time">${fmt(r.time)}</div></div><div class="pts ${r.points >= 0 ? 'plus' : 'minus'}">${r.points >= 0 ? '+' : ''}${r.points} 分</div></div>`).join('')
      + (recs.length > 50 ? `<div class="empty">仅显示最近 50 条，共 ${recs.length} 条</div>` : '')
    : '<div class="empty">暂无记录</div>');
}
export function addRule(name, points, streaks) {
  return call('rule/add', { name, points, streaks });
}
export function editRule(id, name, points, streaks) {
  return call('rule/edit', { id, name, points, streaks });
}
export function delRule(id) { if (confirm('删除该计分规则？')) return call('rule/del', { id }); }
export function applyScore(member, rule) {
  return call('score/add', { memberId: member.id, ruleId: rule.id })
    .then(() => toast(`${member.name} ${rule.points >= 0 ? '加' : '减'} ${Math.abs(rule.points)} 分 ✓`));
}
export function applyCustomScore(member, title, points) {
  return call('score/custom', { memberId: member.id, title, points })
    .then(() => toast(`${member.name} ${points > 0 ? '加' : '减'} ${Math.abs(points)} 分 ✓`));
}
export function addItem(name, cost) { return call('item/add', { name, cost }); }
export function delItem(id) { if (confirm('删除该物品？')) return call('item/del', { id }); }
export function redeemModal(itemId) {
  const it = state.items.find(x => x.id === itemId);
  openModal(`兑换「${it.name}」`, state.members.length
    ? state.members.map(m => `<button class="rule-btn" onclick="window.__doRedeem('${m.id}','${itemId}')">${esc(m.name)} <span style="float:right" class="pts ${m.score < it.cost ? 'minus' : ''}">当前 ${m.score} 分</span></button>`).join('')
    : '<div class="empty">暂无成员</div>');
}
export function redeem(memberId, itemId) {
  closeModal();
  return call('item/redeem', { memberId, itemId }).then(() => toast('兑换成功 🎉'));
}
window.__doRedeem = redeem;
// 自助兑换：当前登录账号用自己绑定的成员积分兑换
export function redeemSelf(itemId) {
  const it = state.items.find(x => x.id === itemId);
  if (!it) return;
  return call('item/redeemSelf', { itemId }).then(() => toast(`兑换「${it.name}」成功 🎉`));
}
window.__doRedeemSelf = itemId => {
  const it = state.items.find(x => x.id === itemId);
  if (it && confirm(`确定用 ${it.cost} 积分兑换「${it.name}」？`)) redeemSelf(itemId);
};
