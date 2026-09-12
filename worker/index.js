/**
 * 家庭积分榜 — Cloudflare Worker（数据接口）
 *
 * 部署配置见 wrangler.jsonc。Vue 前端由 Vite 构建到 dist/ 并由 Workers Static Assets 托管，
 * 本函数只处理 /api/* 数据请求（未命中静态资源的请求才会进入这里）。
 * 数据保存在 Cloudflare KV 中（binding 名为 KV，见 wrangler.jsonc 的 kv_namespaces）。
 */

const STATE_KEY = 'state';
const MAX_RECORDS = 500; // 历史记录上限，超出后丢弃最旧的
const TOKEN_DAYS = 30; // 登录有效期（天）

// 连续打卡奖励：规则可自带 streak: { every, bonus }（在后台设置页配置），
// 该规则每连续打卡 every 天自动额外奖励 bonus 分
const TZ_OFFSET = 8 * 3600e3; // “同一天”按东八区计算
const dayKey = t => new Date(t + TZ_OFFSET).toISOString().slice(0, 10);

// 成员当前连续打卡：返回 { n: 连续天数, from: 本轮起始日的日期键 }（截至今天或昨天）
function streak(state, memberId, ruleName) {
  const days = new Set(state.records
    .filter(r => r.memberId === memberId && r.title === ruleName)
    .map(r => dayKey(r.time)));
  let t = Date.now();
  if (!days.has(dayKey(t))) t -= 86400e3;
  let n = 0;
  while (days.has(dayKey(t))) { n++; t -= 86400e3; }
  return { n, from: n ? dayKey(t + 86400e3) : '' };
}

// ---------- 数据与操作 ----------

const emptyState = () => ({ members: [], rules: [], items: [], records: [], accounts: [], secret: '' });
const uid = () => Date.now().toString(36) + Math.random().toString(36).slice(2, 7);

async function getState(kv) {
  const raw = await kv.get(STATE_KEY);
  try {
    const s = raw ? JSON.parse(raw) : null;
    return { ...emptyState(), ...(s && typeof s === 'object' ? s : {}) };
  } catch { return emptyState(); }
}

async function saveState(kv, state) {
  state.records = state.records.slice(-MAX_RECORDS);
  await kv.put(STATE_KEY, JSON.stringify(state));
}

const json = (data, status = 200) =>
  new Response(JSON.stringify(data), { status, headers: { 'content-type': 'application/json; charset=utf-8' } });

// ---------- 登录鉴权 ----------
// 账号保存在 KV 的 state.accounts 里（密码加盐哈希），在后台设置页「账号」标签中管理。
// 首次使用（还没有任何账号）时，打开网页会先要求创建管理员账号，签名密钥也随之自动生成。

const te = s => new TextEncoder().encode(s);

async function hmac(secret, msg) {
  const key = await crypto.subtle.importKey('raw', te(secret), { name: 'HMAC', hash: 'SHA-256' }, false, ['sign']);
  const sig = await crypto.subtle.sign('HMAC', key, te(msg));
  return btoa(String.fromCharCode(...new Uint8Array(sig))).replace(/=+$/, '');
}

async function hashPassword(password, salt) {
  const d = await crypto.subtle.digest('SHA-256', te(`${salt}:${password}`));
  return btoa(String.fromCharCode(...new Uint8Array(d)));
}

// token 形如 "username.exp.hmac(username.exp)"，无状态；角色以 KV 中账号的当前配置为准
async function makeToken(username, secret) {
  const exp = Date.now() + TOKEN_DAYS * 86400e3;
  return `${username}.${exp}.${await hmac(secret, `${username}.${exp}`)}`;
}

// 返回 { username, role } 或 null
async function verifyToken(token, state) {
  if (!token || !state.secret) return null;
  const parts = token.split('.');
  if (parts.length !== 3) return null;
  const [username, exp, sig] = parts;
  if (Number(exp) < Date.now()) return null;
  const acc = state.accounts.find(a => a.username === username);
  if (!acc) return null;
  if ((await hmac(state.secret, `${username}.${exp}`)) !== sig) return null;
  return { username, role: acc.role === 'admin' ? 'admin' : 'viewer' };
}

// 返回当前请求的用户信息；未初始化（无账号）时返回 null，由前端引导创建管理员
async function currentUser(request, state) {
  if (!state.accounts.length) return null;
  return verifyToken(request.headers.get('x-auth'), state);
}

const publicState = s => ({ members: s.members, rules: s.rules, items: s.items, records: s.records });

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
    const rule = { id: uid(), name, points };
    // 可选连续打卡奖励：每 every 天奖 bonus 分（两项都填正整数才生效）
    const every = parseInt(b.streakEvery, 10);
    const bonus = parseInt(b.streakBonus, 10);
    if (every > 0 && bonus > 0) rule.streak = { every, bonus };
    s.rules.push(rule);
  },
  'rule/del': (s, b) => { s.rules = s.rules.filter(r => r.id !== b.id); },
  'score/add': (s, b) => {
    const m = s.members.find(x => x.id === b.memberId);
    const r = s.rules.find(x => x.id === b.ruleId);
    if (!m || !r) throw new Error('成员或规则不存在');
    m.score += r.points;
    s.records.push({ time: Date.now(), memberId: m.id, name: m.name, title: r.name, points: r.points });
    // 连续打卡奖励：该规则配置了 streak 时，每达到 every 的整数倍天数自动加 bonus 分
    //（每轮每个里程碑只奖励一次）
    if (r.streak) {
      const st = streak(s, m.id, r.name);
      const milestone = st.n > 0 && st.n % r.streak.every === 0;
      const alreadyAwarded = s.records.some(x =>
        x.memberId === m.id && x.streakAward && x.streakAward.n === st.n && x.streakAward.from === st.from);
      if (milestone && !alreadyAwarded) {
        m.score += r.streak.bonus;
        s.records.push({
          time: Date.now(), memberId: m.id, name: m.name,
          title: `连续 ${st.n} 天「${r.name}」，奖励`, points: r.streak.bonus,
          streakAward: { n: st.n, from: st.from },
        });
      }
    }
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
  // 自助兑换：登录账号使用自己绑定的成员的积分兑换
  'item/redeemSelf': (s, b, user) => {
    const acc = s.accounts.find(a => a.username === user.username);
    const m = acc && s.members.find(x => x.id === acc.memberId);
    if (!m) throw new Error('当前账号未绑定成员，请联系管理员在「账号」中绑定');
    const it = s.items.find(x => x.id === b.itemId);
    if (!it) throw new Error('物品不存在');
    if (m.score < it.cost) throw new Error(`你的积分不够（需要 ${it.cost}，当前 ${m.score}）`);
    m.score -= it.cost;
    s.records.push({ time: Date.now(), memberId: m.id, name: m.name, title: `兑换「${it.name}」`, points: -it.cost });
  },
  // ---------- 账号管理（仅管理员）----------
  'account/add': async (s, b) => {
    const username = (b.username || '').trim();
    const password = b.password || '';
    if (!username || !/^[a-zA-Z0-9_-]{1,20}$/.test(username)) throw new Error('用户名限 1-20 位字母、数字、_ 或 -');
    if (password.length < 4) throw new Error('密码至少 4 位');
    if (s.accounts.some(a => a.username === username)) throw new Error('该用户名已存在');
    const salt = uid() + uid();
    const acc = { username, salt, passHash: await hashPassword(password, salt), role: b.role === 'admin' ? 'admin' : 'viewer' };
    if (b.memberId) {
      if (!s.members.some(m => m.id === b.memberId)) throw new Error('绑定的成员不存在');
      if (s.accounts.some(a => a.memberId === b.memberId)) throw new Error('该成员已绑定其他账号');
      acc.memberId = b.memberId;
    }
    s.accounts.push(acc);
  },
  'account/bind': (s, b, user) => {
    const acc = s.accounts.find(a => a.username === b.username);
    if (!acc) throw new Error('账号不存在');
    if (!b.memberId) { delete acc.memberId; return; }
    if (!s.members.some(m => m.id === b.memberId)) throw new Error('绑定的成员不存在');
    if (s.accounts.some(a => a.memberId === b.memberId && a.username !== acc.username)) throw new Error('该成员已绑定其他账号');
    acc.memberId = b.memberId;
  },
  'account/del': (s, b, user) => {
    const acc = s.accounts.find(a => a.username === b.username);
    if (!acc) throw new Error('账号不存在');
    if (acc.username === user.username) throw new Error('不能删除自己的账号');
    const admins = s.accounts.filter(a => a.role === 'admin');
    if (acc.role === 'admin' && admins.length === 1) throw new Error('至少保留一个管理员账号');
    s.accounts = s.accounts.filter(a => a.username !== b.username);
  },
  'account/role': (s, b, user) => {
    const acc = s.accounts.find(a => a.username === b.username);
    if (!acc) throw new Error('账号不存在');
    const role = b.role === 'admin' ? 'admin' : 'viewer';
    if (acc.role === 'admin' && role !== 'admin' && s.accounts.filter(a => a.role === 'admin').length === 1) {
      throw new Error('至少保留一个管理员账号');
    }
    acc.role = role;
  },
  'account/pass': async (s, b) => {
    const acc = s.accounts.find(a => a.username === b.username);
    if (!acc) throw new Error('账号不存在');
    if (!b.password || b.password.length < 4) throw new Error('密码至少 4 位');
    acc.salt = uid() + uid();
    acc.passHash = await hashPassword(b.password, acc.salt);
  },
};

export default {
  async fetch(request, env) {
    const url = new URL(request.url);

    if (!url.pathname.startsWith('/api/')) {
      // 正常情况下静态资源由 Workers Static Assets 托管，这里兜底
      return new Response('Not Found', { status: 404 });
    }

    const kv = env.KV;
    const path = url.pathname.slice(5); // 去掉 /api/

    try {
      // 未初始化（还没有任何账号）：允许创建第一个管理员账号
      if (request.method === 'POST' && path === 'setup') {
        const state = await getState(kv);
        if (state.accounts.length) return json({ ok: false, error: '已初始化，请直接登录' }, 403);
        const body = await request.json().catch(() => ({}));
        const username = (body.username || '').trim();
        if (!username || !/^[a-zA-Z0-9_-]{1,20}$/.test(username)) return json({ ok: false, error: '用户名限 1-20 位字母、数字、_ 或 -' }, 400);
        if (!body.password || body.password.length < 4) return json({ ok: false, error: '密码至少 4 位' }, 400);
        if (!state.secret) state.secret = uid() + uid() + uid();
        const salt = uid() + uid();
        state.accounts.push({ username, salt, passHash: await hashPassword(body.password, salt), role: 'admin' });
        await saveState(kv, state);
        return json({ ok: true, token: await makeToken(username, state.secret), role: 'admin', username, state: publicState(state) });
      }

      // 登录接口不需要鉴权
      if (request.method === 'POST' && path === 'login') {
        const state = await getState(kv);
        if (!state.accounts.length) return json({ ok: false, error: '请先完成初始化' }, 401);
        const body = await request.json().catch(() => ({}));
        const acc = state.accounts.find(a => a.username === body.username);
        const passHash = acc ? await hashPassword(body.password || '', acc.salt) : '';
        if (!acc || passHash !== acc.passHash) return json({ ok: false, error: '用户名或密码错误' }, 401);
        return json({
          ok: true, token: await makeToken(acc.username, state.secret),
          role: acc.role === 'admin' ? 'admin' : 'viewer', username: acc.username, state: publicState(state),
        });
      }

      const state = await getState(kv);

      // 其余接口需要登录（未初始化时前端会引导创建管理员）
      const user = await currentUser(request, state);
      if (!user) {
        return json({ ok: false, needSetup: !state.accounts.length, error: state.accounts.length ? '未登录' : '请先完成初始化' }, 401);
      }

      if (request.method === 'GET' && path === 'state') {
        const acc = state.accounts.find(a => a.username === user.username);
        return json({
          ok: true, role: user.role, username: user.username,
          myMemberId: acc?.memberId ?? '',
          needSetup: false,
          state: publicState(state),
          accounts: user.role === 'admin'
            ? state.accounts.map(a => ({ username: a.username, role: a.role, memberId: a.memberId ?? '' }))
            : undefined,
        });
      }
      if (request.method === 'POST') {
        // 查看者仅允许自助兑换
        if (user.role !== 'admin' && path !== 'item/redeemSelf') {
          return json({ ok: false, error: '没有修改权限，请使用管理员账号' }, 403);
        }
        const handler = actions[path];
        if (!handler) return json({ ok: false, error: '未知操作' }, 404);
        const body = await request.json().catch(() => ({}));
        await handler(state, body, user);
        await saveState(kv, state);
        return json({ ok: true, state: publicState(state) });
      }
      return json({ ok: false, error: '不支持的请求' }, 405);
    } catch (e) {
      return json({ ok: false, error: e.message || '服务器错误' }, 400);
    }
  },
};
