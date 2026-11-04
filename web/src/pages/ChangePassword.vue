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
  if (!oldPwd.value || !newPwd.value) { err.value = '请填写原密码与新密码'; return }
  if (newPwd.value !== again.value) { err.value = '两次输入的新密码不一致'; return }
  if (newPwd.value.length < 6) { err.value = '新密码至少 6 位'; return }

  loading.value = true
  try {
    await api('/admin/password', {
      method: 'POST',
      body: { old_password: oldPwd.value, new_password: newPwd.value },
    })
    // 改密成功后 must_change_password 已清零，重新拉一次身份即可正常进后台
    await Promise.all([loadMe(), loadFamilies()])
    router.replace('/')
  } catch (e) {
    err.value = e.message
  } finally {
    loading.value = false
  }
}

function logout() {
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
