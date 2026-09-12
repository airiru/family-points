<script setup>
// 根组件：hash 路由（#/ 主页，#/admin 后台设置）+ 登录页 + 全局弹窗/提示
import { ref, onMounted, onUnmounted, computed } from 'vue';
import { state, loaded, needLogin, user, isAdmin, toastMsg, modal, closeModal, resetScore, loadState, logout } from './store.js';
import HomePage from './pages/HomePage.vue';
import AdminPage from './pages/AdminPage.vue';
import LoginPage from './pages/LoginPage.vue';

const page = ref(location.hash === '#/admin' ? 'admin' : 'home');
const onHash = () => (page.value = location.hash === '#/admin' ? 'admin' : 'home');
onMounted(() => window.addEventListener('hashchange', onHash));
onUnmounted(() => window.removeEventListener('hashchange', onHash));
function go(p) { location.hash = p === 'admin' ? '#/admin' : '#/'; }
// 查看者访问后台路由时切回主页（服务端同时会拒绝所有修改请求）
const effectivePage = computed(() => (page.value === 'admin' && !isAdmin.value) ? 'home' : page.value);

const modalHtml = computed(() => modal.value?.html ?? '');
window.__resetScore = id => { const m = state.members.find(x => x.id === id); if (m) resetScore(m); };

loadState();
</script>

<template>
  <div class="wrap">
    <h1>🏆 家庭积分榜</h1>

    <template v-if="needLogin">
      <LoginPage />
    </template>
    <template v-else>
      <div class="tabs top-nav">
        <button :class="{ on: effectivePage === 'home' }" @click="go('home')">积分榜</button>
        <button v-if="isAdmin" :class="{ on: effectivePage === 'admin' }" @click="go('admin')">后台设置</button>
        <button v-if="user" class="logout" @click="logout">退出</button>
      </div>

      <HomePage v-if="effectivePage === 'home'" />
      <AdminPage v-else />
    </template>
  </div>

  <div class="modal-bg" :class="{ show: modal }" @click.self="closeModal">
    <div class="modal" v-if="modal">
      <h3>{{ modal.title }}</h3>
      <div v-html="modalHtml"></div>
    </div>
  </div>
  <div class="toast" :class="{ show: toastMsg }">{{ toastMsg }}</div>
</template>

<style scoped>
.top-nav { max-width: 420px; margin: 0 auto 16px; }
.logout { flex: 0 0 auto; padding: 9px 14px; }
</style>
