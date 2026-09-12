<script setup>
// 登录页：首次使用时引导创建管理员账号（初始化），之后为登录表单
import { ref } from 'vue';
import { login, setup, toast, needSetup } from '../store.js';

const username = ref('');
const password = ref('');
const password2 = ref('');
const busy = ref(false);

async function submit() {
  if (!username.value.trim() || !password.value) return toast('请输入用户名和密码');
  if (needSetup.value && password.value !== password2.value) return toast('两次输入的密码不一致');
  busy.value = true;
  try {
    if (needSetup.value) await setup(username.value.trim(), password.value);
    else await login(username.value.trim(), password.value);
  } catch (e) {
    toast(e.message || '操作失败');
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="card login">
    <template v-if="needSetup">
      <div class="desc" style="text-align:center">首次使用，请创建管理员账号</div>
      <div class="form" style="flex-direction:column;align-items:stretch">
        <input v-model="username" placeholder="管理员用户名" @keydown.enter="submit">
        <input v-model="password" type="password" placeholder="密码（至少 4 位）" @keydown.enter="submit">
        <input v-model="password2" type="password" placeholder="再输入一次密码" @keydown.enter="submit">
        <button class="btn" :disabled="busy" @click="submit">{{ busy ? '创建中…' : '创建并进入' }}</button>
      </div>
    </template>
    <template v-else>
      <div class="desc" style="text-align:center">请登录后使用</div>
      <div class="form" style="flex-direction:column;align-items:stretch">
        <input v-model="username" placeholder="用户名" @keydown.enter="submit">
        <input v-model="password" type="password" placeholder="密码" @keydown.enter="submit">
        <button class="btn" :disabled="busy" @click="submit">{{ busy ? '登录中…' : '登录' }}</button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.login { max-width: 320px; margin: 24px auto 0; }
.login input { width: 100%; }
</style>
