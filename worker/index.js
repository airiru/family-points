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
// 连续打卡按“连续点几次”计数：某成员使用该规则加分的次数即为连续次数，
// 每满 every 次自动奖励 bonus 分（奖励后计数继续累积）。
// 若成员被名称相关的减分规则扣分（如“不按时睡觉”之于“按时睡觉”），连续次数自动清零重新计。
function streakResetAt(m, ruleName) {
  return (m.streakResetAt && m.streakResetAt[ruleName]) || 0;
}
function streakCount(state, memberId, ruleName, after) {
  return state.records
    .filter(r => r.memberId === memberId && r.title === ruleName && r.time > after)
    .length;
}
// 名称相关 = 互相包含（“不按时睡觉”与“按时睡觉”相关）
const namesRelated = (a, b) => a.includes(b) || b.includes(a);

// 解析可选的补记日期（YYYY-MM-DD，按东八区当天中午计），不填或非法则用当前时间；不接受未来日期
function resolveTime(b) {
  if (typeof b.date === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(b.date)) {
    const t = Date.parse(b.date + 'T12:00:00+08:00');
    if (isNaN(t)) throw new Error('日期格式不正确');
    if (t > Date.now() + 86400e3) throw new Error('日期不能是未来的');
    return t;
  }
  return Date.now();
}

// 违反减分规则时，重置相关的连续计数并写入一条 0 分记录说明
function resetStreaksOnViolation(s, m, ruleName) {
  for (const rule of s.rules) {
    if (!(rule.streaks?.length || rule.streak)) continue;
    if (ruleName === rule.name || !namesRelated(ruleName, rule.name)) continue;
    if (!m.streakResetAt) m.streakResetAt = {};
    m.streakResetAt[rule.name] = Date.now();
    s.records.push({
      time: Date.now(), memberId: m.id, name: m.name, points: 0,
      title: `违反「${ruleName}」，「${rule.name}」连续打卡重新计数`,
    });
  }
}

// ---------- 数据与操作 ----------

const emptyState = () => ({ members: [], rules: [], items: [], records: [], secret: '' });
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
  state.records.forEach(r => {
    if (!r.id) r.id = uid();
    if (!r.addedAt) r.addedAt = Date.now(); // 实际添加时间（补记时与事项发生时间不同）
  });
  await kv.put(STATE_KEY, JSON.stringify(state));
}

const json = (data, status = 200) =>
  new Response(JSON.stringify(data), { status, headers: { 'content-type': 'application/json; charset=utf-8' } });

// ---------- 登录鉴权 ----------
// 账号即成员：成员可自带登录凭证（username + 加盐密码哈希，存于成员对象），
// 在后台「成员」中设置。首次使用时打开网页会引导创建管理员（也是一名成员）。

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

// token 形如 "username.exp.hmac(username.exp)"，无状态；角色以成员的当前配置为准
async function makeToken(username, secret) {
  const exp = Date.now() + TOKEN_DAYS * 86400e3;
  return `${username}.${exp}.${await hmac(secret, `${username}.${exp}`)}`;
}

// 返回 { username, memberId, role } 或 null
async function verifyToken(token, state) {
  if (!token || !state.secret) return null;
  const parts = token.split('.');
  if (parts.length !== 3) return null;
  const [username, exp, sig] = parts;
  if (Number(exp) < Date.now()) return null;
  const m = state.members.find(x => x.username === username);
  if (!m) return null;
  if ((await hmac(state.secret, `${username}.${exp}`)) !== sig) return null;
  return { username, memberId: m.id, role: m.role === 'admin' ? 'admin' : 'member' };
}

// 返回当前请求的用户信息；尚未初始化（没有任何可登录成员）时返回 null，由前端引导创建管理员
function currentUser(request, state) {
  if (!state.members.some(m => m.username)) return null;
  return verifyToken(request.headers.get('x-auth'), state);
}

// 对外输出的成员信息：去掉密码哈希等敏感字段
const publicMember = ({ salt, passHash, ...pub }) => pub;
const publicState = s => ({ members: s.members.map(publicMember), rules: s.rules, items: s.items, records: s.records });

const validUsername = u => /^[a-zA-Z0-9_-]{1,20}$/.test(u);

// 解析连续奖励档位：接受 streaks: [{every, bonus}, ...]，或旧的单档 streakEvery/streakBonus
function parseStreaks(b) {
  let list = Array.isArray(b.streaks) ? b.streaks : [];
  if (!list.length && parseInt(b.streakEvery, 10) > 0 && parseInt(b.streakBonus, 10) > 0) {
    list = [{ every: b.streakEvery, bonus: b.streakBonus }];
  }
  const tiers = list
    .map(t => ({ every: parseInt(t.every, 10), bonus: parseInt(t.bonus, 10) }))
    .filter(t => t.every > 0 && t.bonus !== 0 && !isNaN(t.every) && !isNaN(t.bonus))
    .sort((a, b2) => a.every - b2.every);
  // 去重：同一周期只保留一档
  return tiers.filter((t, i) => i === 0 || t.every !== tiers[i - 1].every);
}

// 设置成员登录凭证
async function applyLogin(m, username, password, role) {
  if (!username) { // 清除登录
    delete m.username; delete m.salt; delete m.passHash;
    m.role = 'member';
    return;
  }
  if (!validUsername(username)) throw new Error('用户名限 1-20 位字母、数字、_ 或 -');
  m.username = username;
  m.role = role === 'admin' ? 'admin' : 'member';
  if (password) {
    if (password.length < 4) throw new Error('密码至少 4 位');
    m.salt = uid() + uid();
    m.passHash = await hashPassword(password, m.salt);
  } else if (!m.passHash) {
    throw new Error('请设置密码（至少 4 位）');
  }
}

// 每个操作：接收 (state, body, user) => 同步/异步修改 state
const actions = {
  'member/add': async (s, b) => {
    const name = (b.name || '').trim();
    if (!name) throw new Error('请输入成员名字');
    if (s.members.some(m => m.name === name)) throw new Error('该成员已存在');
    const m = { id: uid(), name, score: 0, role: 'member' };
    if (b.username) {
      if (s.members.some(x => x.username === b.username)) throw new Error('该用户名已被使用');
      await applyLogin(m, b.username, b.password, b.role);
    }
    s.members.push(m);
  },
  'member/setLogin': async (s, b, user) => {
    const m = s.members.find(x => x.id === b.id);
    if (!m) throw new Error('成员不存在');
    const username = (b.username || '').trim();
    if (username && username !== m.username && s.members.some(x => x.username === username)) throw new Error('该用户名已被使用');
    await applyLogin(m, username, b.password, b.role ?? m.role);
    if (user && user.memberId === m.id && m.role !== 'admin') {
      const admins = s.members.filter(x => x.role === 'admin' && x.username);
      if (!admins.length) throw new Error('至少保留一个管理员');
    }
  },
  'member/del': (s, b, user) => {
    const m = s.members.find(x => x.id === b.id);
    if (!m) throw new Error('成员不存在');
    if (user && m.username === user.username) throw new Error('不能删除当前登录的成员');
    if (m.role === 'admin' && m.username && s.members.filter(x => x.role === 'admin' && x.username).length === 1) {
      throw new Error('至少保留一个管理员');
    }
    s.members = s.members.filter(x => x.id !== b.id);
    s.records = s.records.filter(r => r.memberId !== b.id);
  },
  'member/hide': (s, b) => {
    const m = s.members.find(x => x.id === b.id);
    if (!m) throw new Error('成员不存在');
    m.hidden = !!b.hidden;
  },
  'member/reset': (s, b) => {
    const m = s.members.find(x => x.id === b.id);
    if (!m) throw new Error('成员不存在');
    m.score = 0;
  },
  'rule/add': (s, b) => {
    const name = (b.name || '').trim();
    if (!name) throw new Error('请填写规则名称');
    const rule = { id: uid(), name };
    // 分值留空 = 灵活规则：记一笔时再填分值
    const points = parseInt(b.points, 10);
    if (b.points !== '' && b.points !== null && b.points !== undefined && !isNaN(points)) rule.points = points;
    else rule.flex = true;
    const streaks = parseStreaks(b);
    if (streaks.length) rule.streaks = streaks;
    s.rules.push(rule);
  },
  'rule/edit': (s, b) => {
    const r = s.rules.find(x => x.id === b.id);
    if (!r) throw new Error('规则不存在');
    const name = (b.name || '').trim();
    if (!name) throw new Error('请填写规则名称');
    r.name = name;
    const points = parseInt(b.points, 10);
    if (b.points !== '' && b.points !== null && b.points !== undefined && !isNaN(points)) { r.points = points; delete r.flex; }
    else { delete r.points; r.flex = true; }
    const streaks = parseStreaks(b);
    if (streaks.length) r.streaks = streaks;
    else delete r.streaks;
  },
  'rule/del': (s, b) => { s.rules = s.rules.filter(r => r.id !== b.id); },
  'score/add': (s, b) => {
    const m = s.members.find(x => x.id === b.memberId);
    const r = s.rules.find(x => x.id === b.ruleId);
    if (!m || !r) throw new Error('成员或规则不存在');
    // 灵活规则：分值由记一笔时传入；0 分允许（属于中性记录，不算扣分、不清连续）
    let pts = r.points;
    if (r.flex || pts === undefined) {
      pts = parseInt(b.points, 10);
      if (isNaN(pts)) throw new Error('请填写分值');
    }
    const time = resolveTime(b);
    m.score += pts;
    s.records.push({ time, memberId: m.id, name: m.name, title: r.name, points: pts });
    const bonusNotes = []; // 本次触发的连续奖励提示
    // 负分视为“违反”，相关的连续计数清零（灵活规则按本次填的分值判断）
    if (pts < 0) resetStreaksOnViolation(s, m, r.name);
    // 连续打卡奖励：规则可配置多档 streaks: [{every, bonus}]，各档独立计算——
    // 该规则累计打卡每满 every 次自动加该档 bonus 分（每个里程碑只奖励一次）
    if (r.streaks?.length || r.streak) {
      const tiers = r.streaks?.length ? r.streaks : [r.streak]; // 兼容旧的单档字段
      const from = streakResetAt(m, r.name); // 本轮清零时间：区分“清零前”与“清零后”的同一里程碑
      const count = streakCount(s, m.id, r.name, from);
      for (const tier of tiers) {
        if (count > 0 && count % tier.every === 0) {
          const alreadyAwarded = s.records.some(x =>
            x.memberId === m.id && x.streakAward && x.streakAward.every === tier.every
            && x.streakAward.n === count && (x.streakAward.from ?? 0) === from);
          if (!alreadyAwarded) {
            m.score += tier.bonus;
            s.records.push({
              time, memberId: m.id, name: m.name,
              title: `连续 ${count} 次「${r.name}」，奖励`, points: tier.bonus,
              streakAward: { every: tier.every, n: count, from },
            });
            bonusNotes.push(`连续 ${count} 次，奖励 +${tier.bonus} 分`);
          }
        }
      }
    }
    return { bonus: bonusNotes };
  },
  'score/custom': (s, b) => {
    const m = s.members.find(x => x.id === b.memberId);
    const points = parseInt(b.points, 10);
    const title = (b.title || '').trim();
    if (!m || isNaN(points) || !title) throw new Error('参数不完整');
    const time = resolveTime(b);
    m.score += points;
    s.records.push({ time, memberId: m.id, name: m.name, title, points });
    if (points < 0) resetStreaksOnViolation(s, m, title);
  },
  'item/add': (s, b) => {
    const name = (b.name || '').trim();
    const cost = parseInt(b.cost, 10);
    if (!name || isNaN(cost) || cost <= 0) throw new Error('请填写物品名称和正数积分');
    s.items.push({ id: uid(), name, cost });
  },
  'item/edit': (s, b) => {
    const it = s.items.find(x => x.id === b.id);
    if (!it) throw new Error('物品不存在');
    const name = (b.name || '').trim();
    const cost = parseInt(b.cost, 10);
    if (!name || isNaN(cost) || cost <= 0) throw new Error('请填写物品名称和正数积分');
    it.name = name;
    it.cost = cost;
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
  // 自助兑换：登录成员使用自己的积分兑换
  'item/redeemSelf': (s, b, user) => {
    const m = s.members.find(x => x.id === user.memberId);
    if (!m) throw new Error('当前登录成员不存在');
    const it = s.items.find(x => x.id === b.itemId);
    if (!it) throw new Error('物品不存在');
    if (m.score < it.cost) throw new Error(`你的积分不够（需要 ${it.cost}，当前 ${m.score}）`);
    m.score -= it.cost;
    s.records.push({ time: Date.now(), memberId: m.id, name: m.name, title: `兑换「${it.name}」`, points: -it.cost });
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
      // 未初始化（还没有任何可登录成员）：允许创建第一个管理员（同时是一名成员）
      if (request.method === 'POST' && path === 'setup') {
        const state = await getState(kv);
        if (state.members.some(m => m.username)) return json({ ok: false, error: '已初始化，请直接登录' }, 403);
        const body = await request.json().catch(() => ({}));
        const username = (body.username || '').trim();
        if (!validUsername(username)) return json({ ok: false, error: '用户名限 1-20 位字母、数字、_ 或 -' }, 400);
        if (!body.password || body.password.length < 4) return json({ ok: false, error: '密码至少 4 位' }, 400);
        if (!state.secret) state.secret = uid() + uid() + uid();
        const salt = uid() + uid();
        state.members.push({
          id: uid(), name: username, score: 0, role: 'admin',
          username, salt, passHash: await hashPassword(body.password, salt),
        });
        await saveState(kv, state);
        return json({ ok: true, token: await makeToken(username, state.secret), role: 'admin', username, state: publicState(state) });
      }

      // 登录接口不需要鉴权
      if (request.method === 'POST' && path === 'login') {
        const state = await getState(kv);
        if (!state.members.some(m => m.username)) return json({ ok: false, error: '请先完成初始化' }, 401);
        const body = await request.json().catch(() => ({}));
        const m = state.members.find(x => x.username === body.username);
        const passHash = m ? await hashPassword(body.password || '', m.salt) : '';
        if (!m || passHash !== m.passHash) return json({ ok: false, error: '用户名或密码错误' }, 401);
        return json({
          ok: true, token: await makeToken(m.username, state.secret),
          role: m.role === 'admin' ? 'admin' : 'member', username: m.username, state: publicState(state),
        });
      }

      const state = await getState(kv);

      // 其余接口需要登录（未初始化时前端会引导创建管理员）
      const user = await currentUser(request, state);
      if (!user) {
        return json({ ok: false, needSetup: !state.members.some(m => m.username), error: state.members.some(m => m.username) ? '未登录' : '请先完成初始化' }, 401);
      }

      if (request.method === 'GET' && path === 'state') {
        return json({
          ok: true, role: user.role, username: user.username, myMemberId: user.memberId,
          needSetup: false,
          state: publicState(state),
        });
      }
      if (request.method === 'POST') {
        // 普通成员仅允许自助兑换
        if (user.role !== 'admin' && path !== 'item/redeemSelf') {
          return json({ ok: false, error: '没有修改权限，请使用管理员账号' }, 403);
        }
        const handler = actions[path];
        if (!handler) return json({ ok: false, error: '未知操作' }, 404);
        const body = await request.json().catch(() => ({}));
        const extra = await handler(state, body, user); // 操作可返回附加信息（如连续奖励提示）
        await saveState(kv, state);
        return json({ ok: true, state: publicState(state), ...(extra || {}) });
      }
      return json({ ok: false, error: '不支持的请求' }, 405);
    } catch (e) {
      return json({ ok: false, error: e.message || '服务器错误' }, 400);
    }
  },
};
