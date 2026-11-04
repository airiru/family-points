import { createRouter, createWebHashHistory } from 'vue-router'
import { token } from './api'

const routes = [
  { path: '/login', component: () => import('./pages/Login.vue'), meta: { public: true } },
  { path: '/', component: () => import('./pages/Overview.vue'), meta: { title: '总览', group: '总览' } },
  { path: '/members', component: () => import('./pages/Members.vue'), meta: { title: '成员管理', group: '家庭' } },
  { path: '/rules', component: () => import('./pages/Rules.vue'), meta: { title: '睡觉规则', group: '家庭' } },
  { path: '/items', component: () => import('./pages/Items.vue'), meta: { title: '兑换商城', group: '家庭' } },
  { path: '/tasks', component: () => import('./pages/Tasks.vue'), meta: { title: '任务管理', group: '家庭' } },
  { path: '/transfers', component: () => import('./pages/Transfers.vue'), meta: { title: '赠送管理', group: '家庭' } },
  { path: '/screen', component: () => import('./pages/Screen.vue'), meta: { title: '屏幕时间', group: '家庭' } },
  { path: '/settings', component: () => import('./pages/FamilySettings.vue'), meta: { title: '家庭组设置', group: '家庭' } },
  { path: '/records', component: () => import('./pages/Records.vue'), meta: { title: '记录', group: '数据' } },
  { path: '/stats', component: () => import('./pages/Stats.vue'), meta: { title: '统计', group: '数据' } },
  { path: '/audit', component: () => import('./pages/Audit.vue'), meta: { title: '审计日志', group: '数据' } },
  { path: '/devices', component: () => import('./pages/Devices.vue'), meta: { title: '设备管理', group: '系统' } },
  { path: '/accounts', component: () => import('./pages/Accounts.vue'), meta: { title: '管理员账号', group: '系统' } },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

export const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

router.beforeEach((to) => {
  if (to.meta.public) return true
  if (!token.get()) return { path: '/login', query: { next: to.fullPath } }
  return true
})
