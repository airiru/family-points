<script setup>
import { onMounted, ref } from 'vue'
import { api, fmtTime } from '../api'
import { store } from '../store'
import { toast, toastErr } from '../toast'

const list = ref([])
const loading = ref(true)
const creating = ref(false)
const form = ref({ username: '', password: '' })
const myNickname = ref('')
const savingNick = ref(false)

async function load() {
  loading.value = true
  try {
    const d = await api('/admin/accounts')
    list.value = d.admins || []
  } catch (e) { toastErr(e) } finally { loading.value = false }
}

async function loadMine() {
  try {
    const me = await api('/admin/me')
    myNickname.value = me.nickname || ''
    store.me = me
  } catch (e) { toastErr(e) }
}

// 只改 nickname。username 若被塞进请求体会被后端明确 400，这里根本不提供入口。
async function saveNickname() {
  savingNick.value = true
  try {
    const d = await api('/admin/me/profile', {
      method: 'POST', body: { nickname: myNickname.value.trim() },
    })
    myNickname.value = d.nickname
    store.me = { ...(store.me || {}), nickname: d.nickname }
    toast('昵称已保存')
  } catch (e) { toastErr(e) } finally { savingNick.value = false }
}

onMounted(() => { load(); loadMine() })

async function create() {
  const u = form.value.username.trim()
  // 用户名/密码的长度策略**只在后端定义**（service/auth.go），这里不复制一份，
  // 否则后端一改前端就漂（曾经三处各写一套：4 位 / 6 位 / 8 位）。
  try {
    await api('/admin/accounts', { method: 'POST', body: { username: u, password: form.value.password } })
    toast('已创建，该账号首次登录必须改密')
    creating.value = false
    form.value = { username: '', password: '' }
    await load()
  } catch (e) { toastErr(e) }
}

async function toggle(a) {
  try {
    await api('/admin/accounts/disable', { method: 'POST', body: { admin_id: a.id, disabled: !a.disabled } })
    toast(a.disabled ? '已启用' : '已停用')
    await load()
  } catch (e) { toastErr(e) }
}
</script>

<template>
  <div class="card">
    <div class="card-h"><h3>我的昵称</h3></div>
    <div class="pad">
      <div class="hint" style="margin-bottom:10px">
        用户名 <b>{{ store.me ? store.me.username : '—' }}</b> 是登录凭证，注册后 <b>不可修改</b>；
        昵称只用于显示，可以随时修改（1–32 字）。
      </div>
      <div class="row" style="gap:10px">
        <input class="input" v-model="myNickname" maxlength="32" placeholder="昵称"
               style="max-width:280px" @keyup.enter="saveNickname" />
        <button class="btn" :disabled="savingNick" @click="saveNickname">保存昵称</button>
      </div>
    </div>
  </div>

  <div class="card">
    <div class="card-h">
      <h3>管理员账号</h3><span class="sp"></span>
      <span class="hint">管理员不属于任何家庭组，只能走 <code>/api/admin/*</code></span>
      <button class="btn sm" style="margin-left:10px" @click="creating = true">＋ 新增管理员</button>
    </div>
    <table>
      <thead><tr><th>账号</th><th>状态</th><th>密码</th><th>创建时间</th><th></th></tr></thead>
      <tbody>
        <tr v-for="a in list" :key="a.id">
          <td>
            <b>{{ a.username }}</b>
            <span v-if="store.me && a.id === store.me.id" class="tag blue" style="margin-left:6px">我</span>
            <div class="hint">昵称 {{ a.nickname || '—' }} · #{{ a.id }}</div>
          </td>
          <td><span class="tag" :class="a.disabled ? 'bad' : 'ok'">{{ a.disabled ? '已停用' : '正常' }}</span></td>
          <td><span v-if="a.must_change_password" class="tag warn">待改初始密码</span><span v-else class="hint">已改</span></td>
          <td class="muted">{{ fmtTime(a.created_at) }}</td>
          <td style="text-align:right">
            <button v-if="!(store.me && a.id === store.me.id)" class="btn ghost sm" @click="toggle(a)">
              {{ a.disabled ? '启用' : '停用' }}
            </button>
            <span v-else class="hint">不能停用自己</span>
          </td>
        </tr>
      </tbody>
    </table>
    <div class="pad" style="border-top:1px solid var(--line)">
      <div class="hint">
        新增的账号一律 <code>must_change_password=1</code>：首次登录除改密接口外全部 403。<br />
        默认账号由 <code>migrate -seed-admin</code> 按环境变量创建，<b>已有管理员则跳过</b>（幂等）。
      </div>
    </div>
  </div>

  <div v-if="creating" class="mask" @click.self="creating = false">
    <div class="modal">
      <h3>新增管理员</h3>
      <p class="sub">用户名 3–20 位，密码至少 8 位。创建后对方首次登录必须改密。</p>
      <div class="field" style="margin-bottom:12px">
        <label>用户名</label><input class="input" v-model="form.username" placeholder="3–20 位" />
      </div>
      <div class="field">
        <label>初始密码</label><input class="input" type="password" v-model="form.password" placeholder="至少 8 位" />
      </div>
      <div class="acts">
        <button class="btn ghost" @click="creating = false">取消</button>
        <button class="btn" @click="create">创建</button>
      </div>
    </div>
  </div>
</template>
