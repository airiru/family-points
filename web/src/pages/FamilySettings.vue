<script setup>
import { computed, onMounted, ref } from 'vue'
import { api, fmtTime } from '../api'
import { store, loadFamilies, currentFamily } from '../store'
import { toast, toastErr } from '../toast'

const info = ref(null)
const members = ref([])
const bans = ref([])
const attempts = ref([])
const loading = ref(true)
const name = ref('')

const fam = computed(() => currentFamily())

async function load() {
  if (!store.familyId) { loading.value = false; return }
  loading.value = true
  try {
    const [d, m, b, a] = await Promise.all([
      api(`/admin/family/${store.familyId}`),
      api(`/admin/family/${store.familyId}/members`),
      api(`/admin/family/${store.familyId}/bans`),
      api(`/admin/family/${store.familyId}/invitations`),
    ])
    info.value = d.family || d
    name.value = info.value.name || ''
    members.value = m.members || []
    bans.value = b.bans || []
    attempts.value = a.attempts || []
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

const statusTag = { pending: 'blue', accepted: 'ok', rejected: 'bad', cancelled: 'gray', expired: 'gray' }
const statusText = { pending: '待处理', accepted: '已接受', rejected: '已拒绝', cancelled: '已取消', expired: '已过期' }
const dirText = { invite: '邀请', request: '凭码申请' }
const banReason = { kicked: '被踢出', left: '自行退出' }
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
          <span class="muted">家庭码</span>
          <span>
            <code>{{ info?.invite_code || fam?.invite_code || '—' }}</code>
            <span v-if="info?.code_hidden" class="tag warn" style="margin-left:6px">已隐藏</span>
            <span v-else class="tag gray" style="margin-left:6px">可被搜索</span>
          </span>
        </div>
        <div class="hint" style="margin-top:-8px">
          家庭码永不过期、不可重置；隐藏开关由组长在 App 的「家庭管理」里切换。
        </div>
        <div style="display:flex;justify-content:space-between;font-size:13px">
          <span class="muted">成员数</span><b>{{ members.length }}</b>
        </div>
        <div style="display:flex;justify-content:space-between;font-size:13px">
          <span class="muted">当前组长</span>
          <b>{{ leader ? leader.nickname : '—' }}</b>
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
            <td><div class="row"><div class="avatar">{{ m.avatar || m.nickname[0] }}</div>{{ m.nickname }}</div></td>
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

    <div class="card">
      <div class="card-h"><h3>封禁名单</h3><span class="sp"></span>
        <span class="hint">只读。解除只有一条路：组长在 App 里按用户名重新邀请，对方接受时自动清除</span>
      </div>
      <table>
        <thead><tr><th>成员</th><th>用户名</th><th>原因</th><th>时间</th></tr></thead>
        <tbody>
          <tr v-for="b in bans" :key="b.member_id">
            <td><b>{{ b.nickname }}</b></td>
            <td><code>{{ b.username }}</code></td>
            <td><span class="tag bad">{{ banReason[b.reason] || b.reason }}</span></td>
            <td class="muted">{{ fmtTime(b.created_at) }}</td>
          </tr>
          <tr v-if="!loading && !bans.length"><td colspan="4"><div class="empty">没有被封禁的成员</div></td></tr>
        </tbody>
      </table>
    </div>

    <div class="card">
      <div class="card-h"><h3>邀请与加入申请</h3><span class="sp"></span>
        <span class="hint">只读。全部邀请（按用户名）与凭家庭码的申请，含已处理 / 已过期</span>
      </div>
      <table>
        <thead><tr><th>类型</th><th>对方</th><th>状态</th><th>发起人</th><th>发起时间</th><th>处理时间</th></tr></thead>
        <tbody>
          <tr v-for="a in attempts" :key="a.id">
            <td><span class="tag" :class="a.direction === 'invite' ? 'blue' : 'warn'">{{ dirText[a.direction] || a.direction }}</span></td>
            <td><b>{{ a.invitee_nickname }}</b> <code class="muted">{{ a.invitee_username }}</code></td>
            <td><span class="tag" :class="statusTag[a.status] || 'gray'">{{ statusText[a.status] || a.status }}</span></td>
            <td>{{ a.inviter_nickname }}</td>
            <td class="muted">{{ fmtTime(a.created_at) }}</td>
            <td class="muted">{{ a.decided_at ? fmtTime(a.decided_at) : '—' }}</td>
          </tr>
          <tr v-if="!loading && !attempts.length"><td colspan="6"><div class="empty">还没有邀请或申请记录</div></td></tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
