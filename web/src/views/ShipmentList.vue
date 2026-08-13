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
import {
  copyPngDataUrl,
  copyText,
  downloadDataUrl,
  renderLabelPdfToPng,
} from '../utils/labelPdfPreview'

const router = useRouter()

const loading = ref(false)
const list = ref<Shipment[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

const labelVisible = ref(false)
const labelLoading = ref(false)
const labelPng = ref('')
const labelPdfUrl = ref('')
const labelTitle = ref('面单预览')
const labelError = ref('')

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
const promiseLabel = ref('')
const promiseHint = ref('')
const promiseLoading = ref(false)
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
  { label: '抖店', value: 'FXG' },
  { label: '淘宝', value: 'TB' },
  { label: '小红书', value: 'XHS' },
  { label: '拼多多', value: 'PDD' },
  { label: '快手', value: 'KSXD' },
  { label: '京东', value: 'JD' },
  { label: '视频号', value: 'SPH' },
  { label: '1688', value: 'ALI1688' },
  { label: '淘工厂', value: 'TGC' },
  { label: '手工单', value: 'DFHAND' },
]

const sourceLabels: Record<string, string> = {
  kdzs: '电商',
  wx_mall: '小程序',
  store: '门店',
  xianyu: '闲鱼',
  manual: '手工订单',
  ordercore: '订单中心',
  storesyncagent: '同步代理',
}

const platformLabels: Record<string, string> = {
  FXG: '抖店',
  DY: '抖店',
  TB: '淘宝',
  XHS: '小红书',
  PDD: '拼多多',
  KSXD: '快手',
  KS: '快手',
  JD: '京东',
  SPH: '视频号',
  ALI1688: '1688',
  TGC: '淘工厂',
  DFHAND: '手工单',
  HAND: '手工单',
  MANUAL: '手工单',
}

function statusTag(statusValue: string) {
  return shipmentStatusMap[statusValue] || { label: statusValue, type: 'info' as const }
}

function labelSource(v?: string) {
  const key = (v || '').trim()
  return (key && sourceLabels[key]) || key || '-'
}

function labelPlatform(v?: string) {
  const key = (v || '').trim().toUpperCase()
  return (key && platformLabels[key]) || v || '-'
}

/** 与待发货列表「订单类型」一致 */
function formatOrderSource(row: Shipment) {
  const channel = (row.sourceChannel || '').trim()
  if (channel === 'manual') {
    return (row.manualSourceName || row.shopName || '').trim() || '手工订单'
  }
  const src = labelSource(channel)
  const plat = labelPlatform(row.platform)
  const shop = (row.shopName || '').trim()
  if (src !== '-' && shop) return `${src} · ${shop}`
  if (src !== '-' && plat && plat !== '-') return `${src} · ${plat}`
  if (src !== '-') return src
  if (shop) return shop
  return plat || '-'
}

function shopDisplay(row: Shipment) {
  return (row.shopName || row.manualSourceName || '').trim() || '-'
}

/** 与待发货「订单号」一致：优先 orderNo（OC…），兼容旧数据 */
function orderNoDisplay(row: Shipment) {
  const no = (row.orderNo || '').trim()
  if (no) return no
  const tid = (row.sourceTid || '').trim()
  if (tid.toUpperCase().startsWith('OC')) return tid
  const ref = (row.sourceRef || '').trim()
  if (ref.toUpperCase().startsWith('OC')) return ref
  return ref || tid || '-'
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

/** 商品行展示：优先规格名(skuCode)，与待发货一致 */
function formatGoodsLine(it: { goodsName?: string; skuCode?: string; quantity?: number }) {
  const spec = (it.skuCode || '').trim()
  const name = (it.goodsName || '').trim()
  const title = spec || name
  if (!title) return ''
  const num = it.quantity && it.quantity > 0 ? it.quantity : 1
  return `${title} x${num}`
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

/** 发货时间：优先 shippedAt，旧数据回退打印/创建时间 */
function shipTimeOf(row: Pick<Shipment, 'shippedAt' | 'printedAt' | 'createdAt'>) {
  return fmtTime(row.shippedAt || row.printedAt || row.createdAt)
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

async function loadPromiseTm(id: number, mailNo?: string) {
  promiseLabel.value = ''
  promiseHint.value = ''
  if (!mailNo?.trim()) return
  promiseLoading.value = true
  try {
    const res = await shippingApi.searchPromiseTm(id)
    promiseLabel.value = res.promiseLabel || ''
    promiseHint.value = res.hint || ''
  } catch (e) {
    promiseHint.value = (e as Error).message || '预计派送时间查询失败'
  } finally {
    promiseLoading.value = false
  }
}

async function openDetail(row: Shipment) {
  try {
    detail.value = await shippingApi.getShipment(row.id)
    detailVisible.value = true
    void loadPromiseTm(detail.value.id, detail.value.mailNo)
  } catch (e) {
    ElMessage.error((e as Error).message || '加载详情失败')
  }
}

async function openLabelPreview(row: Shipment) {
  const url = (row.labelPdfUrl || '').trim()
  if (!url) {
    ElMessage.warning('暂无面单存档，打印后会自动生成，请稍后刷新')
    return
  }
  labelVisible.value = true
  labelLoading.value = true
  labelPng.value = ''
  labelError.value = ''
  labelPdfUrl.value = url
  labelTitle.value = row.mailNo ? `面单 ${row.mailNo}` : '面单预览'
  try {
    labelPng.value = await renderLabelPdfToPng(url)
  } catch (e) {
    labelError.value = (e as Error).message || '面单渲染失败'
  } finally {
    labelLoading.value = false
  }
}

function downloadLabelPng() {
  if (!labelPng.value) return
  const name = (labelTitle.value || 'label').replace(/\s+/g, '_')
  downloadDataUrl(labelPng.value, `${name}.png`)
}

async function copyLabelLink() {
  if (!labelPdfUrl.value) return
  try {
    await copyText(labelPdfUrl.value)
    ElMessage.success('已复制面单链接')
  } catch {
    ElMessage.error('复制失败')
  }
}

async function copyLabelImage() {
  if (!labelPng.value) return
  try {
    await copyPngDataUrl(labelPng.value)
    ElMessage.success('已复制面单图片')
  } catch {
    ElMessage.warning('当前环境不支持复制图片，请改用下载图片')
  }
}

function openLabelPdf() {
  if (!labelPdfUrl.value) return
  window.open(labelPdfUrl.value, '_blank', 'noopener')
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

      <el-table :data="list" border stripe empty-text="暂无发货单">
        <el-table-column label="订单号" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">{{ orderNoDisplay(row) }}</template>
        </el-table-column>
        <el-table-column label="订单类型" width="120" show-overflow-tooltip>
          <template #default="{ row }">{{ formatOrderSource(row) }}</template>
        </el-table-column>
        <el-table-column label="平台" width="90">
          <template #default="{ row }">{{ labelPlatform(row.platform) }}</template>
        </el-table-column>
        <el-table-column label="平台单号" min-width="180">
          <template #default="{ row }">
            <div v-if="row.sourceTid">{{ row.sourceTid }}</div>
            <div
              v-if="row.sourceRef && row.sourceRef !== row.sourceTid"
              class="muted"
            >
              系统：{{ row.sourceRef }}
            </div>
            <span v-if="!row.sourceTid && !row.sourceRef">-</span>
          </template>
        </el-table-column>
        <el-table-column label="店铺" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">{{ shopDisplay(row) }}</template>
        </el-table-column>
        <el-table-column label="商品信息" min-width="260">
          <template #default="{ row }">
            <div v-if="row.items?.length" class="goods-list">
              <div v-for="(it, idx) in row.items" :key="it.id || idx" class="goods-cell">
                <div class="goods-text">{{ formatGoodsLine(it) || '-' }}</div>
              </div>
            </div>
            <span v-else>{{ row.cargoName || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="mailNo" label="运单号" min-width="150" show-overflow-tooltip />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status).type" size="small">{{ statusTag(row.status).label }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="收件信息" min-width="200">
          <template #default="{ row }">
            <div class="cell-stack">
              <div class="primary">{{ receiverLines(row).nameMobile || '-' }}</div>
              <div class="secondary addr">{{ receiverLines(row).addr || '-' }}</div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="发货时间" width="170">
          <template #default="{ row }">{{ shipTimeOf(row) }}</template>
        </el-table-column>
        <el-table-column label="打印时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.printedAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openDetail(row)">详情</el-button>
            <el-button
              v-if="row.labelPdfUrl"
              link
              type="primary"
              size="small"
              @click="openLabelPreview(row)"
            >
              面单
            </el-button>
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

    <el-dialog v-model="labelVisible" :title="labelTitle" width="520px" destroy-on-close>
      <div v-loading="labelLoading" class="label-preview">
        <el-image
          v-if="labelPng"
          :src="labelPng"
          fit="contain"
          class="label-img"
          :preview-src-list="[labelPng]"
          preview-teleported
        />
        <div v-else-if="labelError" class="label-error">
          <p>{{ labelError }}</p>
          <el-button v-if="labelPdfUrl" type="primary" link @click="openLabelPdf">改为打开 PDF</el-button>
        </div>
        <div v-else class="muted">加载中…</div>
      </div>
      <template #footer>
        <el-button :disabled="!labelPng" @click="downloadLabelPng">下载图片</el-button>
        <el-button :disabled="!labelPng" @click="copyLabelImage">复制图片</el-button>
        <el-button :disabled="!labelPdfUrl" @click="copyLabelLink">复制链接</el-button>
        <el-button :disabled="!labelPdfUrl" type="primary" @click="openLabelPdf">打开 PDF</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="detailVisible" title="发货单详情" size="480px">
      <template v-if="detail">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="状态">
            <el-tag :type="statusTag(detail.status).type" size="small">{{ statusTag(detail.status).label }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="运单号">{{ detail.mailNo || '-' }}</el-descriptions-item>
          <el-descriptions-item label="发货时间">{{ shipTimeOf(detail) }}</el-descriptions-item>
          <el-descriptions-item label="打印时间">{{ fmtTime(detail.printedAt) }}</el-descriptions-item>
          <el-descriptions-item v-if="detail.mailNo" label="预计派送">
            <span v-if="promiseLoading" class="muted">查询中…</span>
            <span v-else-if="promiseLabel">{{ promiseLabel }}</span>
            <span v-else class="muted">{{ promiseHint || '-' }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="订单号">{{ orderNoDisplay(detail) }}</el-descriptions-item>
          <el-descriptions-item label="订单类型">{{ formatOrderSource(detail) }}</el-descriptions-item>
          <el-descriptions-item label="平台">{{ labelPlatform(detail.platform) }}</el-descriptions-item>
          <el-descriptions-item label="平台单号">{{ detail.sourceTid || '-' }}</el-descriptions-item>
          <el-descriptions-item label="店铺">{{ shopDisplay(detail) }}</el-descriptions-item>
          <el-descriptions-item label="收件信息">{{ receiverText(detail) }}</el-descriptions-item>
          <el-descriptions-item label="寄件人">
            {{ detail.shipperName }} / {{ detail.shipperMobile }} / {{ detail.shipperAddress }}
          </el-descriptions-item>
          <el-descriptions-item label="托寄物">{{ detail.cargoName || '-' }}</el-descriptions-item>
          <el-descriptions-item label="运单备注">{{ detail.remark || '-' }}</el-descriptions-item>
          <el-descriptions-item label="月结">{{ detail.useMonthly ? '是' : '否' }}</el-descriptions-item>
          <el-descriptions-item label="面单">
            <template v-if="detail.labelPdfUrl">
              <el-button link type="primary" @click="openLabelPreview(detail)">查看面单图片</el-button>
              <el-link :href="detail.labelPdfUrl" target="_blank" type="primary" class="ml8">打开 PDF</el-link>
            </template>
            <span v-else class="muted">打单后自动存档，请稍后刷新</span>
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
.goods-list { display: flex; flex-direction: column; gap: 6px; }
.goods-cell { display: flex; gap: 8px; align-items: flex-start; }
.goods-text {
  font-size: 13px;
  line-height: 1.4;
  white-space: normal;
  word-break: break-all;
  color: #303133;
}
.items-block { margin-top: 20px; }
.block-title { font-weight: 600; margin-bottom: 8px; }
.error-text { color: #f56c6c; }
.muted { color: #909399; font-size: 12px; }
.ml8 { margin-left: 8px; }
.detail-actions { margin-top: 20px; }
.label-preview {
  min-height: 280px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.label-img {
  width: 100%;
  max-height: 60vh;
  background: #f5f7fa;
}
.label-error {
  text-align: center;
  color: #f56c6c;
  font-size: 13px;
}
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
