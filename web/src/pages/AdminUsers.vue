<script setup>
import { onMounted, ref } from 'vue'
import { api, fmtTime } from '../api'
import { toast, toastErr } from '../toast'

const items = ref([])
const keyword = ref('')
const loading = ref(true)

async function load() {
  loading.value = true
  try {
    const kw = keyword.value.trim()
    const d = await api(`/admin/members?limit=200${kw ? `&keyword=${encodeURIComponent(kw)}` : ''}`)
    items.value = d.items || []
  } catch (e) { toastErr(e) } finally { loading.value = false }
}
onMounted(load)

async function toggle(f) {
  const action = f.disabled ? '启用' : '停用'
  if (!window.confirm(`确认${action}账号「${f.nickname || f.username}」？${f.disabled ? '' : '停用后立即失效，且无法登录。'}`)) return
  try {
    await api(`/admin/member/${f.id}/disable`, { method: 'POST', body: { disabled: !f.disabled } })
    toast(`已${action}`)
    await load()
  } catch (e) { toastErr(e) }
}

async function resetPwd(f) {
  const pw = window.prompt(`为「${f.nickname || f.username}」设置新密码（至少 8 位）：`)
  if (pw === null) return
  if (pw.length < 8) { toast('密码至少 8 位'); return }
  try {
    await api(`/admin/member/${f.id}/reset-password`, { method: 'POST', body: { password: pw } })
    toast('已重置，该账号需用新密码重新登录')
  } catch (e) { toastErr(e) }
}
</script>

<template>
  <div class="card">
    <div class="card-h">
      <h3>用户账号</h3>
      <span class="sp"></span>
      <input
        class="input" style="max-width:240px" v-model="keyword"
        placeholder="搜索用户名 / 昵称" @keyup.enter="load"
      />
      <button class="btn ghost sm" @click="load">搜索</button>
    </div>
    <table>
      <thead><tr><th>#</th><th>用户名</th><th>昵称</th><th>家庭组</th><th>角色</th><th>积分</th><th>状态</th><th>注册时间</th><th>操作</th></tr></thead>
      <tbody>
        <tr v-for="f in items" :key="f.id">
          <td class="num">{{ f.id }}</td>
          <td><code>{{ f.username }}</code></td>
          <td>{{ f.nickname }}</td>
          <td>{{ f.family_name || '—' }}</td>
          <td><span class="tag" :class="f.role === 'leader' ? 'lead' : 'gray'">{{ f.role === 'leader' ? '组长' : '成员' }}</span></td>
          <td class="num">{{ f.score + f.frozen_score }}</td>
          <td>
            <span class="tag" :class="f.disabled ? 'bad' : 'ok'">{{ f.disabled ? '已停用' : '正常' }}</span>
          </td>
          <td class="muted">{{ fmtTime(f.created_at) }}</td>
          <td style="text-align:right;white-space:nowrap">
            <button class="btn ghost sm" @click="resetPwd(f)">重置密码</button>
            <button class="btn ghost sm" :style="f.disabled ? '' : 'color:var(--red)'" @click="toggle(f)">
              {{ f.disabled ? '启用' : '停用' }}
            </button>
          </td>
        </tr>
        <tr v-if="!loading && !items.length">
          <td colspan="9"><div class="empty">没有匹配的账号</div></td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
