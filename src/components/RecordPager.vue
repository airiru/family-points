<script setup>
// 记录分页条：上一页/下一页 + 页码 + 跳转页数 + 每页条数
const props = defineProps({ page: Number, totalPages: Number, pageSize: Number });
const emit = defineEmits(['update:page', 'update:pageSize']);
const SIZES = [15, 30, 50, 100];
function goto(p) {
  const n = parseInt(p, 10);
  if (isNaN(n)) return;
  emit('update:page', Math.min(Math.max(1, n), props.totalPages));
}
function onSizeChange(e) {
  emit('update:pageSize', +e.target.value);
  emit('update:page', 1);
}
</script>

<template>
  <div class="pager">
    <button class="btn ghost" :disabled="page <= 1" @click="goto(page - 1)">上一页</button>
    <span class="sub" style="white-space:nowrap">{{ page }} / {{ totalPages }}</span>
    <button class="btn ghost" :disabled="page >= totalPages" @click="goto(page + 1)">下一页</button>
    <span class="sub pager-label">跳至</span>
    <input class="jump" type="number" min="1" :max="totalPages" :value="page" @change="goto($event.target.value)" @keydown.enter="goto($event.target.value)">
    <span class="sub pager-label">页</span>
    <select :value="pageSize" @change="onSizeChange" title="每页条数">
      <option v-for="s in SIZES" :key="s" :value="s">{{ s }} 条/页</option>
    </select>
  </div>
</template>

<style scoped>
.jump { width: 64px; text-align: center; }
.pager { flex-wrap: wrap; }
@media (max-width: 480px) {
  .pager-label { display: none; }
  .jump { width: 56px; }
}
</style>
