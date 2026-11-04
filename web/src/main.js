import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import { setUnauthorizedHandler, setMustChangePasswordHandler } from './api'
import './style.css'

// 401 时统一跳登录（避免每个页面各写一遍）
setUnauthorizedHandler(() => {
  if (router.currentRoute.value.path !== '/login') {
    router.replace({ path: '/login', query: { next: router.currentRoute.value.fullPath } })
  }
})

// 未改初始密码时被 403 拦住 → 统一送改密页，避免停在空白页
setMustChangePasswordHandler(() => {
  if (router.currentRoute.value.path !== '/change-password') {
    router.replace('/change-password')
  }
})

createApp(App).use(router).mount('#app')
