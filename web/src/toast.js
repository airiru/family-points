import { reactive } from 'vue'

export const toasts = reactive([])

let seq = 0
export function toast(msg, type = 'ok') {
  const id = ++seq
  toasts.push({ id, msg, type })
  setTimeout(() => {
    const i = toasts.findIndex(t => t.id === id)
    if (i >= 0) toasts.splice(i, 1)
  }, 2600)
}
export function toastErr(e) {
  toast(e && e.message ? e.message : String(e), 'err')
}
