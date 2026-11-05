<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, token } from '../api'
import { loadMe } from '../store'

const route = useRoute()
const router = useRouter()
const username = ref('admins')
const password = ref('')
const loading = ref(false)
const err = ref('')

async function submit() {
  err.value = ''
  if (!username.value || !password.value) { err.value = '请填写账号与密码'; return }
  loading.value = true
  try {
    const d = await api('/admin/login', { method: 'POST', body: { username: username.value, password: password.value } })
    token.set(d.token.access_token, username.value, d.token.refresh_token)
    // 首次登录必须先改密：此时除 /admin/password 外全部 403，
    // 所以不能先 loadMe()（那会抛「请先修改初始密码」并停在登录页）。
    if (d.token.must_change_password) {
      router.replace('/change-password')
      return
    }
    await loadMe()
    router.replace(route.query.next || '/')
  } catch (e) {
    err.value = e.message
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-wrap">
    <div class="login-card">
      <div class="logo">🏠</div>
      <h1>家庭积分 · 管理后台</h1>
      <p class="sub">仅管理员使用。成员端请用 Android App。</p>

      <div class="field" style="margin-bottom:14px">
        <label>管理员账号</label>
        <input class="input" v-model="username" placeholder="username" @keyup.enter="submit" />
      </div>
      <div class="field" style="margin-bottom:18px">
        <label>密码</label>
        <input class="input" type="password" v-model="password" placeholder="password" @keyup.enter="submit" />
      </div>

      <div v-if="err" class="hint" style="color:var(--red);margin-bottom:12px">{{ err }}</div>

      <button class="btn" style="width:100%" :disabled="loading" @click="submit">
        {{ loading ? '登录中…' : '登录' }}
      </button>

      <p class="hint" style="margin-top:16px">
        管理员由部署时初始化（<code>migrate -seed-admin</code>），<b>不提供注册入口</b>。
      </p>
    </div>
  </div>
</template>
