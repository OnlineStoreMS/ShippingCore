<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { shippingApi, type ExpressTemplate } from '../api/shipping'

const loading = ref(false)
const syncing = ref(false)
const items = ref<ExpressTemplate[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)

function sourceLabel(row: ExpressTemplate): string {
  return row.kdzsAccountName || row.kdzsAccountCode || (row.source === 'kdzs' ? '快递助手' : row.source || '-')
}

async function load() {
  loading.value = true
  try {
    const data = await shippingApi.listExpressTemplates({ page: page.value, pageSize: pageSize.value })
    items.value = data.list || []
    total.value = data.total || 0
  } catch (e) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function sync() {
  syncing.value = true
  try {
    const stats = await shippingApi.syncKdzsPrintAssets()
    const delTpl = stats.templatesDeleted || 0
    const delAuth = stats.authsDeleted || 0
    const extra = delTpl || delAuth ? `，清理模板 ${delTpl} / 授权 ${delAuth}` : ''
    ElMessage.success(`同步完成：授权 ${stats.auths} 条，模板 ${stats.templates} 条${extra}（已与快递助手对齐）`)
    page.value = 1
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '同步失败')
  } finally {
    syncing.value = false
  }
}

function openKdzsBatchPrint() {
  window.open('https://df.kdzs.com/?#/batchPrint', '_blank')
}

function onPageChange(p: number) {
  page.value = p
  load()
}

onMounted(load)
</script>

<template>
  <div class="page">
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <div class="title-block">
            <span>快递模板</span>
            <span class="hint">与快递助手模板保持一致；同步会增改并删除助手侧已不存在的本地记录</span>
          </div>
          <div class="actions">
            <el-button @click="openKdzsBatchPrint">快递助手打单发货</el-button>
            <el-button type="primary" :loading="syncing" @click="sync">从快递助手同步（对齐）</el-button>
          </div>
        </div>
      </template>

      <el-table :data="items" v-loading="loading" stripe border empty-text="暂无数据，请点击同步或打开快递助手打单发货查看">
        <el-table-column label="来源" width="140">
          <template #default="{ row }">
            <el-tag size="small" type="success" effect="plain">{{ sourceLabel(row) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="platform" label="平台" width="100" />
        <el-table-column prop="templateName" label="模板名称" min-width="180" />
        <el-table-column prop="templateId" label="模板 ID" min-width="140" />
        <el-table-column prop="carrierName" label="承运商" width="120" />
        <el-table-column prop="carrierCode" label="承运商编码" width="110" />
        <el-table-column prop="shopName" label="店铺" min-width="140" />
        <el-table-column prop="syncedAt" label="同步时间" width="170" />
      </el-table>

      <div class="pager">
        <el-pagination
          v-model:current-page="page"
          :page-size="pageSize"
          :total="total"
          layout="total, prev, pager, next"
          @current-change="onPageChange"
        />
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 12px; }
.hdr { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.title-block { display: flex; flex-direction: column; gap: 4px; }
.hint { color: #909399; font-size: 12px; font-weight: 400; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; }
.pager { margin-top: 12px; display: flex; justify-content: flex-end; }
</style>
