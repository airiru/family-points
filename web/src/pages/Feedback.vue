<script setup>
import { onMounted, ref } from 'vue'
import { api, fmtTime } from '../api'
import { store } from '../store'
import { toast, toastErr } from '../toast'

const items = ref([])
const handled = ref('0')
const loading = ref(true)

async function load() {
  loading.value = true
  try {
    const d = await api(`/admin/feedbacks?handled=${handled.value}&limit=200`)
    items.value = d.items || []
  } catch (e) { toastErr(e) } finally { loading.value = false }
}
onMounted(load)

async function handle(id) {
  if (!window.confirm('标记为已处理？')) return
  try {
    await api(`/admin/feedback/${id}/handle`, { method: 'POST', body: {} })
    toast('已标记')
    await load()
  } catch (e) { toastErr(e) }
}
</script>

<template>
  <div class="card">
    <div class="card-h">
      <h3>用户反馈</h3>
      <span class="sp"></span>
      <select class="input" style="max-width:140px" v-model="handled" @change="load">
        <option value="0">未处理</option>
        <option value="1">已处理</option>
        <option value="2">全部</option>
      </select>
    </div>
    <table>
      <thead><tr><th>#</th><th>用户</th><th>内容</th><th>状态</th><th>时间</th><th></th></tr></thead>
      <tbody>
        <tr v-for="f in items" :key="f.id">
          <td class="num">{{ f.id }}</td>
          <td><code>{{ f.username }}</code></td>
          <td style="max-width:480px;white-space:pre-wrap">{{ f.content }}</td>
          <td><span class="tag" :class="f.handled ? 'ok' : 'warn'">{{ f.handled ? '已处理' : '未处理' }}</span></td>
          <td class="muted">{{ fmtTime(f.created_at) }}</td>
          <td style="text-align:right">
            <button v-if="!f.handled" class="btn ghost sm" @click="handle(f.id)">标记已处理</button>
          </td>
        </tr>
        <tr v-if="!loading && !items.length">
          <td colspan="6"><div class="empty">暂无反馈</div></td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
