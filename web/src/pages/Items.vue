<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { store } from '../store'
import { toast, toastErr } from '../toast'

const items = ref([])
const loading = ref(true)
const editing = ref(null) // { id, name, cost }

async function load() {
  if (!store.familyId) { loading.value = false; return }
  loading.value = true
  try {
    const d = await api(`/admin/family/${store.familyId}/items`)
    items.value = d.items || []
  } catch (e) { toastErr(e) } finally { loading.value = false }
}
onMounted(load)

function newItem() { editing.value = { id: 0, name: '', cost: 10 } }
function edit(i) { editing.value = { id: i.id, name: i.name, cost: i.cost } }

async function save() {
  const e = editing.value
  if (!e.name.trim()) { toast('请填商品名称'); return }
  if (!(Number(e.cost) > 0)) { toast('所需积分必须大于 0'); return }
  try {
    await api(`/admin/family/${store.familyId}/item/save`, { method: 'POST', body: { id: e.id, name: e.name.trim(), cost: Number(e.cost) } })
    toast(e.id ? '已更新' : '已新增')
    editing.value = null
    await load()
  } catch (err) { toastErr(err) }
}
</script>

<template>
  <div class="card">
    <div class="card-h">
      <h3>商品</h3><span class="sp"></span>
      <span class="hint">成员在 App「兑换商城」看到的就是这里</span>
      <button class="btn sm" style="margin-left:10px" @click="newItem">＋ 新增商品</button>
    </div>
    <table>
      <thead><tr><th>商品</th><th>所需积分</th><th>已兑换</th><th></th></tr></thead>
      <tbody>
        <tr v-for="i in items" :key="i.id">
          <td><b>{{ i.name }}</b> <span class="hint">#{{ i.id }}</span></td>
          <td class="num">{{ i.cost }}</td>
          <td class="muted">{{ i.redeemed ?? '—' }}</td>
          <td style="text-align:right"><button class="btn ghost sm" @click="edit(i)">编辑</button></td>
        </tr>
        <tr v-if="!loading && !items.length">
          <td colspan="4"><div class="empty"><b>还没有商品</b>先新增一个，成员就能兑换了</div></td>
        </tr>
      </tbody>
    </table>
    <div class="pad" style="border-top:1px solid var(--line)">
      <div class="hint">
        · 兑换会<b>扣分并自动发券</b>，券要经过核销投票才算真正领到（9.6）<br />
        · 改名或删除不影响已发出的券（券里存的是快照）<br />
        · 后台目前<b>没有删除商品的接口</b>，需要下架请在组长端删除
      </div>
    </div>
  </div>

  <div v-if="editing" class="mask" @click.self="editing = null">
    <div class="modal">
      <h3>{{ editing.id ? '编辑商品' : '新增商品' }}</h3>
      <p class="sub">名称与积分会写进兑换快照，之后改名不影响历史记录。</p>
      <div class="field" style="margin-bottom:12px">
        <label>商品名称</label><input class="input" v-model="editing.name" placeholder="例如：冰淇淋" />
      </div>
      <div class="field">
        <label>所需积分</label><input class="input" type="number" v-model="editing.cost" />
      </div>
      <div class="acts">
        <button class="btn ghost" @click="editing = null">取消</button>
        <button class="btn" @click="save">保存</button>
      </div>
    </div>
  </div>
</template>
