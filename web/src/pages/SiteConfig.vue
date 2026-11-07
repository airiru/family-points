<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { toast, toastErr } from '../toast'

const icp = ref('')
const regPhone = ref(false)
const regEmail = ref(false)

onMounted(async () => {
  try {
    const d = await api('/admin/site-config')
    icp.value = d.icp || ''
    regPhone.value = d.reg_phone === '1'
    regEmail.value = d.reg_email === '1'
  } catch (e) { toastErr(e) }
})

async function save() {
  try {
    await api('/admin/site-config', {
      method: 'POST',
      body: { icp: icp.value.trim(), reg_phone: regPhone.value, reg_email: regEmail.value },
    })
    toast('已保存')
  } catch (e) { toastErr(e) }
}
</script>

<template>
  <div class="card" style="max-width:640px">
    <div class="card-h"><h3>站点设置</h3></div>
    <div class="pad" style="display:flex;flex-direction:column;gap:10px">
      <div class="field">
        <label>ICP 备案号（显示在 App「我的」页底部；留空则不显示）</label>
        <div class="row">
          <input class="input" v-model="icp" placeholder="如：黔ICP备2025XXXXXX号" />
          <button class="btn" @click="save">保存</button>
        </div>
      </div>
      <div class="hint">保存后 App 端下次进入「我的」页或下拉刷新即可看到。</div>

      <div class="field">
        <label>注册方式</label>
        <div class="row" style="gap:18px">
          <label style="display:flex;align-items:center;gap:6px">
            <input type="checkbox" v-model="regPhone" /> 手机号注册
          </label>
          <label style="display:flex;align-items:center;gap:6px">
            <input type="checkbox" v-model="regEmail" /> 邮箱注册
          </label>
        </div>
        <div class="hint">开发阶段不校验短信/邮件验证码；生产前需接入服务商。</div>
      </div>
    </div>
  </div>
</template>
