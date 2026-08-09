<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Document, Setting, User, Van, Key, Tickets, Connection, Box } from '@element-plus/icons-vue'

const route = useRoute()
const router = useRouter()
const collapsed = defineModel<boolean>('collapsed', { default: false })

const activeMenu = computed(() => route.path)
const logoText = computed(() => (collapsed.value ? '发' : '发货中心'))
const kdzsOpen = computed(() =>
  ['/kdzs-accounts', '/waybill-auths', '/express-templates'].includes(route.path) ? ['kdzs'] : [],
)

function navigate(path: string) {
  router.push(path)
}
</script>

<template>
  <aside class="sidebar" :class="{ collapsed }">
    <div class="logo">{{ logoText }}</div>
    <el-menu
      :default-active="activeMenu"
      :default-openeds="kdzsOpen"
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

      <el-sub-menu index="kdzs">
        <template #title>
          <el-icon><Box /></el-icon><span>快递助手</span>
        </template>
        <el-menu-item index="/kdzs-accounts" @click="navigate('/kdzs-accounts')">
          <el-icon><Connection /></el-icon><span>账号管理</span>
        </el-menu-item>
        <el-menu-item index="/waybill-auths" @click="navigate('/waybill-auths')">
          <el-icon><Key /></el-icon><span>面单授权</span>
        </el-menu-item>
        <el-menu-item index="/express-templates" @click="navigate('/express-templates')">
          <el-icon><Tickets /></el-icon><span>快递模板</span>
        </el-menu-item>
      </el-sub-menu>

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
.sidebar :deep(.el-sub-menu .el-menu-item) {
  min-width: 0;
  background-color: #000c17 !important;
}
</style>
