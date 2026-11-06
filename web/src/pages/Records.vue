<script setup>
import { onMounted, ref } from 'vue'
import { api, downloadCSV, fmtTime } from '../api'
import { store } from '../store'
import { toast, toastErr } from '../toast'

const list = ref([])
const total = ref(0)
const members = ref([])
const loading = ref(true)

const f = ref({ member_id: 0, source: '', keyword: '', from: '', to: '' })
const limit = 50
const offset = ref(0)

const sources = [
  ['', '全部类型'], ['screen', '屏幕时间'], ['night', '睡觉'], ['task', '任务'], ['transfer', '赠送'], ['redeem', '兑换'],
]
const nameOf = id => (members.value.find(m => m.id === id) || {}).nickname || ('#' + id)

async function load() {
  if (!store.familyId) { loading.value = false; return }
  loading.value = true
  try {
    const q = new URLSearchParams({
      member_id: f.value.member_id, source: f.value.source, keyword: f.value.keyword,
      from: f.value.from, to: f.value.to, limit, offset: offset.value,
    })
    const [d, m] = await Promise.all([
      api(`/admin/family/${store.familyId}/records?${q}`),
      api(`/admin/family/${store.familyId}/members`),
    ])
    list.value = d.records || []
    total.value = d.total || 0
    members.value = m.members || []
  } catch (e) { toastErr(e) } finally { loading.value = false }
}
onMounted(load)

function search() { offset.value = 0; load() }
function prev() { if (offset.value > 0) { offset.value = Math.max(0, offset.value - limit); load() } }
function next() { if (offset.value + limit < total.value) { offset.value += limit; load() } }
function reset() { f.value = { member_id: 0, source: '', keyword: '', from: '', to: '' }; offset.value = 0; load() }

async function exportCSV() {
  try {
    const q = new URLSearchParams({ family_id: store.familyId })
    if (f.value.from) q.set('from', f.value.from)
    if (f.value.to) q.set('to', f.value.to)
    await downloadCSV(`/admin/export/records?${q}`, `records-family-${store.familyId}.csv`)
    toast('已导出 CSV（带 UTF-8 BOM，Excel 不乱码）')
  } catch (e) { toastErr(e) }
}
</script>

<template>
  <div class="card" style="margin-bottom:16px">
    <div class="pad">
      <div class="filters">
        <div class="field">
          <label>成员</label>
          <select class="input" style="width:150px" v-model.number="f.member_id" @change="search">
            <option :value="0">全部成员</option>
            <option v-for="m in members" :key="m.id" :value="m.id">{{ m.nickname }}</option>
          </select>
        </div>
        <div class="field">
          <label>类型</label>
          <select class="input" style="width:130px" v-model="f.source" @change="search">
            <option v-for="s in sources" :key="s[0]" :value="s[0]">{{ s[1] }}</option>
          </select>
        </div>
        <div class="field">
          <label>关键字</label>
          <input class="input" style="width:180px" v-model="f.keyword" placeholder="标题模糊搜索" @keyup.enter="search" />
        </div>
        <div class="field"><label>从</label><input class="input" type="date" style="width:150px" v-model="f.from" @change="search" /></div>
        <div class="field"><label>到</label><input class="input" type="date" style="width:150px" v-model="f.to" @change="search" /></div>
        <button class="btn" @click="search">筛选</button>
        <button class="btn ghost" @click="reset">重置</button>
        <span class="sp"></span>
        <button class="btn ghost" @click="exportCSV">导出 CSV</button>
      </div>
    </div>
  </div>

  <div class="card">
    <div class="card-h">
      <h3>流水</h3><span class="sp"></span>
      <span class="hint">共 {{ total }} 条 · 第 {{ total ? offset + 1 : 0 }}–{{ Math.min(offset + limit, total) }} 条</span>
    </div>
    <table>
      <thead><tr><th>发生时间</th><th>成员</th><th>事项</th><th>来源</th><th>积分</th></tr></thead>
      <tbody>
        <tr v-for="r in list" :key="r.id">
          <td class="muted" style="white-space:nowrap">{{ fmtTime(r.occurred_at) }}</td>
          <td>{{ r.member_name }}</td>
          <td>{{ r.title }}</td>
          <td><span class="tag gray">{{ sources.find(s => s[0] === r.source)?.[1] || r.source }}</span></td>
          <td class="num" :class="r.points > 0 ? 'up' : r.points < 0 ? 'dn' : ''">{{ r.points > 0 ? '+' : '' }}{{ r.points }}</td>
        </tr>
        <tr v-if="!loading && !list.length"><td colspan="5"><div class="empty">没有符合条件的记录</div></td></tr>
      </tbody>
    </table>
    <div class="pad" style="display:flex;gap:10px;align-items:center;border-top:1px solid var(--line)">
      <button class="btn ghost sm" :disabled="offset === 0" @click="prev">上一页</button>
      <button class="btn ghost sm" :disabled="offset + limit >= total" @click="next">下一页</button>
      <span class="sp"></span>
      <span class="hint">只读：记录是 append-only，后台<b>没有删除接口</b></span>
    </div>
  </div>
</template>
