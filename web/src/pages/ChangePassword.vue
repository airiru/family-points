<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, token } from '../api'
import { loadMe, loadFamilies } from '../store'

const router = useRouter()
const oldPwd = ref('')
const newPwd = ref('')
const again = ref('')
const loading = ref(false)
const err = ref('')

async function submit() {
  err.value = ''
  // 只校验「两次输入一致」这种纯表单问题；密码强度策略由后端判定并返回错误，
  // 前端不再写一份长度规则（否则后端改了这里就骗人）。
  if (newPwd.value !== again.value) { err.value = '两次输入的新密码不一致'; return }

  loading.value = true
  try {
    await api('/admin/password', {
      method: 'POST',
      body: { old_password: oldPwd.value, new_password: newPwd.value },
    })
    // 改密会**吊销该账号全部 refresh token**（后端一步到位），
    // 所以本地存的旧 refresh 此刻已失效。若不重新登录，
    // 当前 access 还能用 2h，但到期后就再也换不出新 token —— 用户会莫名被踢。
    // 这里直接用刚设置的新密码重新登录，拿一对全新的 token。
    const d = await api('/admin/login', {
      method: 'POST',
      body: { username: token.name(), password: newPwd.value },
    })
    token.set(d.token.access_token, token.name(), d.token.refresh_token)

    await Promise.all([loadMe(), loadFamilies()])
    router.replace('/')
  } catch (e) {
    err.value = e.message
  } finally {
    loading.value = false
  }
}

async function logout() {
  try { await api('/admin/logout', { method: 'POST' }) } catch { /* 忽略 */ }
  token.clear()
  router.replace('/login')
}
</script>

<template>
  <div class="login-wrap">
    <div class="login-card">
      <div class="logo">🔑</div>
      <h1>修改初始密码</h1>
      <p class="sub">
        默认管理员首次登录必须改密。未改密前，除本页外的所有后台接口都会返回 403。
      </p>

      <div class="field" style="margin-bottom:14px">
        <label>原密码（初始密码）</label>
        <input class="input" type="password" v-model="oldPwd" placeholder="部署时 .env 里的 ADMIN_PASSWORD" />
      </div>
      <div class="field" style="margin-bottom:14px">
        <label>新密码</label>
        <input class="input" type="password" v-model="newPwd" placeholder="至少 6 位" />
      </div>
      <div class="field" style="margin-bottom:18px">
        <label>再输一次</label>
        <input class="input" type="password" v-model="again" @keyup.enter="submit" />
      </div>

      <div v-if="err" class="hint" style="color:var(--red);margin-bottom:12px">{{ err }}</div>

      <button class="btn" style="width:100%" :disabled="loading" @click="submit">
        {{ loading ? '提交中…' : '修改并进入后台' }}
      </button>

      <p class="hint" style="margin-top:16px">
        <a href="javascript:;" @click="logout">退出登录</a>
      </p>
    </div>
  </div>
</template>
