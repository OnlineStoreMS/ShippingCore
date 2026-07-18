<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { Box } from '@element-plus/icons-vue'

const route = useRoute()
const title = computed(() => (route.meta.title as string) || '功能页')
const tip = computed(() => (route.meta.placeholderTip as string) || '功能建设中，后续按普源商品板块对齐补齐。')
</script>

<template>
  <el-card class="placeholder-page">
    <div class="body">
      <el-icon :size="48" color="#909399"><Box /></el-icon>
      <h2>{{ title }}</h2>
      <p>{{ tip }}</p>
    </div>
  </el-card>
</template>

<style scoped>
.body {
  min-height: 320px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: #606266;
  text-align: center;
}
h2 {
  margin: 0;
  font-size: 20px;
  color: #303133;
}
p {
  margin: 0;
  max-width: 420px;
  line-height: 1.6;
  color: #909399;
}
</style>
