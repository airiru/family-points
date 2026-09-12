<script setup>
// 后台设置页：成员管理（成员即账号）、计分规则、记一笔、兑换商城、全部记录
import { ref } from 'vue';
import RecordPager from '../components/RecordPager.vue';
import {
  state, loaded, ranked, memberRecords, sortedRecords, call,
  addMember, delMember, resetScore, showMember, setMemberLogin, setMemberHidden,
  addRule, delRule, editRule, applyScore, applyCustomScore,
  addItem, delItem, editItem, redeemModal, fmt, addedText, ptsText, ptsClass, memberStreak, user, toast,
} from '../store.js';

const tab = ref('members');
const TABS = [['members', '成员'], ['rules', '计分规则'], ['score', '记一笔'], ['items', '兑换商城'], ['records', '记录']];

const newMemberName = ref('');
function doAddMember() {
  const v = newMemberName.value.trim();
  if (!v) return;
  addMember(v).then(() => (newMemberName.value = ''));
}

// ---------- 成员登录设置（账号即成员）----------
const loginEdit = ref(null); // { id, name, username, password, role, hadLogin }
function doSetLogin(m) {
  loginEdit.value = {
    id: m.id, name: m.name,
    username: m.username || '', password: '',
    role: m.role === 'admin' ? 'admin' : 'member',
    hadLogin: !!m.username,
  };
}
function doSaveLogin() {
  const f = loginEdit.value;
  const username = f.username.trim();
  if (!username) {
    if (!f.hadLogin) return;
    if (!confirm(`清除「${f.name}」的登录？TA 将无法再登录。`)) return;
  } else if (!f.hadLogin || username !== f.username) {
    if (f.password.length < 4) return toast('密码至少 4 位');
  }
  setMemberLogin(f.id, username, f.password, f.role)
    .then(() => { toast('登录设置已保存 ✓'); loginEdit.value = null; })
    .catch(e => toast(e.message));
}
function loginDesc(m) {
  if (!m.username) return '未设置登录';
  return `登录：${m.username} · ${m.role === 'admin' ? '管理员' : '普通成员'}`;
}

const newRule = ref({ name: '', points: '', streaks: [] });
const ruleEdit = ref(null); // { id, name, points, streaks: [...] }

// 连续奖励档位：每档 { every: 每N次, bonus: 奖M分 }，可加多档
const emptyStreak = () => ({ every: '', bonus: '' });
function tiersOf(f) {
  return f.streaks
    .map(t => ({ every: parseInt(t.every, 10), bonus: parseInt(t.bonus, 10) }))
    .filter(t => t.every > 0 && t.bonus !== 0 && !isNaN(t.every) && !isNaN(t.bonus))
    .sort((a, b2) => a.every - b2.every);
}
function streakText(r) {
  const tiers = r.streaks?.length ? r.streaks : (r.streak ? [r.streak] : []);
  return tiers.map(t => `连续 ${t.every} 次 +${t.bonus} 分`).join(' · ');
}
function doAddRule() {
  const n = newRule.value.name.trim();
  if (!n) return;
  addRule(n, newRule.value.points, tiersOf(newRule.value))
    .then(() => (newRule.value = { name: '', points: '', streaks: [] }));
}
function doEditRule(r) {
  const tiers = r.streaks?.length ? r.streaks : (r.streak ? [r.streak] : []);
  ruleEdit.value = {
    id: r.id, name: r.name, points: r.points !== undefined ? String(r.points) : '',
    streaks: tiers.map(t => ({ every: String(t.every), bonus: String(t.bonus) })),
  };
}
function doSaveRule() {
  const f = ruleEdit.value;
  const n = f.name.trim();
  if (!n) return toast('请填写规则名称');
  editRule(f.id, n, f.points, tiersOf(f))
    .then(() => { toast('规则已保存 ✓'); ruleEdit.value = null; })
    .catch(e => toast(e.message));
}
// 规则显示的分值文案
function rulePointsText(r) {
  if (r.flex) return '灵活';
  return (r.points >= 0 ? '+' : '') + r.points + ' 分';
}

const pickedMemberId = ref('');
const pickedMember = () => state.members.find(m => m.id === pickedMemberId.value);
const customScore = ref({ title: '', points: '' });
const scoreDate = ref(''); // 可选补记日期（留空 = 今天）
function doApplyCustom() {
  const m = pickedMember();
  if (!m) return;
  const title = customScore.value.title.trim(); const points = parseInt(customScore.value.points, 10);
  if (!title || isNaN(points) || points === 0) return;
  applyCustomScore(m, title, points, scoreDate.value).then(() => (customScore.value = { title: '', points: '' }));
}

const newItem = ref({ name: '', cost: '' });
function doAddItem() {
  const n = newItem.value.name.trim(); const c = parseInt(newItem.value.cost, 10);
  if (!n || isNaN(c) || c <= 0) return;
  addItem(n, c).then(() => (newItem.value = { name: '', cost: '' }));
}

// ---------- 编辑兑换物品 ----------
const itemEdit = ref(null); // { id, name, cost }
function doEditItem(it) {
  itemEdit.value = { id: it.id, name: it.name, cost: String(it.cost) };
}
function doSaveItem() {
  const f = itemEdit.value;
  const n = f.name.trim(); const c = parseInt(f.cost, 10);
  if (!n || isNaN(c) || c <= 0) return toast('请填写物品名称和正数积分');
  editItem(f.id, n, c).then(() => { toast('物品已保存 ✓'); itemEdit.value = null; }).catch(e => toast(e.message));
}

const recordFilter = ref('');
// ---------- 记录分页 ----------
const recordPageSize = ref(15);
const recordPage = ref(1);
const filteredRecordList = () => sortedRecords().filter(r => !recordFilter.value || r.memberId === recordFilter.value);
const recordTotalPages = () => Math.max(1, Math.ceil(filteredRecordList().length / recordPageSize.value));
function pageRecords() {
  const list = filteredRecordList();
  return list.slice((recordPage.value - 1) * recordPageSize.value, recordPage.value * recordPageSize.value);
}

// ---------- 记一笔 ----------
const scoreInput = ref(null); // 灵活规则记一笔：{ member, rule, points }
function doApplyScore(rule) {
  const m = pickedMember();
  if (!m) return;
  if (rule.flex) { scoreInput.value = { member: m, rule, points: '', date: scoreDate.value }; return; }
  applyScore(m, rule, rule.points, scoreDate.value);
}
function doSaveScoreInput() {
  const f = scoreInput.value;
  const p = parseInt(f.points, 10);
  if (isNaN(p) || p === 0) return toast('请填写不为 0 的分值');
  applyScore(f.member, f.rule, p, f.date).then(() => (scoreInput.value = null));
}
</script>

<template>
  <div v-if="!loaded" class="card"><div class="empty">加载中…</div></div>
  <template v-else>
    <div class="tabs">
      <button v-for="[k, label] in TABS" :key="k" :class="{ on: tab === k }" @click="tab = k">{{ label }}</button>
    </div>

    <!-- 成员管理（账号即成员） -->
    <template v-if="tab === 'members'">
      <div class="card" v-for="m in ranked()" :key="m.id">
        <div class="row">
          <div class="grow clickable" @click="showMember(m)">
            <div class="name">{{ m.name }} ›<span v-if="m.username === user?.username" class="sub" style="display:inline">（当前登录）</span></div>
            <div class="sub">{{ memberRecords(m.id).length }} 条记录<template v-if="memberStreak(m.id) > 0"> · 🔥 连续 {{ memberStreak(m.id) }} 次</template> · {{ loginDesc(m) }}</div>
          </div>
          <div class="score">{{ m.score }} 分</div>
          <button class="btn ghost" @click="pickedMemberId = m.id; tab = 'score'">记一笔</button>
          <button class="btn ghost" @click="setMemberHidden(m, !m.hidden)">{{ m.hidden ? '显示' : '隐藏' }}</button>
          <button class="btn ghost" @click="doSetLogin(m)">登录</button>
          <button class="btn ghost" @click="resetScore(m)">清零</button>
          <button class="btn del" @click="delMember(m)">删</button>
        </div>
      </div>
      <div v-if="!state.members.length" class="card"><div class="empty">还没有成员，先添加一个吧</div></div>
      <div class="card">
        <div class="desc">账号就是成员：给成员设置登录后，TA 可以用用户名密码登录，查看自己的积分并用积分自助兑换物品。设为管理员的成员还能进入后台修改数据。不需要登录的成员可以不设置。</div>
        <div class="form">
          <input v-model="newMemberName" placeholder="成员名字" style="flex:1" @keydown.enter="doAddMember">
          <button class="btn" @click="doAddMember">添加成员</button>
        </div>
      </div>
    </template>

    <!-- 计分规则 -->
    <template v-if="tab === 'rules'">
      <div class="card">
        <div class="desc">计分规则可自定义，正数为加分，负数为减分。可给规则附加"连续打卡奖励"：该规则每累计打卡 N 次自动额外奖 M 分（如：按时睡觉 +1，每满 7 次再奖 2 分），按点加分次数连续计数。若成员被名称相关的减分规则扣分（如"不按时睡觉"扣分），对应连续次数自动清零重新计。</div>
        <div class="row" v-for="r in state.rules" :key="r.id">
          <div class="grow"><div class="name">{{ r.name }}</div><div class="sub" v-if="streakText(r)">🔥 {{ streakText(r) }}</div></div>
          <div class="pts" :class="!r.flex && r.points >= 0 ? 'plus' : 'minus'">{{ rulePointsText(r) }}</div>
          <button class="btn ghost" @click="doEditRule(r)">编辑</button>
          <button class="btn del" @click="delRule(r.id)">删</button>
        </div>
        <div v-if="!state.rules.length" class="empty">还没有规则</div>
        <div class="form">
          <input v-model="newRule.name" placeholder="规则名称，如：按时睡觉" style="flex:1;min-width:120px" @keydown.enter="doAddRule">
          <input v-model="newRule.points" class="n" type="number" placeholder="±分值" @keydown.enter="doAddRule">
        </div>
        <div class="sub" style="margin-top:6px">±分值留空则为"灵活规则"：规则只定项目，记一笔时再填这次的分值。</div>
        <div class="form" style="margin-top:8px;align-items:center" v-for="(t, i) in newRule.streaks" :key="i">
          <span class="sub" style="white-space:nowrap">连续</span>
          <input v-model="t.every" class="n" type="number" placeholder="N次">
          <span class="sub" style="white-space:nowrap">次奖</span>
          <input v-model="t.bonus" class="n" type="number" placeholder="M分值">
          <button class="btn del" @click="newRule.streaks.splice(i, 1)">删</button>
        </div>
        <div style="margin-top:8px">
          <button class="btn ghost" @click="newRule.streaks.push(emptyStreak())">+ 添加连续打卡奖励档位（选填）</button>
          <button class="btn" style="margin-left:8px" @click="doAddRule">添加规则</button>
        </div>
      </div>
    </template>

    <!-- 记一笔 -->
    <template v-if="tab === 'score'">
      <div v-if="!state.members.length" class="card"><div class="empty">请先在「成员」页添加成员</div></div>
      <template v-else>
        <div class="card">
          <div class="form" style="margin:0">
            <select v-model="pickedMemberId" style="flex:1">
              <option value="" disabled>选择成员</option>
              <option v-for="m in state.members" :key="m.id" :value="m.id">{{ m.name }}（{{ m.score }} 分）</option>
            </select>
            <input v-model="scoreDate" type="date" title="事项发生的日期（留空 = 今天）">
          </div>
          <div class="sub" style="margin-top:6px">日期留空默认记为今天；选过去的日期可补记。</div>
          <template v-if="state.rules.length">
            <div class="form" style="margin-bottom:8px"></div>
            <button class="rule-btn" v-for="r in state.rules" :key="r.id" @click="doApplyScore(r)">
              <b>{{ r.name }}</b>
              <span class="pts" :class="!r.flex && r.points >= 0 ? 'plus' : 'minus'" style="float:right">{{ rulePointsText(r) }}</span>
              <div class="sub" v-if="streakText(r)">🔥 {{ streakText(r) }}</div>
            </button>
          </template>
          <div v-else class="empty">还没有计分规则，可用下方自定义记一笔</div>
        </div>
        <div class="card">
          <div class="desc">自定义记一笔：不在规则里的临时事项，直接填名称和分值。</div>
          <div class="form">
            <input v-model="customScore.title" placeholder="事项，如：主动做家务" style="flex:1;min-width:140px">
            <input v-model="customScore.points" class="n" type="number" placeholder="±分值">
            <button class="btn" @click="doApplyCustom">记一笔</button>
          </div>
        </div>
      </template>
    </template>

    <!-- 兑换商城 -->
    <template v-if="tab === 'items'">
      <div class="card">
        <div class="desc">自定义可兑换的物品和所需积分，成员用积分兑换。</div>
        <div class="row" v-for="it in state.items" :key="it.id">
          <div class="grow"><div class="name">🎁 {{ it.name }}</div><div class="sub">需要 {{ it.cost }} 积分</div></div>
          <button class="btn ghost" @click="doEditItem(it)">编辑</button>
          <button class="btn ghost" @click="redeemModal(it.id)">兑换</button>
          <button class="btn del" @click="delItem(it.id)">删</button>
        </div>
        <div v-if="!state.items.length" class="empty">还没有物品</div>
        <div class="form">
          <input v-model="newItem.name" placeholder="物品名称，如：冰淇淋" style="flex:1" @keydown.enter="doAddItem">
          <input v-model="newItem.cost" class="n" type="number" placeholder="积分" @keydown.enter="doAddItem">
          <button class="btn" @click="doAddItem">添加物品</button>
        </div>
      </div>
    </template>

    <!-- 全部记录 -->
    <template v-if="tab === 'records'">
      <div class="card">
        <select v-model="recordFilter" style="width:100%;margin-bottom:10px" @change="recordPage = 1">
          <option value="">全部成员</option>
          <option v-for="m in state.members" :key="m.id" :value="m.id">{{ m.name }}</option>
        </select>
        <div class="row" v-for="r in pageRecords()" :key="r.id || (r.time + r.memberId)">
          <div class="grow">
            <div class="name" style="font-size:14px;font-weight:500">{{ r.name }} · {{ r.title }}</div>
            <div class="time">{{ fmt(r.time) }}<template v-if="addedText(r)"> · {{ addedText(r) }}</template></div>
          </div>
          <div class="pts" :class="ptsClass(r)">{{ ptsText(r) }}</div>
        </div>
        <div v-if="!filteredRecordList().length" class="empty">暂无记录</div>
        <RecordPager v-if="filteredRecordList().length" v-model:page="recordPage" v-model:page-size="recordPageSize" :total-pages="recordTotalPages()" />
      </div>
    </template>
    <!-- 灵活规则记一笔：填写本次分值 -->
    <div class="modal-bg" :class="{ show: scoreInput }" @click.self="scoreInput = null">
      <div class="modal" v-if="scoreInput">
        <h3>「{{ scoreInput.rule.name }}」— {{ scoreInput.member.name }}</h3>
        <div class="form" style="flex-direction:column;align-items:stretch">
          <input v-model="scoreInput.points" type="number" placeholder="这次的分值（正数加分、负数减分）" @keydown.enter="doSaveScoreInput">
          <div style="display:flex;gap:8px;margin-top:4px">
            <button class="btn" style="flex:1" @click="doSaveScoreInput">确定</button>
            <button class="btn ghost" style="flex:1" @click="scoreInput = null">取消</button>
          </div>
        </div>
      </div>
    </div>
    <!-- 编辑规则表单 -->
    <div class="modal-bg" :class="{ show: ruleEdit }" @click.self="ruleEdit = null">
      <div class="modal modal-wide" v-if="ruleEdit">
        <h3>编辑规则</h3>
        <div class="form" style="flex-direction:column;align-items:stretch">
          <input v-model="ruleEdit.name" placeholder="规则名称" @keydown.enter="doSaveRule">
          <input v-model="ruleEdit.points" type="number" placeholder="±分值（留空为灵活规则，记一笔时再填）" @keydown.enter="doSaveRule">
          <div class="sub" style="margin:4px 0 0">连续打卡奖励档位（选填，可加多档，各档独立计算）：</div>
          <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap" v-for="(t, i) in ruleEdit.streaks" :key="i">
            <span class="sub" style="white-space:nowrap">连续</span>
            <input v-model="t.every" type="number" placeholder="N次" style="flex:1 1 80px;min-width:0">
            <span class="sub" style="white-space:nowrap">次奖</span>
            <input v-model="t.bonus" type="number" placeholder="M分" style="flex:1 1 80px;min-width:0">
            <button class="btn del" @click="ruleEdit.streaks.splice(i, 1)">删</button>
          </div>
          <button class="btn ghost" @click="ruleEdit.streaks.push(emptyStreak())">+ 添加档位</button>
          <div style="display:flex;gap:8px;margin-top:4px">
            <button class="btn" style="flex:1" @click="doSaveRule">保存</button>
            <button class="btn ghost" style="flex:1" @click="ruleEdit = null">取消</button>
          </div>
        </div>
      </div>
    </div>
    <!-- 成员登录设置表单 -->
    <div class="modal-bg" :class="{ show: loginEdit }" @click.self="loginEdit = null">
      <div class="modal" v-if="loginEdit">
        <h3>「{{ loginEdit.name }}」的登录设置</h3>
        <div class="form" style="flex-direction:column;align-items:stretch">
          <input v-model="loginEdit.username" placeholder="用户名（限 1-20 位字母、数字、_、-）">
          <input v-model="loginEdit.password" type="password" :placeholder="loginEdit.hadLogin ? '新密码（留空则不修改）' : '密码（至少 4 位）'">
          <select v-model="loginEdit.role">
            <option value="member">普通成员（只能看积分和自己兑换）</option>
            <option value="admin">管理员（可进后台修改）</option>
          </select>
          <div style="display:flex;gap:8px;margin-top:4px">
            <button class="btn" style="flex:1" @click="doSaveLogin">保存</button>
            <button v-if="loginEdit.hadLogin" class="btn del" style="flex:1" @click="loginEdit.username = ''; doSaveLogin()">清除登录</button>
            <button class="btn ghost" style="flex:1" @click="loginEdit = null">取消</button>
          </div>
        </div>
      </div>
    </div>
    <!-- 编辑兑换物品 -->
    <div class="modal-bg" :class="{ show: itemEdit }" @click.self="itemEdit = null">
      <div class="modal" v-if="itemEdit">
        <h3>编辑物品</h3>
        <div class="form" style="flex-direction:column;align-items:stretch">
          <input v-model="itemEdit.name" placeholder="物品名称" @keydown.enter="doSaveItem">
          <input v-model="itemEdit.cost" type="number" placeholder="所需积分（正数）" @keydown.enter="doSaveItem">
          <div style="display:flex;gap:8px;margin-top:4px">
            <button class="btn" style="flex:1" @click="doSaveItem">保存</button>
            <button class="btn ghost" style="flex:1" @click="itemEdit = null">取消</button>
          </div>
        </div>
      </div>
    </div>
  </template>
</template>
