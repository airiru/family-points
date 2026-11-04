import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import { setUnauthorizedHandler } from './api'
import './style.css'

// 401 时统一跳登录（避免每个页面各写一遍）
setUnauthorizedHandler(() => {
  if (router.currentRoute.value.path !== '/login') {
    router.replace({ path: '/login', query: { next: router.currentRoute.value.fullPath } })
  }
})

createApp(App).use(router).mount('#app')
