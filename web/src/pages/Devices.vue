<script setup>
import { onMounted, ref } from 'vue'
import { api, fmtTime } from '../api'
import { store } from '../store'
import { toast, toastErr } from '../toast'

const list = ref([])
const members = ref([])
const loading = ref(true)
const creating = ref(false)
const form = ref({ member_id: 0, name: 'Android 设备' })
const newToken = ref('')

const nameOf = id => (members.value.find(m => m.id === id) || {}).name || ('#' + id)

async function load() {
  if (!store.familyId) { loading.value = false; return }
  loading.value = true
  try {
    const [d, m] = await Promise.all([
      api(`/admin/family/${store.familyId}/device`),
      api(`/admin/family/${store.familyId}/members`),
    ])
    list.value = d.devices || []
    members.value = m.members || []
  } catch (e) { toastErr(e) } finally { loading.value = false }
}
onMounted(load)

function openCreate() {
  newToken.value = ''
  form.value = { member_id: members.value[0]?.id || 0, name: 'Android 设备' }
  creating.value = true
}

async function create() {
  if (!form.value.member_id) { toast('请选择成员'); return }
  try {
    const d = await api(`/admin/family/${store.familyId}/device`, { method: 'POST', body: form.value })
    newToken.value = d.token
    await load()
    toast('令牌已生成，只显示这一次')
  } catch (e) { toastErr(e) }
}

async function revoke(d) {
  try {
    await api(`/admin/family/${store.familyId}/device/revoke`, { method: 'POST', body: { device_id: d.id } })
    toast('已吊销，该设备下次上报会 401')
    await load()
  } catch (e) { toastErr(e) }
}
</script>

<template>
  <div class="card">
    <div class="card-h">
      <h3>设备令牌</h3><span class="sp"></span>
      <span class="hint">屏幕时间的<b>唯一写入凭证</b>（成员 JWT 打不进来）</span>
      <button class="btn sm" style="margin-left:10px" @click="openCreate">＋ 生成令牌</button>
    </div>
    <table>
      <thead><tr><th>设备</th><th>成员</th><th>状态</th><th>创建</th><th>最近上报</th><th></th></tr></thead>
      <tbody>
        <tr v-for="d in list" :key="d.id">
          <td><b>{{ d.name }}</b><div class="hint">#{{ d.id }}</div></td>
          <td>{{ nameOf(d.member_id) }}</td>
          <td><span class="tag" :class="d.revoked ? 'bad' : 'ok'">{{ d.revoked ? '已吊销' : '有效' }}</span></td>
          <td class="muted">{{ fmtTime(d.created_at) }}</td>
          <td class="muted">{{ d.last_seen_at ? fmtTime(d.last_seen_at) : '从未上报' }}</td>
          <td style="text-align:right">
            <button v-if="!d.revoked" class="btn danger sm" @click="revoke(d)">吊销</button>
          </td>
        </tr>
        <tr v-if="!loading && !list.length"><td colspan="6"><div class="empty"><b>还没有设备令牌</b>生成后填进 Android App 才能上报屏幕时间</div></td></tr>
      </tbody>
    </table>
    <div class="pad" style="border-top:1px solid var(--line)">
      <div class="hint">令牌明文<b>只在生成时返回一次</b>，库里只存 SHA-256。吊销后立即失效，无法恢复。</div>
    </div>
  </div>

  <div v-if="creating" class="mask" @click.self="creating = false">
    <div class="modal">
      <h3>生成设备令牌</h3>
      <p class="sub">一个令牌绑定一名成员，用于该成员设备的屏幕时间上报。</p>

      <template v-if="!newToken">
        <div class="field" style="margin-bottom:12px">
          <label>绑定成员</label>
          <select class="input" v-model.number="form.member_id">
            <option v-for="m in members" :key="m.id" :value="m.id">{{ m.name }}（@{{ m.username }}）</option>
          </select>
        </div>
        <div class="field">
          <label>设备名称</label>
          <input class="input" v-model="form.name" placeholder="例如：小星的平板" />
        </div>
        <div class="acts">
          <button class="btn ghost" @click="creating = false">取消</button>
          <button class="btn" @click="create">生成</button>
        </div>
      </template>

      <template v-else>
        <div class="hint" style="color:var(--red);font-weight:650;margin-bottom:8px">⚠ 只显示这一次，关掉就再也看不到了</div>
        <pre class="json" style="font-size:12.5px">{{ newToken }}</pre>
        <div class="acts">
          <button class="btn" @click="creating = false">我已保存</button>
        </div>
      </template>
    </div>
  </div>
</template>
