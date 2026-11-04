<script setup>
import { onMounted, ref } from 'vue'
import { api, fmtTime } from '../api'
import { store } from '../store'
import { toastErr } from '../toast'

const items = ref([])
const total = ref(0)
const loading = ref(true)
const expanded = ref(0)

const f = ref({ action: '', from: '', to: '', all: false })
const limit = 50
const offset = ref(0)

async function load() {
  loading.value = true
  try {
    const q = new URLSearchParams({ limit, offset: offset.value })
    if (f.value.action) q.set('action', f.value.action)
    if (f.value.from) q.set('from', f.value.from)
    if (f.value.to) q.set('to', f.value.to)
    if (!f.value.all && store.familyId) q.set('family_id', store.familyId)
    const d = await api('/admin/audit/list?' + q)
    items.value = d.items || []
    total.value = d.total || 0
  } catch (e) { toastErr(e) } finally { loading.value = false }
}
onMounted(load)

function search() { offset.value = 0; load() }
function next() { if (offset.value + limit < total.value) { offset.value += limit; load() } }
function prev() { if (offset.value > 0) { offset.value = Math.max(0, offset.value - limit); load() } }

function pretty(s) {
  if (!s) return '—'
  try { return JSON.stringify(JSON.parse(s), null, 2) } catch { return s }
}
</script>

<template>
  <div class="card" style="margin-bottom:16px">
    <div class="pad">
      <div class="filters">
        <div class="field"><label>动作</label>
          <input class="input" style="width:180px" v-model="f.action" placeholder="如 item.redeem" @keyup.enter="search" />
        </div>
        <div class="field"><label>从</label><input class="input" type="date" style="width:150px" v-model="f.from" @change="search" /></div>
        <div class="field"><label>到</label><input class="input" type="date" style="width:150px" v-model="f.to" @change="search" /></div>
        <label class="row hint" style="gap:6px;margin-bottom:10px">
          <input type="checkbox" v-model="f.all" @change="search" /> 不限家庭组
        </label>
        <button class="btn" @click="search">筛选</button>
      </div>
    </div>
  </div>

  <div class="card">
    <div class="card-h"><h3>审计日志</h3><span class="sp"></span>
      <span class="hint">共 {{ total }} 条 · append-only，无删除接口</span>
    </div>
    <table>
      <thead><tr><th>时间</th><th>操作者</th><th>动作</th><th>对象</th><th>家庭组</th><th></th></tr></thead>
      <tbody>
        <template v-for="a in items" :key="a.id">
          <tr>
            <td class="muted" style="white-space:nowrap">{{ fmtTime(a.created_at) }}</td>
            <td>{{ a.operator_type }} #{{ a.operator_id ?? '—' }}</td>
            <td><code>{{ a.action }}</code></td>
            <td class="muted">{{ a.target_type || '—' }} {{ a.target_id ? '#' + a.target_id : '' }}</td>
            <td class="muted">{{ a.family_id ?? '—' }}</td>
            <td style="text-align:right">
              <button class="btn ghost sm" @click="expanded = expanded === a.id ? 0 : a.id">
                {{ expanded === a.id ? '收起' : '详情' }}
              </button>
            </td>
          </tr>
          <tr v-if="expanded === a.id">
            <td colspan="6" style="background:#FBFCFF">
              <div class="grid g2" style="gap:12px">
                <div><div class="hint" style="margin-bottom:6px">before</div><pre class="json">{{ pretty(a.before) }}</pre></div>
                <div><div class="hint" style="margin-bottom:6px">after</div><pre class="json">{{ pretty(a.after) }}</pre></div>
              </div>
            </td>
          </tr>
        </template>
        <tr v-if="!loading && !items.length"><td colspan="6"><div class="empty">没有审计记录</div></td></tr>
      </tbody>
    </table>
    <div class="pad" style="display:flex;gap:10px;align-items:center;border-top:1px solid var(--line)">
      <button class="btn ghost sm" :disabled="offset === 0" @click="prev">上一页</button>
      <button class="btn ghost sm" :disabled="offset + limit >= total" @click="next">下一页</button>
      <span class="sp"></span>
      <span class="hint">写入端已脱敏：<code>pass_hash</code> / <code>token</code> 等字段不会进库</span>
    </div>
  </div>
</template>
