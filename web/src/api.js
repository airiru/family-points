// 统一 API 客户端：token 存 localStorage，401 先尝试刷新、失败才回登录页
const TOKEN_KEY = 'fp_admin_token'
const NAME_KEY = 'fp_admin_name'
const REFRESH_KEY = 'fp_admin_refresh'

export const token = {
  get: () => localStorage.getItem(TOKEN_KEY) || '',
  // name 传 null 表示「只更新 token，不动用户名」
  set: (t, name, refresh) => {
    localStorage.setItem(TOKEN_KEY, t)
    if (name) localStorage.setItem(NAME_KEY, name)
    if (refresh) localStorage.setItem(REFRESH_KEY, refresh)
  },
  clear: () => {
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(NAME_KEY)
    localStorage.removeItem(REFRESH_KEY)
  },
  name: () => localStorage.getItem(NAME_KEY) || '管理员',
  refresh: () => localStorage.getItem(REFRESH_KEY) || '',
}

let onUnauthorized = null
export function setUnauthorizedHandler(fn) { onUnauthorized = fn }

// ---- access 过期自动刷新 ----
// access 只有 2h。没有这段逻辑时，后台每 2 小时必被踢回登录页一次，
// 而 refresh 明明还能用 30 天。
// refreshing 用来做**并发去重**：同时发起 5 个请求一起 401，
// 只能换一次 token，否则后 4 个拿旧 refresh 去换会被服务端判为已轮换而失败。
let refreshing = null

async function doRefresh() {
  const rt = token.refresh()
  if (!rt) return false
  if (refreshing) return refreshing
  refreshing = (async () => {
    try {
      const res = await fetch('/api/admin/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: rt }),
      })
      const j = await res.json().catch(() => null)
      if (!res.ok || !j || !j.ok || !j.data) return false
      token.set(j.data.access_token, null, j.data.refresh_token)
      return true
    } catch {
      return false
    } finally {
      refreshing = null
    }
  })()
  return refreshing
}

// 「尚未改初始密码」是 403，不是 401。单独挂一个处理器，
// 否则每个页面都要各自处理，实际表现就是「满屏 403、无处可去」。
let onMustChangePassword = null
export function setMustChangePasswordHandler(fn) { onMustChangePassword = fn }

export async function api(path, { method = 'GET', body, raw = false } = {}) {
  const send = () => {
    const headers = {}
    if (body !== undefined) headers['Content-Type'] = 'application/json'
    const t = token.get()
    if (t) headers['Authorization'] = 'Bearer ' + t
    return fetch('/api' + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  }

  let res = await send()

  // 401 先尝试刷新并重放一次；刷新失败才真正登出。
  // 刷新接口本身 401 不能再递归刷新。
  if (res.status === 401 && !path.startsWith('/admin/refresh')) {
    if (await doRefresh()) {
      res = await send()
    }
  }

  if (res.status === 401) {
    token.clear()
    if (onUnauthorized) onUnauthorized()
    throw new Error('登录已过期，请重新登录')
  }

  if (raw) {
    if (!res.ok) throw new Error('HTTP ' + res.status)
    return res
  }

  const data = await res.json().catch(() => null)
  if (!data) throw new Error('响应不是合法 JSON（HTTP ' + res.status + '）')
  if (!data.ok) {
    if (res.status === 403 && onMustChangePassword && String(data.error || '').includes('初始密码')) {
      onMustChangePassword()
    }
    throw new Error(data.error || '请求失败（HTTP ' + res.status + '）')
  }
  return data.data
}

// 导出 CSV：需要带 Authorization，所以不能直接 window.open
export async function downloadCSV(path, filename) {
  const res = await api(path, { raw: true })
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 2000)
}

// 时间格式化
export function fmtTime(s) {
  if (!s) return '—'
  const d = new Date(s)
  if (isNaN(d)) return s
  const p = n => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}
export function fmtDate(s) {
  if (!s) return '—'
  const d = new Date(s)
  if (isNaN(d)) return s
  const p = n => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}
