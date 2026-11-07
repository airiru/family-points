<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { store } from '../store'
import { toast, toastErr } from '../toast'

const loading = ref(true)
const saving = ref(false)
const ruleId = ref(0)
const name = ref('按时睡觉')
const points = ref(2)
const tiers = ref([])
const exists = ref(false)

function parseStreaks(s) {
  if (!s) return []
  if (Array.isArray(s)) return s // 2026-11-07 起后端直接返回数组
  try {
    const a = JSON.parse(s)
    return Array.isArray(a) ? a : []
  } catch { return [] }
}

async function load() {
  if (!store.familyId) { loading.value = false; return }
  loading.value = true
  try {
    const d = await api(`/admin/family/${store.familyId}/rule`)
    const r = d.rule
    exists.value = !!r
    if (r) {
      ruleId.value = r.id
      name.value = r.name || '按时睡觉'
      points.value = r.points ?? 2
      tiers.value = parseStreaks(r.streaks)
    } else {
      tiers.value = []
    }
  } catch (e) { toastErr(e) } finally { loading.value = false }
}
onMounted(load)

function addTier() { tiers.value.push({ every: 3, bonus: 5, mode: 'repeat' }) }
function removeTier(i) { tiers.value.splice(i, 1) }

async function save() {
  saving.value = true
  try {
    await api(`/admin/family/${store.familyId}/rule/save`, {
      method: 'POST',
      body: { id: ruleId.value, name: name.value, points: Number(points.value), streaks: tiers.value },
    })
    toast('已保存，对成员立即生效')
    await load()
  } catch (e) { toastErr(e) } finally { saving.value = false }
}
</script>

<template>
  <div class="grid g2" style="align-items:start">
    <div class="card">
      <div class="card-h"><h3>睡觉规则</h3><span class="sp"></span>
        <span class="tag" :class="exists ? 'ok' : 'warn'">{{ exists ? '已配置' : '未配置' }}</span>
      </div>
      <div class="pad" style="display:flex;flex-direction:column;gap:14px">
        <div class="field"><label>规则名称</label><input class="input" v-model="name" /></div>
        <div class="field">
          <label>达标奖励（分/天）</label>
          <input class="input" type="number" v-model="points" />
          <span class="hint">每晚按时睡觉当天加多少分。这是组内唯一的积分生成入口。</span>
        </div>
        <div class="hint">
          夜间时段与「允许时长」在<b>成员各自的屏幕规则</b>里配置（组长在 App 端设置），
          后台这里只管规则本身与连续奖励档位。
        </div>
      </div>
    </div>

    <div class="card">
      <div class="card-h">
        <h3>连续奖励档位</h3><span class="sp"></span>
        <button class="btn ghost sm" @click="addTier">＋ 添加档位</button>
      </div>
      <div class="pad" style="display:flex;flex-direction:column;gap:10px">
        <div v-for="(t, i) in tiers" :key="i" class="row" style="gap:8px">
          <span class="hint" style="width:34px">满</span>
          <input class="input" type="number" style="width:76px" v-model.number="t.every" />
          <span class="hint">天</span>
          <input class="input" type="number" style="width:86px" v-model.number="t.bonus" />
          <span class="hint">分</span>
          <select class="input" style="width:120px" v-model="t.mode">
            <option value="repeat">循环发放</option>
            <option value="once">仅一次</option>
          </select>
          <button class="btn danger sm" @click="removeTier(i)">删除</button>
        </div>
        <div v-if="!tiers.length" class="empty" style="padding:24px">
          还没有档位。例如「满 3 天 +5（循环）」「满 7 天 +10（仅一次）」。
        </div>
        <div class="hint">
          <b>repeat</b> = 每满 N 天都发；<b>once</b> = 一轮连续里只在第 N 天发一次。
          两档同时命中会一起发（第 21 天既满 3 的倍数又满 7 的倍数）。
        </div>
      </div>
    </div>
  </div>

  <div class="row" style="margin-top:16px">
    <button class="btn" :disabled="saving" @click="save">{{ saving ? '保存中…' : '保存配置' }}</button>
    <button class="btn ghost" @click="load">重新读取</button>
    <span class="hint">保存后立即对成员的后续上报生效；<b>不追溯</b>历史判定。</span>
  </div>
</template>
