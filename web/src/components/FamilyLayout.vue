<script setup>
// 家庭工作台布局（2026-11-07）：所有依赖「当前家庭组」的页面共用这一层——
// 顶部常驻 家庭选择器 + 横向子页签，切页签不丢家庭上下文；页面组件本身不感知这层。
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { store, selectFamily, currentFamily } from '../store'

const route = useRoute()
const router = useRouter()

const tabs = [
  { key: 'info', title: '信息', path: '/workspace/info' },
  { key: 'members', title: '成员', path: '/workspace/members' },
  { key: 'rules', title: '睡觉规则', path: '/workspace/rules' },
  { key: 'items', title: '商城', path: '/workspace/items' },
  { key: 'tasks', title: '任务', path: '/workspace/tasks' },
  { key: 'transfers', title: '赠送', path: '/workspace/transfers' },
  { key: 'screen', title: '屏幕时间', path: '/workspace/screen' },
  { key: 'records', title: '流水', path: '/workspace/records' },
  { key: 'stats', title: '统计', path: '/workspace/stats' },
]

const fam = computed(() => currentFamily())
const active = computed(() => route.meta.tab || '')

// 没选家庭组时自动选第一个；一个组都没有则整个工作台给空态
watch(
  () => store.familyId,
  (v) => {
    if (!v && store.families.length) selectFamily(store.families[0].id)
  },
  { immediate: true },
)

// 切换家庭组：留在当前页签，页面由 AppLayout 的 :key 重新渲染
function onPick(e) {
  selectFamily(e.target.value)
}
</script>

<template>
  <div class="famws">
    <div class="famws-head">
      <span class="muted" style="font-size:12px">家庭组</span>
      <select class="input" style="width:220px" :value="store.familyId" @change="onPick">
        <option v-for="f in store.families" :key="f.id" :value="f.id">{{ f.name }}（{{ f.member_count }} 人）</option>
      </select>
      <span class="tag ok" style="margin-left:8px" v-if="fam">#{{ fam.id }}</span>
    </div>

    <div class="famws-tabs">
      <div
        v-for="t in tabs"
        :key="t.key"
        class="famws-tab"
        :class="{ on: active === t.key }"
        @click="router.push(t.path)"
      >{{ t.title }}</div>
    </div>

    <router-view v-if="store.familyId" :key="route.path + '-' + store.familyId" />
    <div v-else class="card" style="margin-top:16px"><div class="pad empty">还没有家庭组</div></div>
  </div>
</template>

<style scoped>
.famws-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}
.famws-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 2px solid var(--line);
  margin-bottom: 14px;
  overflow-x: auto;
}
.famws-tab {
  padding: 8px 14px;
  font-size: 13.5px;
  color: #6B7280;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  margin-bottom: -2px;
  white-space: nowrap;
  border-radius: 6px 6px 0 0;
}
.famws-tab:hover { color: var(--primary); background: #FFF6F0; }
.famws-tab.on {
  color: var(--primary);
  font-weight: 700;
  border-bottom-color: var(--primary);
}
</style>
