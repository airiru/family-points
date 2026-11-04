<script setup>
import { onMounted, ref } from 'vue'
import { api, fmtTime } from '../api'
import { store } from '../store'
import { toastErr } from '../toast'

const list = ref([])
const members = ref([])
const loading = ref(true)
const limit = ref(50)

const nameOf = id => (members.value.find(m => m.id === id) || {}).name || ('#' + id)

async function load() {
  if (!store.familyId) { loading.value = false; return }
  loading.value = true
  try {
    const [t, m] = await Promise.all([
      api(`/admin/family/${store.familyId}/transfers?limit=${limit.value}`),
      api(`/admin/family/${store.familyId}/members`),
    ])
    list.value = t.transfers || []
    members.value = m.members || []
  } catch (e) { toastErr(e) } finally { loading.value = false }
}
onMounted(load)
</script>

<template>
  <div class="card">
    <div class="card-h"><h3>赠送记录</h3><span class="sp"></span>
      <select class="input" style="width:110px" v-model.number="limit" @change="load">
        <option :value="20">最近 20</option><option :value="50">最近 50</option><option :value="200">最近 200</option>
      </select>
    </div>
    <table>
      <thead><tr><th>时间</th><th>转出</th><th>转入</th><th>金额</th><th>附言</th><th>状态</th></tr></thead>
      <tbody>
        <tr v-for="t in list" :key="t.id">
          <td class="muted">{{ fmtTime(t.created_at) }}</td>
          <td>{{ nameOf(t.from_member_id) }}</td>
          <td>{{ nameOf(t.to_member_id) }}</td>
          <td class="num">{{ t.amount }}</td>
          <td class="muted">{{ t.note || '—' }}</td>
          <td><span class="tag" :class="t.revoked_at ? 'bad' : 'ok'">{{ t.revoked_at ? '已撤销' : '有效' }}</span></td>
        </tr>
        <tr v-if="!loading && !list.length">
          <td colspan="6"><div class="empty"><b>还没有赠送记录</b>赠送是转移，不改变家庭总积分</div></td>
        </tr>
      </tbody>
    </table>
    <div class="pad" style="border-top:1px solid var(--line)">
      <div class="hint">撤销赠送已按 10.2.6 移除（只保留单向流转），<code>revoked_at</code> 字段为预留，不会有新数据写入。</div>
    </div>
  </div>
</template>
