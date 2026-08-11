<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Printer, Refresh, Search } from '@element-plus/icons-vue'
import {
  shippingApi,
  shipmentStatusMap,
  parseShipmentRemarkImages,
  type CarrierAccount,
  type Shipment,
} from '../api/shipping'
import { printShipmentByChannel } from '../utils/sfPrintLabel'
import { formatDateTime } from '../utils/date'
import {
  getSavedPrinterIndex,
  listLocalPrinters,
  savePrinterSelection,
  type LocalPrinter,
} from '../utils/sfPrintPlugin'

const router = useRouter()

const loading = ref(false)
const list = ref<Shipment[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

const filters = reactive({
  status: '',
  keyword: '',
  mailNo: '',
  sourceRef: '',
  sourceTid: '',
  receiver: '',
  platform: '',
  goods: '',
})

const detailVisible = ref(false)
const detail = ref<Shipment | null>(null)
const actionLoading = ref<Record<number, string>>({})

const printers = ref<LocalPrinter[]>([])
const printerIndex = ref<number | null>(getSavedPrinterIndex())
const printersLoading = ref(false)
const carrierById = ref<Record<number, CarrierAccount>>({})

function printChannelOf(row: Shipment): string {
  return (carrierById.value[row.carrierAccountId]?.printChannel || 'plugin').toLowerCase()
}

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '草稿', value: 'draft' },
  { label: '已建单', value: 'created' },
  { label: '已打印', value: 'printed' },
  { label: '已取消', value: 'cancelled' },
  { label: '失败', value: 'failed' },
]

const platformOptions = [
  { label: '全部平台', value: '' },
  { label: 'manual', value: 'manual' },
  { label: '淘宝', value: 'taobao' },
  { label: '天猫', value: 'tmall' },
  { label: '拼多多', value: 'pdd' },
  { label: '抖音', value: 'douyin' },
  { label: '京东', value: 'jd' },
]

function statusTag(statusValue: string) {
  return shipmentStatusMap[statusValue] || { label: statusValue, type: 'info' as const }
}

function receiverLines(row: Shipment) {
  const nameMobile = [row.receiverName, row.receiverMobile].filter(Boolean).join(' ')
  const addr = [row.receiverProvince, row.receiverCity, row.receiverCounty, row.receiverAddress]
    .filter(Boolean)
    .join('')
  return { nameMobile, addr }
}

function receiverText(row: Shipment) {
  const { nameMobile, addr } = receiverLines(row)
  return [nameMobile, addr].filter(Boolean).join(' / ')
}

function goodsText(row: Shipment) {
  const items = row.items || []
  if (!items.length) {
    return row.cargoName || '-'
  }
  return items
    .slice(0, 4)
    .map((it) => `${it.goodsName || '商品'}×${it.quantity || 1}`)
    .join('；') + (items.length > 4 ? ` 等${items.length}件` : '')
}

function detailRemarkImages(row: Shipment | null) {
  return parseShipmentRemarkImages(row)
}

function fmtTime(v?: string | null) {
  if (!v) return '-'
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) {
    return String(v).replace('T', ' ').replace(/\.\d+Z?$/, '').slice(0, 19)
  }
  return formatDateTime(d)
}

async function load() {
  loading.value = true
  try {
    const res = await shippingApi.listShipments({
      page: page.value,
      pageSize: pageSize.value,
      status: filters.status || undefined,
      keyword: filters.keyword.trim() || undefined,
      mail_no: filters.mailNo.trim() || undefined,
      source_ref: filters.sourceRef.trim() || undefined,
      source_tid: filters.sourceTid.trim() || undefined,
      receiver: filters.receiver.trim() || undefined,
      platform: filters.platform || undefined,
      goods: filters.goods.trim() || undefined,
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

function resetFilters() {
  filters.status = ''
  filters.keyword = ''
  filters.mailNo = ''
  filters.sourceRef = ''
  filters.sourceTid = ''
  filters.receiver = ''
  filters.platform = ''
  filters.goods = ''
  search()
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
    if (idx >= 0) {
      const prev = list.value[idx]
      list.value[idx] = {
        ...updated,
        items: updated.items?.length ? updated.items : prev.items,
      }
    }
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

async function refreshPrinters() {
  printersLoading.value = true
  try {
    printers.value = await listLocalPrinters()
    if (printerIndex.value != null && !printers.value.some((p) => p.index === printerIndex.value)) {
      printerIndex.value = printers.value[0]?.index ?? null
    }
    if (printerIndex.value == null && printers.value.length === 1) {
      printerIndex.value = printers.value[0].index
    }
  } catch (e) {
    printers.value = []
    ElMessage.warning((e as Error).message || '未能读取本机打印机列表')
  } finally {
    printersLoading.value = false
  }
}

watch(printerIndex, (v) => {
  if (v == null) return
  const name = printers.value.find((p) => p.index === v)?.name
  savePrinterSelection(v, name)
})

async function ensurePrinterSelected(): Promise<number> {
  if (!printers.value.length) {
    await refreshPrinters()
  }
  if (printerIndex.value != null) return printerIndex.value
  if (printers.value.length === 1) {
    printerIndex.value = printers.value[0].index
    return printers.value[0].index
  }
  throw new Error('请先在上方选择打印机')
}

/** 统一打印：PDF=浏览器打开官方面单；插件=本机 C-Lodop */
async function printRow(row: Shipment) {
  await withAction(row.id, 'print', async () => {
    const channelName = printChannelOf(row)
    const needPrinter = channelName !== 'pdf'
    const idx = needPrinter ? await ensurePrinterSelected() : null
    const channel = await printShipmentByChannel({
      shipmentId: row.id,
      printChannel: channelName,
      printerIndex: idx,
    })
    ElMessage.success(channel === 'pdf' ? '已在浏览器打开官方 PDF 面单' : '已按插件通道发送到本机打印机')
    return shippingApi.getShipment(row.id)
  })
}

async function cancelRow(row: Shipment) {
  const tip = row.mailNo
    ? `确认取消顺丰快递单 ${row.mailNo}？取消后运单将作废，请确认包裹尚未揽收。`
    : `确认取消发货单 #${row.id}？`
  await ElMessageBox.confirm(tip, '取消快递单', { type: 'warning', confirmButtonText: '确认取消' })
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

async function loadCarriers() {
  try {
    const res = await shippingApi.listCarrierAccounts({ page: 1, pageSize: 200, enabled: true })
    const map: Record<number, CarrierAccount> = {}
    for (const c of res.list || []) {
      if (c.id != null) map[c.id] = c
    }
    carrierById.value = map
  } catch {
    carrierById.value = {}
  }
}

onMounted(() => {
  load()
  loadCarriers()
  refreshPrinters()
})
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header>
        <div class="card-hd">
          <span>发货单列表</span>
          <div class="printer-bar">
            <el-select
              v-model="printerIndex"
              placeholder="选择本机打印机"
              clearable
              filterable
              :loading="printersLoading"
              style="width: 240px"
            >
              <el-option v-for="p in printers" :key="p.index" :label="p.name" :value="p.index" />
            </el-select>
            <el-button :icon="Refresh" :loading="printersLoading" @click="refreshPrinters">刷新</el-button>
            <el-button :icon="Printer" @click="router.push('/clodop')">C-Lodop 云打印</el-button>
          </div>
        </div>
      </template>

      <div class="filters">
        <el-input
          v-model="filters.keyword"
          clearable
          placeholder="综合搜索：运单号/订单号/收件人/手机"
          :prefix-icon="Search"
          style="width: 280px"
          @keyup.enter="search"
        />
        <el-select v-model="filters.status" placeholder="状态" clearable style="width: 120px" @change="search">
          <el-option v-for="opt in statusOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
        </el-select>
        <el-select v-model="filters.platform" placeholder="平台" clearable style="width: 120px" @change="search">
          <el-option v-for="opt in platformOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
        </el-select>
        <el-input
          v-model="filters.mailNo"
          clearable
          placeholder="运单号"
          style="width: 160px"
          @keyup.enter="search"
        />
        <el-input
          v-model="filters.receiver"
          clearable
          placeholder="收件人/手机/地址"
          style="width: 160px"
          @keyup.enter="search"
        />
        <el-input
          v-model="filters.goods"
          clearable
          placeholder="商品名称"
          style="width: 140px"
          @keyup.enter="search"
        />
        <el-input
          v-model="filters.sourceRef"
          clearable
          placeholder="系统订单号"
          style="width: 160px"
          @keyup.enter="search"
        />
        <el-input
          v-model="filters.sourceTid"
          clearable
          placeholder="平台订单号"
          style="width: 160px"
          @keyup.enter="search"
        />
        <el-button type="primary" @click="search">查询</el-button>
        <el-button @click="resetFilters">重置</el-button>
      </div>

      <el-table :data="list" border stripe>
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column label="订单" min-width="160">
          <template #default="{ row }">
            <div class="cell-stack">
              <div class="primary">{{ row.sourceRef || '-' }}</div>
              <div class="secondary">{{ row.sourceTid || '-' }}</div>
              <div class="secondary">{{ row.platform || '-' }}</div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="mailNo" label="运单号" min-width="150" show-overflow-tooltip />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status).type" size="small">{{ statusTag(row.status).label }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="收件信息" min-width="220">
          <template #default="{ row }">
            <div class="cell-stack">
              <div class="primary">{{ receiverLines(row).nameMobile || '-' }}</div>
              <div class="secondary addr">{{ receiverLines(row).addr || '-' }}</div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="商品明细" min-width="200">
          <template #default="{ row }">
            <div class="cell-stack">
              <div class="primary goods">{{ goodsText(row) }}</div>
              <div v-if="row.cargoName" class="secondary">托寄物：{{ row.cargoName }}</div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="寄件人" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            {{ [row.shipperName, row.shipperMobile].filter(Boolean).join(' ') || '-' }}
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column label="打印时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.printedAt) }}</template>
        </el-table-column>
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
              {{ row.mailNo ? '取消快递单' : '作废' }}
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
          <el-descriptions-item label="托寄物">{{ detail.cargoName || '-' }}</el-descriptions-item>
          <el-descriptions-item label="运单备注">{{ detail.remark || '-' }}</el-descriptions-item>
          <el-descriptions-item label="月结">{{ detail.useMonthly ? '是' : '否' }}</el-descriptions-item>
          <el-descriptions-item label="面单 PDF">
            <el-link v-if="detail.labelPdfUrl" :href="detail.labelPdfUrl" target="_blank" type="primary">查看存档 PDF</el-link>
            <span v-else class="muted">打单后自动存档</span>
          </el-descriptions-item>
          <el-descriptions-item v-if="detail.errorMessage" label="错误">
            <span class="error-text">{{ detail.errorMessage }}</span>
          </el-descriptions-item>
        </el-descriptions>

        <div v-if="detailRemarkImages(detail).length" class="items-block">
          <div class="block-title">存档图片</div>
          <div class="remark-imgs">
            <el-image
              v-for="url in detailRemarkImages(detail)"
              :key="url"
              :src="url"
              fit="cover"
              class="remark-img"
              :preview-src-list="detailRemarkImages(detail)"
              preview-teleported
            />
          </div>
        </div>

        <div v-if="detail.items?.length" class="items-block">
          <div class="block-title">商品明细</div>
          <el-table :data="detail.items" border size="small">
            <el-table-column prop="goodsName" label="商品" min-width="160" />
            <el-table-column prop="quantity" label="数量" width="80" />
            <el-table-column prop="outerId" label="商家编码" min-width="120" />
          </el-table>
        </div>

        <div v-if="canCancel(detail)" class="detail-actions">
          <el-button
            type="danger"
            plain
            :loading="actionLoading[detail.id] === 'cancel'"
            @click="cancelRow(detail)"
          >
            {{ detail.mailNo ? '取消快递单' : '作废发货单' }}
          </el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<style scoped>
.card-hd {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.printer-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.filters {
  display: flex;
  gap: 8px;
  margin-bottom: 16px;
  flex-wrap: wrap;
  align-items: center;
}
.cell-stack {
  line-height: 1.4;
  padding: 2px 0;
}
.cell-stack .primary {
  font-size: 13px;
  color: #303133;
}
.cell-stack .secondary {
  margin-top: 2px;
  font-size: 12px;
  color: #909399;
}
.cell-stack .addr,
.cell-stack .goods {
  white-space: normal;
  word-break: break-all;
}
.pager { margin-top: 16px; display: flex; justify-content: flex-end; }
.items-block { margin-top: 20px; }
.block-title { font-weight: 600; margin-bottom: 8px; }
.error-text { color: #f56c6c; }
.muted { color: #909399; }
.detail-actions { margin-top: 20px; }
.remark-imgs {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.remark-img {
  width: 88px;
  height: 88px;
  border-radius: 6px;
  border: 1px solid #e4e7ec;
  cursor: pointer;
}
</style>
