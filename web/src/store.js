import { reactive } from 'vue'
import { api } from './api'

// 全局状态：当前选中的家庭组（大多数后台页面都需要 family_id）
const FAMILY_KEY = 'fp_admin_family'

export const store = reactive({
  me: null,          // { id, username（不可改）, nickname（可改）, must_change_password }
  families: [],
  familyId: Number(localStorage.getItem(FAMILY_KEY)) || 0,
  loading: false,
})

export function currentFamily() {
  return store.families.find(f => f.id === store.familyId) || null
}

export function selectFamily(id) {
  store.familyId = Number(id) || 0
  localStorage.setItem(FAMILY_KEY, String(store.familyId))
}

export async function loadMe() {
  store.me = await api('/admin/me')
  return store.me
}

export async function loadFamilies() {
  const d = await api('/admin/families')
  store.families = d.families || []
  // 选中的组可能已被删，自动回落到第一个
  if (store.families.length && !store.families.some(f => f.id === store.familyId)) {
    selectFamily(store.families[0].id)
  }
  return store.families
}

export async function ensureFamily() {
  if (!store.families.length) await loadFamilies()
  return store.familyId
}

export function resetStore() {
  store.me = null
  store.families = []
}
