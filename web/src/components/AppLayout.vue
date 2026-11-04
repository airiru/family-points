<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { token } from '../api'
import { store, loadMe, loadFamilies, selectFamily, resetStore, currentFamily } from '../store'
import { toastErr } from '../toast'

const route = useRoute()
const router = useRouter()

const groups = [
  { name: '总览', items: [{ path: '/', title: '总览', icon: '◧' }] },
  {
    name: '家庭',
    items: [
      { path: '/members', title: '成员管理', icon: '👥' },
      { path: '/rules', title: '睡觉规则', icon: '🌙' },
      { path: '/items', title: '兑换商城', icon: '🛍' },
      { path: '/tasks', title: '任务管理', icon: '🎯' },
      { path: '/transfers', title: '赠送管理', icon: '🎁' },
      { path: '/screen', title: '屏幕时间', icon: '📱' },
      { path: '/settings', title: '家庭组设置', icon: '⚙' },
    ],
  },
  {
    name: '数据',
    items: [
      { path: '/records', title: '记录', icon: '📄' },
      { path: '/stats', title: '统计', icon: '📊' },
      { path: '/audit', title: '审计日志', icon: '🔍' },
    ],
  },
  {
    name: '系统',
    items: [
      { path: '/devices', title: '设备管理', icon: '🔑' },
      { path: '/accounts', title: '管理员账号', icon: '👤' },
    ],
  },
]

const ready = ref(false)
const fam = computed(() => currentFamily())

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

function logout() {
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
          :class="{ on: route.path === it.path }"
          @click="router.push(it.path)"
        >
          <span style="width:16px;text-align:center">{{ it.icon }}</span>{{ it.title }}
        </div>
      </div>

      <div class="foot">
        管理员 {{ store.me ? store.me.username : '—' }}<br />
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

        <div class="row" v-if="store.families.length">
          <span class="muted" style="font-size:12px">当前家庭组</span>
          <select class="input" style="width:190px" :value="store.familyId" @change="selectFamily($event.target.value)">
            <option v-for="f in store.families" :key="f.id" :value="f.id">{{ f.name }}（{{ f.member_count }} 人）</option>
          </select>
        </div>

        <button class="btn ghost sm" @click="logout">退出登录</button>
      </div>

      <div class="content">
        <router-view v-if="ready" :key="route.path + '-' + store.familyId" />
      </div>
    </div>
  </div>
</template>
