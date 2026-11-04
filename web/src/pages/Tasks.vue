<script setup>
import { computed, onMounted, ref } from 'vue'
import { api, fmtTime } from '../api'
import { store } from '../store'
import { toastErr } from '../toast'

const tasks = ref([])
const members = ref([])
const loading = ref(true)

const nameOf = id => (members.value.find(m => m.id === id) || {}).name || ('#' + id)
const stats = computed(() => ({
  open: tasks.value.filter(t => t.status === 'open').length,
  taken: tasks.value.filter(t => t.status === 'taken').length,
  done: tasks.value.filter(t => t.status === 'done').length,
}))

async function load() {
  if (!store.familyId) { loading.value = false; return }
  loading.value = true
  try {
    const [t, m] = await Promise.all([
      api(`/admin/family/${store.familyId}/tasks`),
      api(`/admin/family/${store.familyId}/members`),
    ])
    tasks.value = t.tasks || []
    members.value = m.members || []
  } catch (e) { toastErr(e) } finally { loading.value = false }
}
onMounted(load)

const statusTag = s => ({ open: ['gray', '待接取'], taken: ['blue', '进行中'], done: ['ok', '已完成'], closed: ['gray', '已关闭'] }[s] || ['gray', s])
</script>

<template>
  <div class="grid g3" style="margin-bottom:16px">
    <div class="stat"><div class="k">待接取</div><div class="v">{{ stats.open }}</div></div>
    <div class="stat"><div class="k">进行中</div><div class="v">{{ stats.taken }}</div></div>
    <div class="stat"><div class="k">已完成</div><div class="v">{{ stats.done }}</div></div>
  </div>

  <div class="card">
    <div class="card-h"><h3>全部任务</h3><span class="sp"></span>
      <span class="hint">只读视图：任务的发布与接取在 App 端，后台不代发</span>
    </div>
    <table>
      <thead><tr><th>任务</th><th>悬赏</th><th>发布者</th><th>接取者</th><th>状态</th><th>截止</th><th>创建</th></tr></thead>
      <tbody>
        <tr v-for="t in tasks" :key="t.id">
          <td><b>{{ t.title }}</b><div class="hint">#{{ t.id }}</div></td>
          <td class="num">{{ t.points }}</td>
          <td>{{ nameOf(t.publisher_id) }}</td>
          <td class="muted">{{ t.assignee_id ? nameOf(t.assignee_id) : '—' }}</td>
          <td><span class="tag" :class="statusTag(t.status)[0]">{{ statusTag(t.status)[1] }}</span></td>
          <td class="muted">{{ t.due_at ? fmtTime(t.due_at) : '—' }}</td>
          <td class="muted">{{ fmtTime(t.created_at) }}</td>
        </tr>
        <tr v-if="!loading && !tasks.length">
          <td colspan="7"><div class="empty"><b>这个组还没有任务</b>成员在 App 里发布后会出现在这里</div></td>
        </tr>
      </tbody>
    </table>
    <div class="pad" style="border-top:1px solid var(--line)">
      <div class="hint">
        任务审核走<b>组内全员投票</b>（发布者除外，过半通过），后台目前<b>没有代审核接口</b>——
        强行代审会绕开「一人一票」的设计，暂不提供。
      </div>
    </div>
  </div>
</template>
