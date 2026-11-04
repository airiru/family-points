<script setup>
import { computed, onMounted, ref } from 'vue'
import { api, fmtTime } from '../api'
import { store, loadFamilies, currentFamily } from '../store'
import { toast, toastErr } from '../toast'

const info = ref(null)
const members = ref([])
const loading = ref(true)
const name = ref('')

const fam = computed(() => currentFamily())

async function load() {
  if (!store.familyId) { loading.value = false; return }
  loading.value = true
  try {
    const [d, m] = await Promise.all([
      api(`/admin/family/${store.familyId}`),
      api(`/admin/family/${store.familyId}/members`),
    ])
    info.value = d.family || d
    name.value = info.value.name || ''
    members.value = m.members || []
  } catch (e) { toastErr(e) } finally { loading.value = false }
}
onMounted(load)

async function rename() {
  const n = name.value.trim()
  if (!n) { toast('组名不能为空'); return }
  try {
    await api(`/admin/family/${store.familyId}/rename`, { method: 'POST', body: { name: n } })
    toast('已改名')
    await load()
    await loadFamilies()
  } catch (e) { toastErr(e) }
}

const leader = computed(() => members.value.find(m => m.role === 'leader'))
</script>

<template>
  <div class="grid g2" style="align-items:start">
    <div class="card">
      <div class="card-h"><h3>家庭组信息</h3></div>
      <div class="pad" style="display:flex;flex-direction:column;gap:14px">
        <div class="field">
          <label>组名</label>
          <div class="row">
            <input class="input" v-model="name" />
            <button class="btn" @click="rename">保存</button>
          </div>
        </div>
        <div class="kv" style="display:flex;justify-content:space-between;font-size:13px;padding:6px 0">
          <span class="muted">家庭组 ID</span><b>#{{ info?.id ?? store.familyId }}</b>
        </div>
        <div style="display:flex;justify-content:space-between;font-size:13px">
          <span class="muted">邀请码</span>
          <span><code>{{ info?.invite_code || fam?.invite_code || '—' }}</code></span>
        </div>
        <div style="display:flex;justify-content:space-between;font-size:13px">
          <span class="muted">成员数</span><b>{{ members.length }}</b>
        </div>
        <div style="display:flex;justify-content:space-between;font-size:13px">
          <span class="muted">当前组长</span>
          <b>{{ leader ? leader.name : '—' }}</b>
        </div>
        <div style="display:flex;justify-content:space-between;font-size:13px">
          <span class="muted">创建时间</span>
          <span class="muted">{{ info?.created_at ? fmtTime(info.created_at) : '—' }}</span>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="card-h"><h3>成员概览</h3><span class="sp"></span>
        <span class="hint">详细操作在「成员管理」</span>
      </div>
      <table>
        <thead><tr><th>成员</th><th>角色</th><th>总积分</th></tr></thead>
        <tbody>
          <tr v-for="m in members" :key="m.id">
            <td><div class="row"><div class="avatar">{{ m.avatar || m.name[0] }}</div>{{ m.name }}</div></td>
            <td><span class="tag" :class="m.role === 'leader' ? 'lead' : 'gray'">{{ m.role === 'leader' ? '组长' : '成员' }}</span></td>
            <td class="num">{{ m.score + m.frozen_score }}</td>
          </tr>
          <tr v-if="!loading && !members.length"><td colspan="3"><div class="empty">这个组还没有成员</div></td></tr>
        </tbody>
      </table>
      <div class="pad" style="border-top:1px solid var(--line)">
        <div class="hint">
          家庭组<b>不能删除</b>（会连带所有历史账目）。需要停用请把成员全部退出，组会自然空置。<br />
          「移交管理员」不在本期范围：管理员是运营方账号，与家庭组无归属关系。
        </div>
      </div>
    </div>
  </div>
</template>
