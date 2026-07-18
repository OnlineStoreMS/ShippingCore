<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import { shippingApi, shipmentStatusMap, type Shipment } from '../api/shipping'

const loading = ref(false)
const list = ref<Shipment[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const status = ref('')
const sourceRef = ref('')

const detailVisible = ref(false)
const detail = ref<Shipment | null>(null)
const actionLoading = ref<Record<number, string>>({})

const statusOptions = [
  { label: '全部', value: '' },
  { label: '草稿', value: 'draft' },
  { label: '已建单', value: 'created' },
  { label: '已打印', value: 'printed' },
  { label: '已取消', value: 'cancelled' },
  { label: '失败', value: 'failed' },
]

function statusTag(statusValue: string) {
  return shipmentStatusMap[statusValue] || { label: statusValue, type: 'info' as const }
}

function receiverText(row: Shipment) {
  const parts = [row.receiverName, row.receiverMobile].filter(Boolean)
  const addr = [row.receiverProvince, row.receiverCity, row.receiverCounty, row.receiverAddress].filter(Boolean).join('')
  if (addr) parts.push(addr)
  return parts.join(' / ')
}

async function load() {
  loading.value = true
  try {
    const res = await shippingApi.listShipments({
      page: page.value,
      pageSize: pageSize.value,
      status: status.value || undefined,
      source_ref: sourceRef.value || undefined,
    })
    list.value = res.list
    total.value = res.total
  } catch (e) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

function search() {
  page.value = 1
  load()
}

async function openDetail(row: Shipment) {
  try {
    detail.value = await shippingApi.getShipment(row.id)
    detailVisible.value = true
  } catch (e) {
    ElMessage.error((e as Error).message || '加载详情失败')
  }
}

async function withAction(id: number, action: string, fn: () => Promise<Shipment>) {
  actionLoading.value[id] = action
  try {
    const updated = await fn()
    const idx = list.value.findIndex((s) => s.id === id)
    if (idx >= 0) list.value[idx] = updated
    if (detail.value?.id === id) detail.value = updated
    ElMessage.success('操作成功')
    return updated
  } catch (e) {
    ElMessage.error((e as Error).message || '操作失败')
    throw e
  } finally {
    delete actionLoading.value[id]
  }
}

async function printRow(row: Shipment) {
  const updated = await withAction(row.id, 'print', () => shippingApi.printShipment(row.id))
  if (updated.labelUrl) window.open(updated.labelUrl, '_blank')
}

async function cancelRow(row: Shipment) {
  await ElMessageBox.confirm(`确认取消发货单 #${row.id}？`, '提示', { type: 'warning' })
  await withAction(row.id, 'cancel', () => shippingApi.cancelShipment(row.id))
}

async function retryWaybill(row: Shipment) {
  await withAction(row.id, 'waybill', () => shippingApi.createShipmentWaybill(row.id))
}

function canPrint(row: Shipment) {
  return row.mailNo && row.status !== 'cancelled'
}

function canCancel(row: Shipment) {
  return row.status !== 'cancelled'
}

function canRetry(row: Shipment) {
  return row.status === 'draft' || row.status === 'failed'
}

onMounted(load)
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header>
        <span>发货单列表</span>
      </template>

      <div class="toolbar">
        <el-select v-model="status" placeholder="状态" clearable style="width: 140px" @change="search">
          <el-option v-for="opt in statusOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
        </el-select>
        <el-input
          v-model="sourceRef"
          clearable
          placeholder="系统订单号"
          :prefix-icon="Search"
          style="width: 220px"
          @change="search"
        />
        <el-button type="primary" @click="search">查询</el-button>
      </div>

      <el-table :data="list" border stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="sourceRef" label="系统订单号" min-width="160" show-overflow-tooltip />
        <el-table-column prop="sourceTid" label="平台订单号" min-width="160" show-overflow-tooltip />
        <el-table-column prop="platform" label="平台" width="80" />
        <el-table-column prop="mailNo" label="运单号" min-width="160" show-overflow-tooltip />
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status).type" size="small">{{ statusTag(row.status).label }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="receiverName" label="收件人" width="100" />
        <el-table-column prop="createdAt" label="创建时间" width="170" />
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openDetail(row)">详情</el-button>
            <el-button
              v-if="canRetry(row)"
              link
              type="warning"
              size="small"
              :loading="actionLoading[row.id] === 'waybill'"
              @click="retryWaybill(row)"
            >
              建单
            </el-button>
            <el-button
              v-if="canPrint(row)"
              link
              type="primary"
              size="small"
              :loading="actionLoading[row.id] === 'print'"
              @click="printRow(row)"
            >
              打印
            </el-button>
            <el-button
              v-if="canCancel(row)"
              link
              type="danger"
              size="small"
              :loading="actionLoading[row.id] === 'cancel'"
              @click="cancelRow(row)"
            >
              取消
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pager">
        <el-pagination
          v-model:current-page="page"
          :page-size="pageSize"
          :total="total"
          layout="total, prev, pager, next"
          @current-change="load"
        />
      </div>
    </el-card>

    <el-drawer v-model="detailVisible" title="发货单详情" size="480px">
      <template v-if="detail">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="状态">
            <el-tag :type="statusTag(detail.status).type" size="small">{{ statusTag(detail.status).label }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="运单号">{{ detail.mailNo || '-' }}</el-descriptions-item>
          <el-descriptions-item label="系统订单号">{{ detail.sourceRef || '-' }}</el-descriptions-item>
          <el-descriptions-item label="平台订单号">{{ detail.sourceTid || '-' }}</el-descriptions-item>
          <el-descriptions-item label="收件信息">{{ receiverText(detail) }}</el-descriptions-item>
          <el-descriptions-item label="寄件人">
            {{ detail.shipperName }} / {{ detail.shipperMobile }} / {{ detail.shipperAddress }}
          </el-descriptions-item>
          <el-descriptions-item label="月结">{{ detail.useMonthly ? '是' : '否' }}</el-descriptions-item>
          <el-descriptions-item label="面单链接">
            <el-link v-if="detail.labelUrl" :href="detail.labelUrl" target="_blank" type="primary">打开面单</el-link>
            <span v-else>-</span>
          </el-descriptions-item>
          <el-descriptions-item v-if="detail.errorMessage" label="错误">
            <span class="error-text">{{ detail.errorMessage }}</span>
          </el-descriptions-item>
        </el-descriptions>

        <div v-if="detail.items?.length" class="items-block">
          <div class="block-title">商品明细</div>
          <el-table :data="detail.items" border size="small">
            <el-table-column prop="goodsName" label="商品" min-width="160" />
            <el-table-column prop="quantity" label="数量" width="80" />
            <el-table-column prop="outerId" label="商家编码" min-width="120" />
          </el-table>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<style scoped>
.toolbar { display: flex; gap: 8px; margin-bottom: 16px; flex-wrap: wrap; }
.pager { margin-top: 16px; display: flex; justify-content: flex-end; }
.items-block { margin-top: 20px; }
.block-title { font-weight: 600; margin-bottom: 8px; }
.error-text { color: #f56c6c; }
</style>
