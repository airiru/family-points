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

// 设备原始上报（screen_device_reports）：每台设备每天一行，与上面的结算行分开看。
const deviceRows = ref([])
const devices = ref([])
const deviceDate = ref('')
const deviceMemberId = ref(0)
const deviceLimit = ref(50)

const nameOf = id => (members.value.find(m => m.id === id) || {}).name || ('#' + id)
const shortKey = k => !k ? '—' : (k.length > 14 ? k.slice(0, 8) + '…' + k.slice(-4) : k)

async function load() {
  if (!store.familyId) { loading.value = false; return }
  loading.value = true
  try {
    const q = `member_id=${memberId.value}&limit=${limit.value}`
    const dq = `member_id=${deviceMemberId.value}&limit=${deviceLimit.value}` +
      (deviceDate.value ? `&date=${deviceDate.value}` : '')
    const [r, m, d] = await Promise.all([
      api(`/admin/family/${store.familyId}/screen-reports?${q}`),
      api(`/admin/family/${store.familyId}/members`),
      api(`/admin/family/${store.familyId}/screen-device-reports?${dq}`),
    ])
    list.value = r.reports || []
    members.value = m.members || []
    deviceRows.value = d.reports || []
    devices.value = d.devices || []
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
      <div class="hint">只读：系统内<b>没有手动录入入口</b>，设备用成员登录态自动上报（12 节）。</div>
    </div>
  </div>

  <div class="card">
    <div class="card-h">
      <h3>设备原始上报</h3><span class="sp"></span>
      <input class="input" type="date" style="width:150px" v-model="deviceDate" @change="load" title="不选则显示全部日期">
      <button v-if="deviceDate" class="btn ghost sm" style="margin-left:8px" @click="deviceDate = ''; load()">全部日期</button>
      <select class="input" style="width:140px;margin-left:8px" v-model.number="deviceMemberId" @change="load">
        <option :value="0">全部成员</option>
        <option v-for="m in members" :key="m.id" :value="m.id">{{ m.name }}</option>
      </select>
      <select class="input" style="width:110px;margin-left:8px" v-model.number="deviceLimit" @change="load">
        <option :value="20">最近 20</option><option :value="50">最近 50</option><option :value="200">最近 200</option>
      </select>
    </div>
    <table>
      <thead><tr><th>日期</th><th>成员</th><th>设备型号</th><th>设备标识</th><th>日总量</th><th>夜间</th><th>上报时间</th><th>更新时间</th></tr></thead>
      <tbody>
        <tr v-for="r in deviceRows" :key="r.id">
          <td><b>{{ fmtDate(r.report_date) }}</b></td>
          <td>{{ nameOf(r.member_id) }}</td>
          <td>{{ r.device_model || '—' }}</td>
          <td class="muted"><code :title="r.device_key">{{ shortKey(r.device_key) }}</code></td>
          <td class="num">{{ r.total_min }} 分</td>
          <td class="num">{{ r.night_min ?? '—' }}</td>
          <td class="muted">{{ fmtTime(r.reported_at) }}</td>
          <td class="muted">{{ r.updated_at ? fmtTime(r.updated_at) : '—' }}</td>
        </tr>
        <tr v-if="!loading && !deviceRows.length">
          <td colspan="8"><div class="empty"><b>还没有设备上报</b>选中日期或清空日期筛选可查看更多</div></td>
        </tr>
      </tbody>
    </table>
    <div class="pad" style="border-top:1px solid var(--line)">
      <div class="hint">
        已登记设备 <b>{{ devices.length }}</b> 台：
        <template v-if="devices.length">
          <span v-for="d in devices" :key="d.id" class="tag blue" style="margin-right:6px" :title="d.device_key">
            {{ d.device_model || '设备' }} · {{ shortKey(d.device_key) }} · {{ nameOf(d.member_id) }} · {{ fmtTime(d.last_seen_at) }}
          </span>
        </template>
        <span v-else class="muted">无</span>
      </div>
      <div class="hint" style="margin-top:6px">
        每台设备每天一行，数值是设备自报的<b>当天累计总时长</b>；结算时把同一成员当天所有设备<b>相加</b>，
        只产生 1 条结算行（12.4）。此处只读展示原始上报，不重新汇总、不重新判定。
      </div>
    </div>
  </div>
</template>
