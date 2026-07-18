<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Document, Setting, User, Van } from '@element-plus/icons-vue'

const route = useRoute()
const router = useRouter()
const collapsed = defineModel<boolean>('collapsed', { default: false })

const activeMenu = computed(() => route.path)
const logoText = computed(() => (collapsed.value ? '发' : '发货中心'))

function navigate(path: string) {
  router.push(path)
}
</script>

<template>
  <aside class="sidebar" :class="{ collapsed }">
    <div class="logo">{{ logoText }}</div>
    <el-menu
      :default-active="activeMenu"
      :collapse="collapsed"
      background-color="#001529"
      text-color="#ffffffa6"
      active-text-color="#fff"
    >
      <el-menu-item index="/pending" @click="navigate('/pending')">
        <el-icon><Document /></el-icon><span>待发货</span>
      </el-menu-item>
      <el-menu-item index="/shipments" @click="navigate('/shipments')">
        <el-icon><Van /></el-icon><span>发货单</span>
      </el-menu-item>
      <el-menu-item index="/carrier-accounts" @click="navigate('/carrier-accounts')">
        <el-icon><Setting /></el-icon><span>物流账号</span>
      </el-menu-item>
      <el-menu-item index="/shipper-profiles" @click="navigate('/shipper-profiles')">
        <el-icon><User /></el-icon><span>寄件人</span>
      </el-menu-item>
    </el-menu>
  </aside>
</template>

<style scoped>
.sidebar {
  width: 220px;
  background: #001529;
  transition: width 0.2s;
  flex-shrink: 0;
  overflow-y: auto;
}
.sidebar.collapsed { width: 64px; }
.logo {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 600;
  font-size: 16px;
  border-bottom: 1px solid #ffffff14;
}
.sidebar :deep(.el-menu) { border-right: none; }
</style>
