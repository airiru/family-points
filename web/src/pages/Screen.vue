<script setup>
import { onMounted, ref } from 'vue'
import { api, fmtDate, fmtTime } from '../api'
import { store } from '../store'
import { toastErr } from '../toast'

const list = ref([])
const members = ref([])
const loading = ref(true)
const memberId = ref(0)
const limit = ref(50)

const nameOf = id => (members.value.find(m => m.id === id) || {}).name || ('#' + id)

async function load() {
  if (!store.familyId) { loading.value = false; return }
  loading.value = true
  try {
    const q = `member_id=${memberId.value}&limit=${limit.value}`
    const [r, m] = await Promise.all([
      api(`/admin/family/${store.familyId}/screen-reports?${q}`),
      api(`/admin/family/${store.familyId}/members`),
    ])
    list.value = r.reports || []
    members.value = m.members || []
  } catch (e) { toastErr(e) } finally { loading.value = false }
}
onMounted(load)
</script>

<template>
  <div class="card">
    <div class="card-h">
      <h3>上报历史</h3><span class="sp"></span>
      <select class="input" style="width:140px" v-model.number="memberId" @change="load">
        <option :value="0">全部成员</option>
        <option v-for="m in members" :key="m.id" :value="m.id">{{ m.name }}</option>
      </select>
      <select class="input" style="width:110px;margin-left:8px" v-model.number="limit" @change="load">
        <option :value="20">最近 20</option><option :value="50">最近 50</option><option :value="200">最近 200</option>
      </select>
    </div>
    <table>
      <thead><tr><th>日期</th><th>成员</th><th>日总量</th><th>夜间</th><th>夜间判定</th><th>得分</th><th>版本</th><th>上报时间</th></tr></thead>
      <tbody>
        <tr v-for="r in list" :key="r.id">
          <td><b>{{ fmtDate(r.report_date) }}</b></td>
          <td>{{ nameOf(r.member_id) }}</td>
          <td class="num">{{ r.total_min }} 分</td>
          <td class="num">{{ r.night_min ?? '—' }}</td>
          <td>
            <span v-if="r.night_ok === null" class="tag gray">未判定</span>
            <span v-else class="tag" :class="r.night_ok ? 'ok' : 'bad'">{{ r.night_ok ? '达标' : '未达标' }}</span>
          </td>
          <td class="num" :class="r.points > 0 ? 'up' : r.points < 0 ? 'dn' : ''">{{ r.points > 0 ? '+' : '' }}{{ r.points }}</td>
          <td class="muted">rev {{ r.revision }}</td>
          <td class="muted">{{ fmtTime(r.reported_at) }}</td>
        </tr>
        <tr v-if="!loading && !list.length">
          <td colspan="8"><div class="empty"><b>还没有上报记录</b>设备每 30 分钟自动上报一次；覆盖上报会累加 revision</div></td>
        </tr>
      </tbody>
    </table>
    <div class="pad" style="border-top:1px solid var(--line)">
      <div class="hint">只读：系统内<b>没有手动录入入口</b>，数据只能由设备令牌上报（12 节）。</div>
    </div>
  </div>
</template>
