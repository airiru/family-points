<script setup>
// 后台设置页：成员管理（成员即账号）、计分规则、记一笔、兑换商城、全部记录
import { ref } from 'vue';
import {
  state, loaded, ranked, memberRecords, sortedRecords, call,
  addMember, delMember, resetScore, showMember, setMemberLogin,
  addRule, delRule, applyScore, applyCustomScore,
  addItem, delItem, redeemModal, fmt, memberStreak, user, toast,
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

const newRule = ref({ name: '', points: '', streakEvery: '', streakBonus: '' });
function doAddRule() {
  const n = newRule.value.name.trim(); const p = parseInt(newRule.value.points, 10);
  if (!n || isNaN(p)) return;
  addRule(n, p, newRule.value.streakEvery, newRule.value.streakBonus)
    .then(() => (newRule.value = { name: '', points: '', streakEvery: '', streakBonus: '' }));
}

const pickedMemberId = ref('');
const pickedMember = () => state.members.find(m => m.id === pickedMemberId.value);
const customScore = ref({ title: '', points: '' });
function doApplyScore(rule) {
  const m = pickedMember();
  if (!m) return;
  applyScore(m, rule);
}
function doApplyCustom() {
  const m = pickedMember();
  if (!m) return;
  const title = customScore.value.title.trim(); const points = parseInt(customScore.value.points, 10);
  if (!title || isNaN(points) || points === 0) return;
  applyCustomScore(m, title, points).then(() => (customScore.value = { title: '', points: '' }));
}

const newItem = ref({ name: '', cost: '' });
function doAddItem() {
  const n = newItem.value.name.trim(); const c = parseInt(newItem.value.cost, 10);
  if (!n || isNaN(c) || c <= 0) return;
  addItem(n, c).then(() => (newItem.value = { name: '', cost: '' }));
}

const recordFilter = ref('');
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
            <div class="sub">{{ memberRecords(m.id).length }} 条记录<template v-if="memberStreak(m.id) > 0"> · 🔥 连续 {{ memberStreak(m.id) }} 天</template> · {{ loginDesc(m) }}</div>
          </div>
          <div class="score">{{ m.score }} 分</div>
          <button class="btn ghost" @click="pickedMemberId = m.id; tab = 'score'">记一笔</button>
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
        <div class="desc">计分规则可自定义，正数为加分，负数为减分。可给规则附加"连续打卡奖励"：该规则每连续打卡 N 天自动额外奖 M 分（如：按时睡觉 +1，每连续 7 天再奖 2 分），中断后重新计数，一天内多次打卡不重复计天。</div>
        <div class="row" v-for="r in state.rules" :key="r.id">
          <div class="grow"><div class="name">{{ r.name }}</div><div class="sub" v-if="r.streak">🔥 连续打卡：每 {{ r.streak.every }} 天奖 {{ r.streak.bonus }} 分</div></div>
          <div class="pts" :class="r.points >= 0 ? 'plus' : 'minus'">{{ r.points >= 0 ? '+' : '' }}{{ r.points }} 分</div>
          <button class="btn del" @click="delRule(r.id)">删</button>
        </div>
        <div v-if="!state.rules.length" class="empty">还没有规则</div>
        <div class="form">
          <input v-model="newRule.name" placeholder="规则名称，如：按时睡觉" style="flex:1;min-width:120px" @keydown.enter="doAddRule">
          <input v-model="newRule.points" class="n" type="number" placeholder="±分值" @keydown.enter="doAddRule">
        </div>
        <div class="form" style="margin-top:8px">
          <input v-model="newRule.streakEvery" class="n" type="number" placeholder="每N天" title="可选：连续打卡奖励周期">
          <input v-model="newRule.streakBonus" class="n" type="number" placeholder="奖M分" title="可选：连续打卡奖励分值">
          <span class="sub" style="align-self:center">← 选填：连续打卡奖励</span>
          <button class="btn" @click="doAddRule">添加规则</button>
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
          </div>
          <template v-if="state.rules.length">
            <div class="form" style="margin-bottom:8px"></div>
            <button class="rule-btn" v-for="r in state.rules" :key="r.id" @click="doApplyScore(r)">
              <b>{{ r.name }}</b>
              <span class="pts" :class="r.points >= 0 ? 'plus' : 'minus'" style="float:right">{{ r.points >= 0 ? '+' : '' }}{{ r.points }} 分</span>
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
        <select v-model="recordFilter" style="width:100%;margin-bottom:10px">
          <option value="">全部成员</option>
          <option v-for="m in state.members" :key="m.id" :value="m.id">{{ m.name }}</option>
        </select>
        <div class="row" v-for="r in sortedRecords().filter(r => !recordFilter || r.memberId === recordFilter)" :key="r.time + r.memberId">
          <div class="grow">
            <div class="name" style="font-size:14px;font-weight:500">{{ r.name }} · {{ r.title }}</div>
            <div class="time">{{ fmt(r.time) }}</div>
          </div>
          <div class="pts" :class="r.points >= 0 ? 'plus' : 'minus'">{{ r.points >= 0 ? '+' : '' }}{{ r.points }} 分</div>
        </div>
        <div v-if="!sortedRecords().length" class="empty">暂无记录</div>
      </div>
    </template>
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
  </template>
</template>
