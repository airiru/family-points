<script setup>
// 主页面：只读展示积分排行榜和成员详情、最近记录；绑定成员的账号可自助兑换
import { ref, computed } from 'vue';
import { state, loaded, ranked, memberRecords, sortedRecords, showMember, fmt, memberStreak, user, myMemberId, redeemSelf, esc } from '../store.js';

const visibleMembers = computed(() => state.members.filter(m => !m.hidden));
const PAGE_SIZE = 15;
const page = ref(1);
const recent = computed(() => {
  const ids = new Set(visibleMembers.value.map(m => m.id));
  return sortedRecords().filter(r => ids.has(r.memberId));
});
const detailId = ref(''); // 记录区按成员筛选
const filteredRecent = computed(() => recent.value.filter(r => !detailId.value || r.memberId === detailId.value));
const totalPages = computed(() => Math.max(1, Math.ceil(filteredRecent.value.length / PAGE_SIZE)));
const pageRecords = computed(() => filteredRecent.value.slice((page.value - 1) * PAGE_SIZE, page.value * PAGE_SIZE));
function gotoPage(p) { page.value = Math.min(Math.max(1, p), totalPages.value); }
const myMember = computed(() => state.members.find(m => m.id === myMemberId.value));
function onRedeem(it) {
  if (confirm(`确定用 ${it.cost} 积分兑换「${it.name}」？`)) redeemSelf(it.id);
}
</script>

<template>
  <div v-if="!loaded" class="card"><div class="empty">加载中…</div></div>
  <template v-else>
    <div class="card" v-for="(m, i) in ranked().filter(m => !m.hidden)" :key="m.id">
      <div class="row">
        <div style="font-size:18px;width:26px;text-align:center">{{ ['🥇', '🥈', '🥉'][i] || (i + 1) }}</div>
        <div class="grow clickable" @click="showMember(m)">
          <div class="name">{{ m.name }} ›</div>
          <div class="sub">{{ memberRecords(m.id).length }} 条记录<template v-if="memberStreak(m.id) > 0"> · 🔥 已连续 {{ memberStreak(m.id) }} 次</template></div>
        </div>
        <div class="score">{{ m.score }} 分</div>
      </div>
    </div>
    <div v-if="!state.members.some(m => !m.hidden)" class="card"><div class="empty">还没有成员，请到「后台设置」添加</div></div>

    <!-- 我的兑换：当前账号绑定的成员用自己积分兑换 -->
    <div class="card" v-if="myMember">
      <div class="desc">{{ myMember.name }} 的兑换商城（当前 {{ myMember.score }} 积分）</div>
      <div class="row" v-for="it in state.items" :key="it.id">
        <div class="grow"><div class="name">🎁 {{ it.name }}</div><div class="sub">需要 {{ it.cost }} 积分</div></div>
        <button class="btn" :disabled="myMember.score < it.cost" @click="onRedeem(it)">
          {{ myMember.score < it.cost ? '积分不足' : '兑换' }}
        </button>
      </div>
      <div v-if="!state.items.length" class="empty">商城还没有物品，请管理员在后台添加</div>
    </div>


    <div class="card">
      <div class="desc">最近记录</div>
      <select v-model="detailId" style="width:100%;margin-bottom:10px" @change="page = 1">
        <option value="">全部成员</option>
        <option v-for="m in visibleMembers" :key="m.id" :value="m.id">{{ m.name }}</option>
      </select>
      <div class="row" v-for="r in pageRecords" :key="r.id || (r.time + r.memberId)">
        <div class="grow">
          <div class="name" style="font-size:14px;font-weight:500">{{ r.name }} · {{ r.title }}</div>
          <div class="time">{{ fmt(r.time) }}</div>
        </div>
        <div class="pts" :class="r.points >= 0 ? 'plus' : 'minus'">{{ r.points >= 0 ? '+' : '' }}{{ r.points }} 分</div>
      </div>
      <div v-if="!filteredRecent.length" class="empty">暂无记录</div>
      <div class="pager" v-if="totalPages > 1">
        <button class="btn ghost" :disabled="page <= 1" @click="gotoPage(page - 1)">上一页</button>
        <span class="sub">{{ page }} / {{ totalPages }}</span>
        <button class="btn ghost" :disabled="page >= totalPages" @click="gotoPage(page + 1)">下一页</button>
      </div>
    </div>
  </template>
</template>
