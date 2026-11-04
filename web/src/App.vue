<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AppLayout from './components/AppLayout.vue'
import { toasts } from './toast'

const route = useRoute()
// 登录页与改密页都不显示侧边栏（改密页在 must_change_password 期间进不了其他页）
const isBare = computed(() => route.path === '/login' || route.meta.bare === true)
</script>

<template>
  <AppLayout v-if="!isBare" />
  <router-view v-else />

  <div v-for="t in toasts" :key="t.id" class="toast" :class="{ err: t.type === 'err' }">{{ t.msg }}</div>
</template>
