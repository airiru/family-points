// 统一 API 客户端：token 存 localStorage，401 自动回登录页
const TOKEN_KEY = 'fp_admin_token'
const NAME_KEY = 'fp_admin_name'

export const token = {
  get: () => localStorage.getItem(TOKEN_KEY) || '',
  set: (t, name) => { localStorage.setItem(TOKEN_KEY, t); if (name) localStorage.setItem(NAME_KEY, name) },
  clear: () => { localStorage.removeItem(TOKEN_KEY); localStorage.removeItem(NAME_KEY) },
  name: () => localStorage.getItem(NAME_KEY) || '管理员',
}

let onUnauthorized = null
export function setUnauthorizedHandler(fn) { onUnauthorized = fn }

// 「尚未改初始密码」是 403，不是 401。单独挂一个处理器，
// 否则每个页面都要各自处理，实际表现就是「满屏 403、无处可去」。
let onMustChangePassword = null
export function setMustChangePasswordHandler(fn) { onMustChangePassword = fn }

export async function api(path, { method = 'GET', body, raw = false } = {}) {
  const headers = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const t = token.get()
  if (t) headers['Authorization'] = 'Bearer ' + t

  const res = await fetch('/api' + path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })

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
