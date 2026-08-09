<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import {
  shippingApi,
  type CarrierAccount,
  type ExpressTemplate,
  type OMSOrder,
  type OrderSnapshot,
  type ShipperProfile,
} from '../api/shipping'

type PrintMode = 'kdzs' | 'sf'

const route = useRoute()
const router = useRouter()

const loading = reactive({ orders: false, ship: false, syncWaybill: false })
const omsOrders = ref<OMSOrder[]>([])
const omsTotal = ref(0)
const carrierAccounts = ref<CarrierAccount[]>([])
const shipperProfiles = ref<ShipperProfile[]>([])
const allTemplates = ref<ExpressTemplate[]>([])

const selectedOrders = ref<OMSOrder[]>([])
const shipDialogVisible = ref(false)
const confirmKdzsVisible = ref(false)
const shipTargets = ref<OMSOrder[]>([])
const printMode = ref<PrintMode>('kdzs')
const selectedTemplateId = ref('')
const shipForm = reactive({
  carrierAccountId: undefined as number | undefined,
  shipperProfileId: undefined as number | undefined,
  useMonthly: false,
})
const shipResult = ref<{ mailNo: string; labelUrl?: string; shipmentId?: number } | null>(null)
const kdzsExpressCompany = ref('')
const kdzsExpressRows = ref<{ order: OMSOrder; expressNo: string }[]>([])

const expressCompanyOptions = [
  '圆通速递',
  '中通快递',
  '申通快递',
  '韵达快递',
  '极兔速递',
  '顺丰速运',
  '京东快递',
  '德邦快递',
  '邮政快递包裹',
  'EMS',
]

function inferExpressCompany(templateName?: string): string {
  const n = (templateName || '').trim()
  if (!n) return ''
  const codeMap: Record<string, string> = {
    YTO: '圆通速递',
    ZTO: '中通快递',
    STO: '申通快递',
    YUNDA: '韵达快递',
    YD: '韵达快递',
    JTSD: '极兔速递',
    JT: '极兔速递',
    SF: '顺丰速运',
    JD: '京东快递',
    DBL: '德邦快递',
    EMS: 'EMS',
  }
  const byCode = codeMap[n.toUpperCase()]
  if (byCode) return byCode
  const hit = expressCompanyOptions.find((c) => n.includes(c.replace(/速递|快递|速运/g, '')) || n.includes(c))
  if (hit) return hit
  if (n.includes('圆通')) return '圆通速递'
  if (n.includes('中通')) return '中通快递'
  if (n.includes('申通')) return '申通快递'
  if (n.includes('韵达')) return '韵达快递'
  if (n.includes('极兔')) return '极兔速递'
  if (n.includes('顺丰')) return '顺丰速运'
  if (n.includes('京东')) return '京东快递'
  if (n.includes('德邦')) return '德邦快递'
  if (n.includes('邮政')) return '邮政快递包裹'
  if (n.includes('EMS')) return 'EMS'
  return ''
}

const omsFilters = reactive({
  sourceChannel: '',
  platform: '',
  keyword: '',
  page: 1,
  pageSize: 20,
})

const sourceChannelOptions = [
  { label: '电商', value: 'kdzs' },
  { label: '小程序', value: 'wx_mall' },
  { label: '门店', value: 'store' },
  { label: '闲鱼', value: 'xianyu' },
  { label: '手工订单', value: 'manual' },
]

const platformOptions = [
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

function templatePlatformGroup(order: OMSOrder): string {
  const code = (order.platform || '').trim().toUpperCase()
  const channel = (order.sourceChannel || '').trim().toLowerCase()
  if (code === 'FXG' || code === 'DY') return '抖店'
  if (code === 'TB') return '菜鸟'
  if (code === 'DFHAND' || code === 'HAND' || code === 'MANUAL' || channel === 'manual') return '菜鸟'
  if (code === 'XHS') return '小红书'
  if (code === 'PDD') return '拼多多'
  if (code === 'KSXD' || code === 'KS') return '快手小店'
  if (code === 'JD') return '京东'
  if (code === 'SPH') return '视频号'
  return labelPlatform(order.platform)
}

function orderPlatformCode(order?: OMSOrder | null): string {
  const code = (order?.platform || '').trim().toUpperCase()
  if (code === 'DY') return 'FXG'
  if (code === 'HAND' || code === 'MANUAL') return 'DFHAND'
  return code || 'FXG'
}

function labelSource(v?: string) {
  const key = (v || '').trim()
  return (key && sourceLabels[key]) || key || '-'
}

function labelPlatform(v?: string) {
  const key = (v || '').trim().toUpperCase()
  return (key && platformLabels[key]) || v || '-'
}

function formatGoodsLine(g: { productName?: string; skuSpecs?: string; quantity?: number }): string {
  const name = g.productName?.trim() || ''
  const spec = g.skuSpecs?.trim() || ''
  const title = name && spec && name !== spec ? `${name} ${spec}` : name || spec || ''
  if (!title) return ''
  const num = g.quantity && g.quantity > 0 ? g.quantity : 1
  return `${title} x${num}`
}

const selectionGroup = computed(() => {
  if (!selectedOrders.value.length) return ''
  const groups = new Set(selectedOrders.value.map(templatePlatformGroup))
  return groups.size === 1 ? [...groups][0] : ''
})

const batchEnabled = computed(() => selectedOrders.value.length > 0 && !!selectionGroup.value)

const shipGroup = computed(() => {
  if (!shipTargets.value.length) return ''
  return templatePlatformGroup(shipTargets.value[0])
})

const filteredTemplates = computed(() => {
  const group = shipGroup.value
  if (!group) return []
  return allTemplates.value.filter((t) => t.enabled !== false && t.platform === group)
})

const selectedTemplate = computed(() =>
  filteredTemplates.value.find((t) => t.templateId === selectedTemplateId.value),
)

const isBatchShip = computed(() => shipTargets.value.length > 1)

async function loadOmsOrders() {
  loading.orders = true
  try {
    const data = await shippingApi.listPendingOMSOrders({
      page: omsFilters.page,
      pageSize: omsFilters.pageSize,
      shipStatus: 'wait_ship',
      allocType: 'self_ship',
      sourceChannel: omsFilters.sourceChannel || undefined,
      platform: omsFilters.platform || undefined,
      keyword: omsFilters.keyword || undefined,
    })
    omsOrders.value = data.list || []
    omsTotal.value = data.total || 0
    selectedOrders.value = []
  } catch (e) {
    ElMessage.error((e as Error).message || '加载订单中心订单失败')
  } finally {
    loading.orders = false
  }
}

async function loadOptions() {
  try {
    const [carriers, shippers, templates] = await Promise.all([
      shippingApi.listCarrierAccounts({ page: 1, pageSize: 200 }),
      shippingApi.listShipperProfiles({ page: 1, pageSize: 200 }),
      shippingApi.listExpressTemplates({ page: 1, pageSize: 200 }),
    ])
    carrierAccounts.value = (carriers.list || []).filter((c) => c.enabled)
    shipperProfiles.value = (shippers.list || []).filter((s) => s.enabled)
    allTemplates.value = templates.list || []
  } catch {
    /* optional */
  }
}

function onOmsFilterChange() {
  omsFilters.page = 1
  loadOmsOrders()
}

function onOmsPageChange(page: number) {
  omsFilters.page = page
  loadOmsOrders()
}

function onSelectionChange(rows: OMSOrder[]) {
  selectedOrders.value = rows
  if (rows.length > 1) {
    const groups = new Set(rows.map(templatePlatformGroup))
    if (groups.size > 1) {
      ElMessage.warning('批量打单发货需选择同一平台的订单')
    }
  }
}

function omsOrderToSnapshot(order: OMSOrder): OrderSnapshot {
  const addr = order.address
  return {
    platform: order.platform || '',
    shopId: order.shopId || '',
    sysTid: order.platformSysTid || '',
    sourceTid: order.platformOrderId || order.orderNo,
    receiverName: addr?.name || order.buyerName || '',
    receiverMobile: addr?.phone || order.buyerPhone || '',
    receiverProvince: addr?.province || '',
    receiverCity: addr?.city || '',
    receiverCounty: addr?.district || '',
    receiverAddress: addr?.fullText || addr?.address || '',
    goods: (order.items || []).map((g) => ({
      title: g.productName || '',
      skuName: g.skuSpecs || '',
      num: g.quantity && g.quantity > 0 ? g.quantity : 1,
      outerId: '',
      price: 0,
    })),
  }
}

function normalizePrintMode(raw?: string | null): PrintMode {
  const v = (raw || '').trim().toLowerCase()
  if (v === 'sf' || v === 'carrier') return 'sf'
  return 'kdzs'
}

function prepareShipDialog(orders: OMSOrder[], preferredMode?: PrintMode) {
  shipTargets.value = orders
  shipResult.value = null
  const mode = preferredMode || 'kdzs'
  // 批量仅支持快递助手
  printMode.value = orders.length > 1 && mode === 'sf' ? 'kdzs' : mode
  selectedTemplateId.value = ''
  kdzsExpressCompany.value = ''
  kdzsExpressRows.value = orders.map((order) => ({ order, expressNo: '' }))
  const defaultCarrier = carrierAccounts.value[0]
  const defaultShipper = shipperProfiles.value.find((s) => s.isDefault) || shipperProfiles.value[0]
  shipForm.carrierAccountId = defaultCarrier?.id
  shipForm.shipperProfileId = defaultShipper?.id
  shipForm.useMonthly = defaultCarrier?.useMonthly ?? false

  const tpls = allTemplates.value.filter(
    (t) => t.enabled !== false && t.platform === templatePlatformGroup(orders[0]),
  )
  if (tpls.length) {
    selectedTemplateId.value = tpls[0].templateId
    kdzsExpressCompany.value = inferExpressCompany(tpls[0].templateName)
  }

  shipDialogVisible.value = true
}

/** 订单中心「创建并打印」跳转：定位订单并打开打单弹窗 */
async function applyDeepLink() {
  const orderIdRaw = route.query.orderId
  const orderId = Number(Array.isArray(orderIdRaw) ? orderIdRaw[0] : orderIdRaw)
  const autoShipRaw = route.query.autoShip
  const autoShip = String(Array.isArray(autoShipRaw) ? autoShipRaw[0] : autoShipRaw || '') === '1'
  const preferredMode = normalizePrintMode(
    String(Array.isArray(route.query.printMode) ? route.query.printMode[0] : route.query.printMode || ''),
  )
  const keywordRaw = route.query.keyword
  const keyword = String(Array.isArray(keywordRaw) ? keywordRaw[0] : keywordRaw || '').trim()
  const channelRaw = route.query.sourceChannel
  const sourceChannel = String(Array.isArray(channelRaw) ? channelRaw[0] : channelRaw || '').trim()

  if (sourceChannel) omsFilters.sourceChannel = sourceChannel
  if (keyword) omsFilters.keyword = keyword
  omsFilters.page = 1

  if (!orderId && !keyword && !sourceChannel && !autoShip) return

  await loadOmsOrders()

  // 清掉一次性 deep-link 参数，避免刷新重复弹窗
  const q = { ...route.query }
  delete q.orderId
  delete q.autoShip
  delete q.printMode
  delete q.keyword
  delete q.sourceChannel
  router.replace({ path: '/pending', query: q })

  if (!autoShip || !orderId) return

  const target = omsOrders.value.find((o) => o.id === orderId)
  if (!target) {
    ElMessage.warning(`未找到待发货订单 #${orderId}，请在列表中手动打单`)
    return
  }
  prepareShipDialog([target], preferredMode)
}

function onTemplateChange(templateId: string) {
  const tpl = filteredTemplates.value.find((t) => t.templateId === templateId)
  const inferred = inferExpressCompany(tpl?.templateName)
  if (inferred) kdzsExpressCompany.value = inferred
}

function openShipDialogOms(order: OMSOrder) {
  prepareShipDialog([order])
}

function openBatchShipDialog() {
  if (!selectedOrders.value.length) {
    ElMessage.warning('请先勾选订单')
    return
  }
  if (!selectionGroup.value) {
    ElMessage.warning('批量打单发货需选择同一平台的订单')
    return
  }
  prepareShipDialog([...selectedOrders.value])
}

function onCarrierChange(id: number | undefined) {
  const carrier = carrierAccounts.value.find((c) => c.id === id)
  if (carrier) shipForm.useMonthly = carrier.useMonthly
}

function onPrintModeChange() {
  if (printMode.value === 'sf' && isBatchShip.value) {
    ElMessage.warning('自建物流暂不支持批量，请单笔操作或改用快递助手')
    printMode.value = 'kdzs'
  }
}

async function openKdzsBatchPrint() {
  const order = shipTargets.value[0]
  if (!order) return
  if (!selectedTemplateId.value) {
    ElMessage.warning('请选择快递模板')
    return
  }
  loading.ship = true
  try {
    const platform = orderPlatformCode(order)
    const data = await shippingApi.getBatchPrintURL(platform)
    const url = data?.url
    if (!url) throw new Error('未获取到打单地址')
    const win = window.open(url, '_blank')
    if (!win) {
      ElMessage.warning('浏览器拦截了新窗口，请允许弹窗后重试')
      return
    }
    const tip = selectedTemplate.value?.templateName
      ? `已打开快递助手，请选择模板「${selectedTemplate.value.templateName}」后打印。完成后点击「确认已打单发货」回填运单号。`
      : '已打开快递助手打单页，完成后请点击「确认已打单发货」回填运单号。'
    ElMessage.success(tip)
    confirmKdzsVisible.value = true
    // 打开确认框后尝试从快递助手同步单号（打印完成后再点「同步单号」也可）
    void syncWaybillsFromKdzs()
  } catch (e) {
    ElMessage.error((e as Error).message || '打开快递助手失败')
  } finally {
    loading.ship = false
  }
}

function mapCompanyToOption(name?: string): string {
  const inferred = inferExpressCompany(name)
  if (inferred) return inferred
  const n = (name || '').trim()
  if (!n) return ''
  const exact = expressCompanyOptions.find((c) => c === n)
  return exact || ''
}

async function syncWaybillsFromKdzs() {
  if (!shipTargets.value.length) return
  const platform = orderPlatformCode(shipTargets.value[0])
  const items = shipTargets.value.map((o) => ({
    sysTid: o.platformSysTid || undefined,
    tid: o.platformOrderId || undefined,
  }))
  if (items.every((i) => !i.sysTid && !i.tid)) {
    ElMessage.warning('订单缺少平台单号，无法从快递助手同步')
    return
  }
  loading.syncWaybill = true
  try {
    const res = await shippingApi.queryPrintWaybills({ platform, items })
    const list = res.items || []
    let filled = 0
    for (let i = 0; i < shipTargets.value.length; i++) {
      const order = shipTargets.value[i]
      const hit =
        list.find(
          (r) =>
            (order.platformSysTid && r.sysTid === order.platformSysTid) ||
            (order.platformOrderId && r.tid === order.platformOrderId),
        ) || list[i]
      if (!hit?.found || !hit.expressNo) continue
      const row = kdzsExpressRows.value.find((r) => r.order.id === order.id)
      if (row) {
        row.expressNo = hit.expressNo
        filled++
      }
      const company =
        mapCompanyToOption(hit.expressCompany) ||
        inferExpressCompany(hit.expressCompany) ||
        inferExpressCompany(hit.expressCode)
      if (company) kdzsExpressCompany.value = company
      else if (hit.expressCompany && !kdzsExpressCompany.value) {
        // 不在下拉内时仍尽量写入（可手动改）
        const matched = expressCompanyOptions.find((c) => hit.expressCompany!.includes(c.slice(0, 2)))
        if (matched) kdzsExpressCompany.value = matched
      }
    }
    if (filled > 0) {
      ElMessage.success(`已从快递助手同步 ${filled} 笔运单号`)
    } else {
      const detail = list.map((r) => r.message).find((m) => !!m)
      ElMessage.warning(detail || '暂未查到运单号，请确认已在快递助手打印完成后，再点「同步单号」')
    }
  } catch (e) {
    ElMessage.error((e as Error).message || '同步单号失败')
  } finally {
    loading.syncWaybill = false
  }
}

async function submitShip() {
  if (printMode.value === 'kdzs') {
    await openKdzsBatchPrint()
    return
  }

  if (isBatchShip.value) {
    ElMessage.warning('自建物流暂不支持批量，请单笔操作')
    return
  }

  const order = shipTargets.value[0]
  if (!order) return
  if (!shipForm.carrierAccountId || !shipForm.shipperProfileId) {
    ElMessage.warning('请选择物流账号和寄件人')
    return
  }
  loading.ship = true
  try {
    const shipment = await shippingApi.createShipmentFromOrder({
      carrierAccountId: shipForm.carrierAccountId,
      shipperProfileId: shipForm.shipperProfileId,
      useMonthly: shipForm.useMonthly,
      orderId: order.id,
      sourceSystem: 'ordercore',
      order: omsOrderToSnapshot(order),
    })
    const waybill = await shippingApi.createShipmentWaybill(shipment.id)
    shipResult.value = {
      mailNo: waybill.mailNo,
      labelUrl: waybill.labelUrl,
      shipmentId: waybill.id,
    }
    ElMessage.success(`打单成功，运单号：${waybill.mailNo}`)
    await loadOmsOrders()
  } catch (e) {
    ElMessage.error((e as Error).message || '打单失败')
  } finally {
    loading.ship = false
  }
}

async function submitKdzsConfirm() {
  const rows = kdzsExpressRows.value
  if (!rows.length) return
  if (!kdzsExpressCompany.value.trim()) {
    ElMessage.warning('请选择快递公司')
    return
  }
  const missing = rows.filter((r) => !r.expressNo.trim())
  if (missing.length) {
    ElMessage.warning(isBatchShip.value ? '请为每笔订单填写运单号' : '请输入运单号')
    return
  }
  loading.ship = true
  try {
    const company = kdzsExpressCompany.value.trim()
    for (const row of rows) {
      await shippingApi.confirmKdzsShip({
        orderId: row.order.id,
        expressNo: row.expressNo.trim(),
        expressCompany: company,
        order: omsOrderToSnapshot(row.order),
      })
    }
    ElMessage.success(isBatchShip.value ? `已确认发货 ${rows.length} 笔` : '已确认发货并回写订单中心')
    confirmKdzsVisible.value = false
    closeShipDialog()
    selectedOrders.value = []
    await loadOmsOrders()
  } catch (e) {
    ElMessage.error((e as Error).message || '确认发货失败')
  } finally {
    loading.ship = false
  }
}

async function printResult() {
  if (!shipResult.value?.shipmentId) return
  try {
    const updated = await shippingApi.printShipment(shipResult.value.shipmentId)
    if (updated.labelUrl) {
      window.open(updated.labelUrl, '_blank')
    } else {
      ElMessage.info('暂无面单链接')
    }
  } catch (e) {
    ElMessage.error((e as Error).message || '打印失败')
  }
}

function closeShipDialog() {
  shipDialogVisible.value = false
  shipTargets.value = []
  shipResult.value = null
  confirmKdzsVisible.value = false
  selectedTemplateId.value = ''
}

onMounted(async () => {
  await loadOptions()
  const hasDeepLink =
    !!route.query.orderId ||
    !!route.query.keyword ||
    !!route.query.sourceChannel ||
    String(route.query.autoShip || '') === '1'
  if (hasDeepLink) {
    await applyDeepLink()
  } else {
    await loadOmsOrders()
  }
})
</script>

<template>
  <div class="page">
    <el-card v-loading="loading.orders">
      <template #header>
        <div class="hdr">
          <div class="title-block">
            <span>待发货 <span class="count">({{ omsTotal }})</span></span>
            <span class="hint">订单中心自营待发货（含手工单）</span>
          </div>
          <el-button type="primary" :disabled="!batchEnabled" @click="openBatchShipDialog">
            批量打单发货
            <template v-if="selectedOrders.length">（{{ selectedOrders.length }}）</template>
          </el-button>
        </div>
      </template>

      <div class="filter-panel">
        <div class="filter-row">
          <span class="filter-label">筛选</span>
          <div class="filters">
            <el-select v-model="omsFilters.sourceChannel" placeholder="订单类型" clearable style="width: 130px" @change="onOmsFilterChange">
              <el-option v-for="opt in sourceChannelOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
            </el-select>
            <el-select v-model="omsFilters.platform" placeholder="平台" clearable style="width: 120px" @change="onOmsFilterChange">
              <el-option v-for="opt in platformOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
            </el-select>
            <el-input v-model="omsFilters.keyword" clearable placeholder="订单号/买家" style="width: 200px" @change="onOmsFilterChange" />
            <el-button type="primary" :icon="Search" :loading="loading.orders" @click="loadOmsOrders">查询</el-button>
          </div>
        </div>
        <div v-if="selectedOrders.length && !selectionGroup" class="warn-tip">
          已选订单平台不一致，无法批量打单发货
        </div>
      </div>

      <el-table :data="omsOrders" border stripe empty-text="暂无待发货订单" @selection-change="onSelectionChange">
        <el-table-column type="selection" width="48" />
        <el-table-column prop="orderNo" label="订单号" min-width="160" />
        <el-table-column label="订单类型" width="100">
          <template #default="{ row }">{{ labelSource(row.sourceChannel) }}</template>
        </el-table-column>
        <el-table-column label="平台" width="90">
          <template #default="{ row }">{{ labelPlatform(row.platform) }}</template>
        </el-table-column>
        <el-table-column label="平台单号" min-width="180">
          <template #default="{ row }">
            <div v-if="row.platformOrderId">{{ row.platformOrderId }}</div>
            <div v-if="row.platformSysTid" class="muted">系统：{{ row.platformSysTid }}</div>
          </template>
        </el-table-column>
        <el-table-column prop="shopName" label="店铺" min-width="120" show-overflow-tooltip />
        <el-table-column label="商品信息" min-width="260">
          <template #default="{ row }">
            <div v-if="row.items?.length" class="goods-list">
              <div v-for="(g, idx) in row.items" :key="idx" class="goods-cell">
                <img v-if="g.picUrl" :src="g.picUrl" class="goods-thumb" alt="" />
                <div class="goods-text">{{ formatGoodsLine(g) || '-' }}</div>
              </div>
            </div>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="收件信息" min-width="220">
          <template #default="{ row }">
            <div>{{ row.address?.name || row.buyerName || '-' }}</div>
            <div class="muted">{{ row.address?.phone || row.buyerPhone }}</div>
            <div class="muted">{{ row.address?.fullText || row.address?.address }}</div>
          </template>
        </el-table-column>
        <el-table-column prop="payTime" label="付款时间" width="170" />
        <el-table-column label="操作" width="110" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openShipDialogOms(row)">打单发货</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pager">
        <el-pagination
          v-model:current-page="omsFilters.page"
          :page-size="omsFilters.pageSize"
          :total="omsTotal"
          layout="total, prev, pager, next"
          @current-change="onOmsPageChange"
        />
      </div>
    </el-card>

    <el-dialog
      v-model="shipDialogVisible"
      title="打单发货"
      :width="isBatchShip ? '640px' : '560px'"
      @close="closeShipDialog"
    >
      <template v-if="shipTargets.length">
        <div class="ship-order-info">
          <span v-if="!isBatchShip">订单中心 #{{ shipTargets[0].orderNo }}</span>
          <span v-else>已选 {{ shipTargets.length }} 笔订单</span>
          <el-tag size="small" type="info" class="ml8">{{ labelPlatform(shipTargets[0].platform) }}</el-tag>
        </div>

        <el-form label-width="100px" class="ship-form">
          <el-form-item label="打单方式">
            <el-radio-group v-model="printMode" @change="onPrintModeChange">
              <el-radio value="kdzs">快递助手</el-radio>
              <el-radio value="sf" :disabled="isBatchShip">自建物流</el-radio>
            </el-radio-group>
          </el-form-item>

          <template v-if="printMode === 'kdzs'">
            <el-form-item label="快递模板" required>
              <div v-if="filteredTemplates.length" class="tpl-bar">
                <el-radio-group v-model="selectedTemplateId" class="tpl-radios" @change="onTemplateChange">
                  <el-radio
                    v-for="t in filteredTemplates"
                    :key="t.templateId"
                    :value="t.templateId"
                    border
                    class="tpl-radio"
                  >
                    {{ t.templateName }}
                  </el-radio>
                </el-radio-group>
              </div>
              <el-alert
                v-else
                type="warning"
                :closable="false"
                :title="`暂无「${shipGroup}」快递模板，请先到「快递模板」页同步`"
              />
            </el-form-item>
            <el-alert
              type="info"
              :closable="false"
              :title="selectedTemplate
                ? `将打开快递助手打单页，请在助手中选择模板「${selectedTemplate.templateName}」并完成打印，然后回填运单号发货。`
                : '请先选择快递模板'"
            />
          </template>

          <template v-else>
            <el-form-item label="物流账号" required>
              <el-select
                v-model="shipForm.carrierAccountId"
                placeholder="选择物流账号"
                style="width: 100%"
                @change="onCarrierChange"
              >
                <el-option v-for="c in carrierAccounts" :key="c.id" :label="c.name" :value="c.id!" />
              </el-select>
            </el-form-item>
            <el-form-item label="寄件人" required>
              <el-select v-model="shipForm.shipperProfileId" placeholder="选择寄件人" style="width: 100%">
                <el-option
                  v-for="s in shipperProfiles"
                  :key="s.id"
                  :label="s.isDefault ? `${s.name}（默认）` : s.name"
                  :value="s.id!"
                />
              </el-select>
            </el-form-item>
            <el-form-item label="月结">
              <el-switch v-model="shipForm.useMonthly" />
            </el-form-item>
          </template>
        </el-form>

        <el-result v-if="shipResult" icon="success" title="打单成功">
          <template #sub-title>
            运单号：<strong>{{ shipResult.mailNo }}</strong>
          </template>
          <template #extra>
            <el-button type="primary" @click="printResult">打开面单</el-button>
          </template>
        </el-result>
      </template>

      <template #footer>
        <el-button @click="closeShipDialog">{{ shipResult ? '关闭' : '取消' }}</el-button>
        <el-button
          v-if="printMode === 'kdzs' && shipTargets.length && !shipResult"
          @click="confirmKdzsVisible = true"
        >
          确认已打单发货
        </el-button>
        <el-button
          v-if="!shipResult"
          type="primary"
          :loading="loading.ship"
          :disabled="printMode === 'kdzs' && !selectedTemplateId"
          @click="submitShip"
        >
          {{ printMode === 'kdzs' ? '打开快递助手' : '确认打单' }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="confirmKdzsVisible"
      title="确认已打单发货"
      :width="isBatchShip ? '560px' : '440px'"
      append-to-body
    >
      <el-form label-width="90px">
        <el-form-item label="快递公司" required>
          <el-select
            v-model="kdzsExpressCompany"
            placeholder="请选择快递公司"
            filterable
            style="width: 100%"
          >
            <el-option v-for="c in expressCompanyOptions" :key="c" :label="c" :value="c" />
          </el-select>
        </el-form-item>
        <template v-if="!isBatchShip">
          <el-form-item label="运单号" required>
            <el-input v-model="kdzsExpressRows[0].expressNo" placeholder="快递助手打单后的运单号" />
          </el-form-item>
        </template>
        <template v-else>
          <div class="batch-confirm-hint muted">请为每笔订单填写运单号</div>
          <div v-for="row in kdzsExpressRows" :key="row.order.id" class="batch-row">
            <div class="batch-order">{{ row.order.orderNo }}</div>
            <el-input v-model="row.expressNo" placeholder="运单号" />
          </div>
        </template>
      </el-form>
      <template #footer>
        <el-button @click="confirmKdzsVisible = false">取消</el-button>
        <el-button :loading="loading.syncWaybill" @click="syncWaybillsFromKdzs">同步单号</el-button>
        <el-button type="primary" :loading="loading.ship" @click="submitKdzsConfirm">确认发货</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 12px; }
.hdr { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.title-block { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.count { color: #909399; font-size: 14px; font-weight: normal; }
.hint { color: #909399; font-size: 12px; font-weight: normal; }
.filter-panel { display: flex; flex-direction: column; gap: 8px; margin-bottom: 16px; }
.filter-row { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.filter-label { width: 72px; color: #606266; flex-shrink: 0; }
.filters { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
.warn-tip { color: #e6a23c; font-size: 13px; }
.muted { color: #909399; font-size: 13px; }
.pager { margin-top: 16px; display: flex; justify-content: flex-end; }
.ship-order-info { margin-bottom: 16px; display: flex; align-items: center; flex-wrap: wrap; gap: 4px; }
.ml8 { margin-left: 8px; }
.ship-form { margin-top: 8px; }
.tpl-bar { width: 100%; }
.tpl-radios { display: flex; flex-wrap: wrap; gap: 8px; }
.tpl-radio { margin-right: 0 !important; }
.batch-confirm-hint { margin-bottom: 12px; }
.batch-row { display: grid; grid-template-columns: 160px 1fr; gap: 8px; align-items: center; margin-bottom: 8px; }
.batch-order { font-size: 13px; color: #606266; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.goods-list { display: flex; flex-direction: column; gap: 8px; }
.goods-cell { display: flex; gap: 8px; align-items: flex-start; }
.goods-thumb { width: 40px; height: 40px; object-fit: cover; border-radius: 4px; flex-shrink: 0; }
.goods-text { font-size: 13px; line-height: 1.4; white-space: normal; word-break: break-all; }
</style>
