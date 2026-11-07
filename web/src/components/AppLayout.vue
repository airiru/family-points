<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, token } from '../api'
import { store, loadMe, loadFamilies, resetStore } from '../store'
import FamilyLayout from './FamilyLayout.vue'
import { toastErr } from '../toast'

const route = useRoute()
const router = useRouter()

// 2026-11-07 二次规划：以家庭为中心——家庭相关的九个页面收进「家庭工作台」
// （选择器 + 子页签常驻工作台头部），侧边栏只留全局入口。
const groups = [
  { name: '总览', items: [{ path: '/', title: '总览', icon: '◧' }] },
  { name: '家庭', items: [{ path: '/workspace/info', title: '家庭工作台', icon: '🏠' }] },
  {
    name: '全站',
    items: [
      { path: '/audit', title: '审计日志', icon: '🔍' },
      { path: '/feedback', title: '用户反馈', icon: '💬' },
    ],
  },
  {
    name: '系统',
    items: [
      { path: '/accounts', title: '管理员账号', icon: '👤' },
      { path: '/site-config', title: '站点设置', icon: '🌐' },
    ],
  },
]

const ready = ref(false)
const isWorkspace = computed(() => route.path.startsWith('/workspace'))

onMounted(async () => {
  // 冷启动时路由还没解析完，route.path 仍是 '/'，布局会先挂载一次。
  // 没有 token 就别发请求——否则每次打开 /#/login 都会白打两个 401。
  if (!token.get()) { ready.value = true; return }
  try {
    await Promise.all([loadMe(), loadFamilies()])
  } catch (e) {
    toastErr(e)
  } finally {
    ready.value = true
  }
})

async function logout() {
  // 必须通知后端：只丢本地 token 的话 refresh 仍有效 30 天，
  // 等于「退出」只是个假动作。
  try { await api('/admin/logout', { method: 'POST' }) } catch { /* 网络失败也要放行本地登出 */ }
  token.clear()
  resetStore()
  router.replace('/login')
}
</script>

<template>
  <div class="layout">
    <aside class="side">
      <div class="brand">
        <div class="logo">🏠</div>
        <div>
          <b>家庭积分</b>
          <small>管理后台</small>
        </div>
      </div>

      <div v-for="g in groups" :key="g.name" class="nav-group">
        <div class="t">{{ g.name }}</div>
        <div
          v-for="it in g.items"
          :key="it.path"
          class="nav-item"
          :class="{ on: route.path === it.path || (it.path === '/workspace/info' && route.path.startsWith('/workspace')) }"
          @click="router.push(it.path)"
        >
          <span style="width:16px;text-align:center">{{ it.icon }}</span>{{ it.title }}
        </div>
      </div>

      <div class="foot">
        管理员 {{ store.me ? (store.me.nickname || store.me.username) : '—' }}<br />
        <span style="color:#8A6A45" v-if="store.me && store.me.must_change_password">
          ⚠ 尚未修改初始密码 ·
          <a href="javascript:;" style="color:var(--primary)" @click="router.push('/change-password')">去修改</a>
        </span>
        <span v-else>仅管理员使用 · 成员端请用 App</span>
      </div>
    </aside>

    <div class="main">
      <div class="topbar">
        <h1>{{ route.meta.title }}</h1>
        <span class="sp"></span>

        <button class="btn ghost sm" @click="router.push('/change-password')">修改密码</button>
        <button class="btn ghost sm" @click="logout">退出登录</button>
      </div>

      <div class="content">
        <FamilyLayout v-if="isWorkspace">
          <router-view v-if="ready" :key="route.path + '-' + store.familyId" />
        </FamilyLayout>
        <router-view v-else-if="ready" :key="route.path + '-' + store.familyId" />
      </div>
    </div>
  </div>
</template>
