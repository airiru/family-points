<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AppLayout from './components/AppLayout.vue'
import { toasts } from './toast'

const route = useRoute()
const isLogin = computed(() => route.path === '/login')
</script>

<template>
  <AppLayout v-if="!isLogin" />
  <router-view v-else />

  <div v-for="t in toasts" :key="t.id" class="toast" :class="{ err: t.type === 'err' }">{{ t.msg }}</div>
</template>
