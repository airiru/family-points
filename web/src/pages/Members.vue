<script setup>
import { computed, onMounted, ref } from 'vue'
import { api, fmtTime } from '../api'
import { store, currentFamily } from '../store'
import { toast, toastErr } from '../toast'

const list = ref([])
const loading = ref(true)
const inviteName = ref('')
const confirm = ref(null) // { kind, member, title, desc }

const fam = computed(() => currentFamily())

async function load() {
  if (!store.familyId) { loading.value = false; return }
  loading.value = true
  try {
    const d = await api(`/admin/family/${store.familyId}/members`)
    list.value = d.members || []
  } catch (e) {
    toastErr(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function toggleHidden(m) {
  try {
    await api(`/admin/family/${store.familyId}/member/hidden`, { method: 'POST', body: { member_id: m.id, hidden: !m.hidden } })
    toast(m.hidden ? '已显示' : '已隐藏')
    await load()
  } catch (e) { toastErr(e) }
}

function askKick(m) {
  confirm.value = {
    kind: 'kick', member: m,
    title: `踢出「${m.nickname}」？`,
    desc: '踢出后该成员积分清零、family_id 置空，且不能再加入本组（写入 family_bans）。历史记录保留在原组。此操作不可撤销。',
  }
}
function askLeader(m) {
  confirm.value = {
    kind: 'leader', member: m,
    title: `把组长转给「${m.nickname}」？`,
    desc: '转让后你自己降为普通成员，对方成为组长。后台仍可再改回来，但组内只有一名组长。',
  }
}

async function doConfirm() {
  const c = confirm.value
  if (!c) return
  try {
    if (c.kind === 'kick') {
      await api(`/admin/family/${store.familyId}/member/kick`, { method: 'POST', body: { member_id: c.member.id } })
      toast('已踢出')
    } else {
      await api(`/admin/family/${store.familyId}/member/leader`, { method: 'POST', body: { member_id: c.member.id } })
      toast('组长已转让')
    }
    confirm.value = null
    await load()
  } catch (e) { toastErr(e) }
}

async function invite() {
  const u = inviteName.value.trim()
  if (!u) return
  try {
    await api(`/admin/family/${store.familyId}/invite`, { method: 'POST', body: { username: u } })
    toast('已把 ' + u + ' 加入本组')
    inviteName.value = ''
    await load()
  } catch (e) { toastErr(e) }
}
</script>

<template>
  <div class="card" style="margin-bottom:16px">
    <div class="card-h"><h3>按用户名邀请</h3><span class="sp"></span>
      <span class="hint">只能拉<b>已注册且未入组</b>的用户</span>
    </div>
    <div class="pad" style="display:flex;gap:10px;align-items:center">
      <input class="input" style="max-width:280px" v-model="inviteName" placeholder="输入用户名，例如 xiaoyue" @keyup.enter="invite" />
      <button class="btn" @click="invite">邀请加入</button>
      <span class="hint" style="margin-left:8px" v-if="fam">邀请码 <code>{{ fam.invite_code }}</code> · 7 天有效</span>
    </div>
  </div>

  <div class="card">
    <div class="card-h">
      <h3>成员</h3><span class="sp"></span>
      <span class="hint" v-if="fam">{{ fam.name }} · {{ list.length }} 人</span>
    </div>
    <table>
      <thead>
        <tr><th>昵称</th><th>用户名（不可改）</th><th>角色</th><th>可用</th><th>冻结</th><th>状态</th><th>加入时间</th><th></th></tr>
      </thead>
      <tbody>
        <tr v-for="m in list" :key="m.id">
          <td>
            <div class="row">
              <div class="avatar">{{ m.avatar || m.nickname[0] }}</div>
              <div><b>{{ m.nickname }}</b><div class="hint">#{{ m.id }}</div></div>
            </div>
          </td>
          <td class="muted">{{ m.username }}</td>
          <td><span class="tag" :class="m.role === 'leader' ? 'lead' : 'gray'">{{ m.role === 'leader' ? '组长' : '成员' }}</span></td>
          <td class="num">{{ m.score }}</td>
          <td class="num" :style="{ color: m.frozen_score ? 'var(--gold)' : '' }">{{ m.frozen_score }}</td>
          <td><span v-if="m.hidden" class="tag warn">已隐藏</span><span v-else class="tag ok">正常</span></td>
          <td class="muted">{{ fmtTime(m.created_at) }}</td>
          <td style="text-align:right;white-space:nowrap">
            <button class="btn ghost sm" @click="toggleHidden(m)">{{ m.hidden ? '显示' : '隐藏' }}</button>
            <button v-if="m.role !== 'leader'" class="btn ghost sm" style="margin-left:6px" @click="askLeader(m)">设组长</button>
            <button v-if="m.role !== 'leader'" class="btn danger sm" style="margin-left:6px" @click="askKick(m)">踢出</button>
          </td>
        </tr>
        <tr v-if="!loading && !list.length">
          <td colspan="8"><div class="empty"><b>这个组还没有成员</b>用上面的用户名邀请，或让成员用邀请码加入</div></td>
        </tr>
      </tbody>
    </table>
  </div>

  <div v-if="confirm" class="mask" @click.self="confirm = null">
    <div class="modal">
      <h3>{{ confirm.title }}</h3>
      <p class="sub">{{ confirm.desc }}</p>
      <div class="acts">
        <button class="btn ghost" @click="confirm = null">取消</button>
        <button class="btn" :class="{ danger: confirm.kind === 'kick' }" @click="doConfirm">
          {{ confirm.kind === 'kick' ? '确认踢出' : '确认转让' }}
        </button>
      </div>
    </div>
  </div>
</template>
