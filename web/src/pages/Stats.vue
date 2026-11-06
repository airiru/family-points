<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { store } from '../store'
import { toastErr } from '../toast'

const mode = ref('points')   // points | score
const range = ref('week')    // week | month
const ranking = ref([])
const trend = ref([])
const days = ref(30)
const loading = ref(true)

async function loadRanking() {
  if (!store.familyId) { loading.value = false; return }
  loading.value = true
  try {
    const d = await api(`/admin/family/${store.familyId}/stats/ranking?mode=${mode.value}&range=${range.value}`)
    ranking.value = d.ranking || []
  } catch (e) { toastErr(e) } finally { loading.value = false }
}

async function loadTrend() {
  if (!store.familyId) return
  try {
    const d = await api(`/admin/family/${store.familyId}/stats/trend?days=${days.value}`)
    trend.value = d.trend || []
  } catch (e) { toastErr(e) }
}

onMounted(() => { loadRanking(); loadTrend() })

function switchMode(m) { mode.value = m; loadRanking() }
function switchRange(r) { range.value = r; loadRanking() }

const maxTrend = computed(() => Math.max(1, ...trend.value.map(t => Math.max(t.earned, 0))))
const totalEarned = computed(() => trend.value.reduce((a, t) => a + t.earned, 0))
const maxRank = computed(() => Math.max(1, ...ranking.value.map(r => mode.value === 'points' ? r.score + r.frozen_score : r.period_score)))
const valOf = r => (mode.value === 'points' ? r.score + r.frozen_score : r.period_score)
</script>

<template>
  <div class="card" style="margin-bottom:16px">
    <div class="card-h">
      <h3>榜单</h3>
      <div class="row" style="margin-left:14px;gap:6px">
        <button class="btn sm" :class="{ ghost: mode !== 'points' }" @click="switchMode('points')">积分榜</button>
        <button class="btn sm" :class="{ ghost: mode !== 'score' }" @click="switchMode('score')">得分榜</button>
      </div>
      <div v-if="mode === 'score'" class="row" style="margin-left:10px;gap:6px">
        <button class="btn sm" :class="{ ghost: range !== 'week' }" @click="switchRange('week')">本周</button>
        <button class="btn sm" :class="{ ghost: range !== 'month' }" @click="switchRange('month')">本月</button>
      </div>
      <span class="sp"></span>
      <span class="hint">{{ mode === 'points' ? '积分榜 = 可用 + 冻结' : '得分榜 = 区间内赚到的正分（兑换不扣它）' }}</span>
    </div>
    <table>
      <thead><tr><th style="width:56px">名次</th><th>成员</th><th>{{ mode === 'points' ? '总积分' : '区间得分' }}</th><th>可用</th><th>冻结</th><th style="width:34%">对比</th></tr></thead>
      <tbody>
        <tr v-for="(r, i) in ranking" :key="r.member_id">
          <td class="num" :style="{ color: i === 0 ? 'var(--gold)' : '' }">{{ i + 1 }}</td>
          <td><div class="row"><div class="avatar">{{ r.avatar || r.nickname[0] }}</div><b>{{ r.nickname }}</b></div></td>
          <td class="num">{{ valOf(r) }}</td>
          <td class="num muted">{{ r.score }}</td>
          <td class="num" :style="{ color: r.frozen_score ? 'var(--gold)' : '' }">{{ r.frozen_score }}</td>
          <td>
            <div style="height:8px;background:#F1F3F8;border-radius:4px;overflow:hidden">
              <div :style="{ width: Math.max(4, valOf(r) / maxRank * 100) + '%', height: '100%', borderRadius: '4px', background: 'linear-gradient(90deg,#FF9F5A,#FF7A3D)' }"></div>
            </div>
          </td>
        </tr>
        <tr v-if="!loading && !ranking.length"><td colspan="6"><div class="empty">这个组还没有成员数据</div></td></tr>
      </tbody>
    </table>
  </div>

  <div class="card">
    <div class="card-h">
      <h3>积分趋势（全组）</h3><span class="sp"></span>
      <span class="hint">近 {{ days }} 天共赚 {{ totalEarned }} 分</span>
      <select class="input" style="width:110px;margin-left:10px" v-model.number="days" @change="loadTrend">
        <option :value="7">7 天</option><option :value="30">30 天</option><option :value="90">90 天</option>
      </select>
    </div>
    <div class="pad">
      <div v-if="trend.length" style="display:flex;align-items:flex-end;gap:4px;height:170px;padding-bottom:22px;position:relative">
        <div v-for="t in trend" :key="t.date" :title="`${t.date} 赚 ${t.earned} / 净 ${t.net}`"
             style="flex:1;max-width:46px;display:flex;flex-direction:column;justify-content:flex-end;height:100%;position:relative">
          <div :style="{ height: Math.max(2, t.earned / maxTrend * 100) + '%', background: 'linear-gradient(180deg,#FF9F5A,#FF7A3D)', borderRadius: '3px 3px 0 0' }"></div>
          <div style="position:absolute;bottom:-20px;left:0;right:0;text-align:center;font-size:10px;color:#9AA0AE;white-space:nowrap">{{ t.date.slice(5) }}</div>
        </div>
      </div>
      <div v-else class="empty" style="padding:30px">这段时间没有积分变动</div>
      <div class="hint" style="margin-top:10px">
        柱高 = 当天赚到的正分（<code>earned</code>）。鼠标悬停看当天净变动。数据按 <code>occurred_at</code> 聚合，
        所以补记的旧日期会落在它自己的那一天。
      </div>
    </div>
  </div>
</template>
