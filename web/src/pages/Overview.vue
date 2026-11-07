<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, fmtTime } from '../api'
import { store, loadFamilies, selectFamily } from '../store'
import { toastErr } from '../toast'

const router = useRouter()
const st = ref({ family_count: 0, member_count: 0, record_count: 0, pending_tasks: 0 })
const loading = ref(true)

onMounted(async () => {
  try {
    st.value = await api('/admin/state')
    await loadFamilies()
  } catch (e) {
    toastErr(e)
  } finally {
    loading.value = false
  }
})

function enter(f) {
  selectFamily(f.id)
  router.push('/members')
}
</script>

<template>
  <div class="grid g4" style="margin-bottom:16px">
    <div class="stat"><div class="k">家庭组</div><div class="v">{{ st.family_count }}</div><div class="s">全部租户</div></div>
    <div class="stat"><div class="k">已入组成员</div><div class="v">{{ st.member_count }}</div><div class="s">不含未入组账号</div></div>
    <div class="stat"><div class="k">流水条数</div><div class="v">{{ st.record_count }}</div><div class="s">append-only</div></div>
    <div class="stat"><div class="k">待审核提交</div><div class="v" :style="{ color: st.pending_tasks ? 'var(--primary)' : '' }">{{ st.pending_tasks }}</div><div class="s">任务投票中</div></div>
  </div>

  <div class="card">
    <div class="card-h">
      <h3>家庭组</h3>
      <span class="sp"></span>
      <span class="hint">点「进入」切换当前组，其它页面都按这个组展示</span>
    </div>
    <table>
      <thead>
        <tr><th>#</th><th>名称</th><th>成员</th><th>家庭码</th><th>创建时间</th><th></th></tr>
      </thead>
      <tbody>
        <tr v-for="f in store.families" :key="f.id">
          <td class="num">{{ f.id }}</td>
          <td><b>{{ f.name }}</b> <span v-if="f.id === store.familyId" class="tag ok">当前</span></td>
          <td>{{ f.member_count }} 人</td>
          <td><code>{{ f.invite_code }}</code></td>
          <td class="muted">{{ fmtTime(f.created_at) }}</td>
          <td style="text-align:right"><button class="btn ghost sm" @click="enter(f)">进入</button></td>
        </tr>
        <tr v-if="!loading && !store.families.length">
          <td colspan="6"><div class="empty">还没有家庭组<br />成员在 App 注册时勾选「创建家庭组」即可</div></td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
