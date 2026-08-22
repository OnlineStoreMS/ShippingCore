<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Printer, Refresh, Search } from '@element-plus/icons-vue'
import {
  shippingApi,
  shipmentStatusMap,
  parseShipmentRemarkImages,
  type CarrierAccount,
  type ExpressTemplate,
  type OMSOrder,
  type OrderSnapshot,
  type ShipperProfile,
  type Shipment,
} from '../api/shipping'
import { printShipmentByChannel } from '../utils/sfPrintLabel'
import { dateRangeDefaultTime, dateShortcuts, defaultDateRange, formatDateTime } from '../utils/date'
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
import { isKdzsShipment, isSFManagedShipment } from '../utils/shipmentFlags'
import { bindTableShiftWheel, useTableFillHeight } from '../composables/useTableFillHeight'
import { parseChineseRegion, parsePastedContact, saveSFOrderHandoff } from '../utils/sfOrderHandoff'
import {
  openKdzsWithCloudToken,
  type KdzsHandoffOrder,
  type KdzsHandoffPayload,
} from '../utils/kdzsExtension'

const router = useRouter()

const loading = ref(false)
const list = ref<Shipment[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

const pageRef = ref<HTMLElement | null>(null)
const headerRef = ref<HTMLElement | null>(null)
const filtersRef = ref<HTMLElement | null>(null)
const pagerRef = ref<HTMLElement | null>(null)
const tableRef = ref<{ $el?: HTMLElement } | null>(null)
const { tableHeight, updateTableHeight } = useTableFillHeight(pageRef, [headerRef, filtersRef, pagerRef], {
  min: 280,
  gap: 24,
})

let unbindWheel: (() => void) | undefined
onUnmounted(() => unbindWheel?.())

async function rebindWheel() {
  await nextTick()
  unbindWheel?.()
  unbindWheel = bindTableShiftWheel(tableRef.value?.$el ?? null)
  updateTableHeight()
}

type DisplayRow = {
  key: string
  kind: 'single' | 'group'
  primary: Shipment
  peers: Shipment[]
}

/** 同页内按 groupId 折叠拆分发货；同组仅 1 票时按普通单展示（不标拆分） */
const displayRows = computed<DisplayRow[]>(() => {
  const seen = new Set<number>()
  const out: DisplayRow[] = []
  for (const s of list.value) {
    const gid = Number(s.groupId || 0)
    if (gid > 0) {
      if (seen.has(gid)) continue
      seen.add(gid)
      const peers = list.value.filter((x) => Number(x.groupId || 0) === gid)
      if (peers.length <= 1) {
        const only = peers[0] || s
        out.push({ kind: 'single', key: `s${only.id}`, primary: only, peers: [only] })
        continue
      }
      out.push({ kind: 'group', key: `g${gid}`, primary: peers[0], peers })
      continue
    }
    out.push({ kind: 'single', key: `s${s.id}`, primary: s, peers: [s] })
  }
  return out
})

const groupDetailPeers = ref<Shipment[] | null>(null)

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
  shippedRange: defaultDateRange() as [string, string] | null,
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

/** —— 重新发货 —— */
type PrintMode = 'kdzs' | 'sf'
type ReshipStep = 'edit' | 'ship'
type ReshipGoodsLine = {
  key: string
  orderItemId: number
  productName: string
  skuSpecs: string
  outerId: string
  quantity: number
}

const reshipVisible = ref(false)
const reshipLoading = ref(false)
const reshipSubmitting = ref(false)
const reshipStep = ref<ReshipStep>('edit')
const reshipSource = ref<Shipment | null>(null)
const reshipOrder = ref<OMSOrder | null>(null)
const reshipPrintMode = ref<PrintMode>('kdzs')
const reshipSfAction = ref<'standard' | 'quick'>('standard')
const reshipForm = reactive({
  carrierAccountId: undefined as number | undefined,
  shipperProfileId: undefined as number | undefined,
  useMonthly: false,
})
const reshipDraft = reactive({
  pasteText: '',
  receiverName: '',
  receiverMobile: '',
  receiverProvince: '',
  receiverCity: '',
  receiverCounty: '',
  receiverAddress: '',
})
const reshipGoods = ref<ReshipGoodsLine[]>([])
const carrierAccounts = ref<CarrierAccount[]>([])
const shipperProfiles = ref<ShipperProfile[]>([])
const allTemplates = ref<ExpressTemplate[]>([])
const selectedTemplateId = ref('')
const kdzsExpressCompany = ref('')
const kdzsExpressNo = ref('')
const confirmKdzsVisible = ref(false)

/** 重新发货默认按手工单（DFHAND / 菜鸟模板）打单 */
const RESHIP_KDZS_PLATFORM = 'DFHAND'
const RESHIP_TEMPLATE_GROUP = '菜鸟'

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

const filteredTemplates = computed(() =>
  allTemplates.value.filter((t) => t.enabled !== false && t.platform === RESHIP_TEMPLATE_GROUP),
)

const selectedTemplate = computed(() =>
  filteredTemplates.value.find((t) => t.templateId === selectedTemplateId.value),
)

const isSFCarrier = computed(() => {
  const c = carrierAccounts.value.find((x) => x.id === reshipForm.carrierAccountId)
  const code = (c?.carrierCode || '').trim().toUpperCase()
  return !code || code === 'SF' || code === 'SHUNFENG'
})

const primaryReshipLabel = computed(() => {
  if (reshipPrintMode.value === 'kdzs') return '打开快递助手'
  if (isSFCarrier.value && reshipSfAction.value === 'standard') return '前往标准寄件'
  return '快速下单打印'
})

const reshipDialogTitle = computed(() =>
  reshipStep.value === 'edit' ? '重新发货 · 确认收件与商品' : '重新发货 · 打单',
)

const reshipDialogWidth = computed(() => (reshipStep.value === 'edit' ? '860px' : '560px'))

function emptyReshipGoodsLine(): ReshipGoodsLine {
  return {
    key: `g${Date.now()}_${Math.random().toString(36).slice(2, 7)}`,
    orderItemId: 0,
    productName: '',
    skuSpecs: '',
    outerId: '',
    quantity: 1,
  }
}

function resetReshipDraft() {
  reshipDraft.pasteText = ''
  reshipDraft.receiverName = ''
  reshipDraft.receiverMobile = ''
  reshipDraft.receiverProvince = ''
  reshipDraft.receiverCity = ''
  reshipDraft.receiverCounty = ''
  reshipDraft.receiverAddress = ''
  reshipGoods.value = []
}

function prefillsReshipDraft(order: OMSOrder, shipment: Shipment) {
  const addr = order.address
  reshipDraft.receiverName = (addr?.name || order.buyerName || shipment.receiverName || '').trim()
  reshipDraft.receiverMobile = (addr?.phone || order.buyerPhone || shipment.receiverMobile || '').trim()
  reshipDraft.receiverProvince = (addr?.province || shipment.receiverProvince || '').trim()
  reshipDraft.receiverCity = (addr?.city || shipment.receiverCity || '').trim()
  reshipDraft.receiverCounty = (addr?.district || shipment.receiverCounty || '').trim()
  reshipDraft.receiverAddress = (addr?.address || shipment.receiverAddress || '').trim()
  if ((!reshipDraft.receiverProvince || !reshipDraft.receiverCity) && addr?.fullText) {
    const parsed = parseChineseRegion(addr.fullText)
    if (!reshipDraft.receiverProvince) reshipDraft.receiverProvince = parsed.province
    if (!reshipDraft.receiverCity) reshipDraft.receiverCity = parsed.city
    if (!reshipDraft.receiverCounty) reshipDraft.receiverCounty = parsed.county
    if (!reshipDraft.receiverAddress) reshipDraft.receiverAddress = parsed.address
  }
  if (!reshipDraft.receiverAddress && addr?.fullText) {
    reshipDraft.receiverAddress = addr.fullText.trim()
  }
  reshipDraft.pasteText = [
    reshipDraft.receiverName,
    reshipDraft.receiverMobile,
    [reshipDraft.receiverProvince, reshipDraft.receiverCity, reshipDraft.receiverCounty, reshipDraft.receiverAddress]
      .filter(Boolean)
      .join(''),
  ]
    .filter(Boolean)
    .join(' ')

  const roots = (order.items || []).filter(
    (it) => !(it.splitKind || (it.parentOrderItemId && it.parentOrderItemId > 0)),
  )
  if (roots.length) {
    reshipGoods.value = roots.map((it) => ({
      key: `oi${it.id || 0}_${Math.random().toString(36).slice(2, 6)}`,
      orderItemId: it.id || 0,
      productName: (it.productName || '').trim(),
      skuSpecs: (it.skuSpecs || '').trim(),
      outerId: '',
      quantity: it.quantity && it.quantity > 0 ? it.quantity : 1,
    }))
  } else if (shipment.items?.length) {
    reshipGoods.value = shipment.items.map((it) => ({
      key: `si${it.id || 0}_${Math.random().toString(36).slice(2, 6)}`,
      orderItemId: it.orderItemId || 0,
      productName: (it.goodsName || '').trim(),
      skuSpecs: (it.skuCode || '').trim(),
      outerId: (it.outerId || '').trim(),
      quantity: it.quantity || 1,
    }))
  } else if (shipment.cargoName) {
    const line = emptyReshipGoodsLine()
    line.productName = shipment.cargoName
    line.skuSpecs = shipment.cargoName
    reshipGoods.value = [line]
  } else {
    reshipGoods.value = [emptyReshipGoodsLine()]
  }
}

function applyReshipPaste() {
  const raw = reshipDraft.pasteText.trim()
  if (!raw) {
    ElMessage.warning('请先粘贴收件信息')
    return
  }
  const contact = parsePastedContact(raw)
  if (contact.name) reshipDraft.receiverName = contact.name
  if (contact.mobile) reshipDraft.receiverMobile = contact.mobile
  const addrRaw = (contact.address || raw).trim()
  const parsed = parseChineseRegion(addrRaw)
  if (parsed.province) reshipDraft.receiverProvince = parsed.province
  if (parsed.city) reshipDraft.receiverCity = parsed.city
  if (parsed.county) reshipDraft.receiverCounty = parsed.county
  if (parsed.address) reshipDraft.receiverAddress = parsed.address
  else if (addrRaw && !parsed.province) reshipDraft.receiverAddress = addrRaw
  ElMessage.success('已填充收件信息')
}

function addReshipGoods() {
  reshipGoods.value.push(emptyReshipGoodsLine())
}

function removeReshipGoods(idx: number) {
  if (reshipGoods.value.length <= 1) {
    ElMessage.warning('至少保留一行商品')
    return
  }
  reshipGoods.value.splice(idx, 1)
}

function validateReshipEdit(): boolean {
  if (!reshipDraft.receiverName.trim()) {
    ElMessage.warning('请填写收件人')
    return false
  }
  if (!reshipDraft.receiverMobile.trim()) {
    ElMessage.warning('请填写收件手机')
    return false
  }
  if (!reshipDraft.receiverAddress.trim() && !reshipDraft.receiverProvince.trim()) {
    ElMessage.warning('请填写收件地址')
    return false
  }
  const lines = reshipGoods.value.filter((g) => (g.skuSpecs || g.productName || '').trim())
  if (!lines.length) {
    ElMessage.warning('请至少填写一件商品或规格')
    return false
  }
  for (const g of lines) {
    if (!(g.quantity > 0)) {
      ElMessage.warning('商品数量须大于 0')
      return false
    }
  }
  return true
}

function buildReshipSnapshot(order: OMSOrder): OrderSnapshot {
  const goods = reshipGoods.value
    .filter((g) => (g.skuSpecs || g.productName || '').trim())
    .map((g) => {
      const product = g.productName.trim()
      const spec = g.skuSpecs.trim()
      return {
        orderItemId: g.orderItemId || 0,
        title: product || spec,
        skuName: spec || product,
        num: Math.max(1, g.quantity || 1),
        outerId: g.outerId.trim(),
        price: 0,
      }
    })
  return {
    platform: RESHIP_KDZS_PLATFORM,
    shopId: order.shopId || '',
    shopName: order.shopName || order.manualSourceName || '',
    sourceChannel: 'manual',
    manualSourceName: order.manualSourceName || order.shopName || '重新发货',
    orderNo: order.orderNo || '',
    sysTid: order.platformSysTid || '',
    sourceTid: order.platformOrderId || order.orderNo || '',
    receiverName: reshipDraft.receiverName.trim(),
    receiverMobile: reshipDraft.receiverMobile.trim(),
    receiverProvince: reshipDraft.receiverProvince.trim(),
    receiverCity: reshipDraft.receiverCity.trim(),
    receiverCounty: reshipDraft.receiverCounty.trim(),
    receiverAddress: reshipDraft.receiverAddress.trim(),
    goods,
  }
}

function canReship(row: Shipment) {
  return !!row.mailNo?.trim() && row.status !== 'cancelled' && Number(row.orderCoreOrderId || 0) > 0
}

function selectDefaultReshipTemplate() {
  const filtered = filteredTemplates.value
  selectedTemplateId.value = filtered[0]?.templateId || ''
  if (filtered[0]) {
    kdzsExpressCompany.value = inferExpressCompany(filtered[0].templateName)
  }
}

async function openReship(row: Shipment) {
  if (!canReship(row)) {
    ElMessage.warning('仅已发货且绑定订单中心的发货单可重新发货')
    return
  }
  reshipLoading.value = true
  reshipVisible.value = true
  reshipStep.value = 'edit'
  reshipSource.value = row
  reshipOrder.value = null
  confirmKdzsVisible.value = false
  kdzsExpressNo.value = ''
  kdzsExpressCompany.value = ''
  selectedTemplateId.value = ''
  reshipPrintMode.value = 'kdzs'
  reshipSfAction.value = 'standard'
  resetReshipDraft()
  try {
    const [ctx, carriers, shippers, tpls] = await Promise.all([
      shippingApi.getReshipContext(row.id),
      shippingApi.listCarrierAccounts({ page: 1, pageSize: 200, enabled: true }),
      shippingApi.listShipperProfiles({ page: 1, pageSize: 200 }),
      shippingApi.listExpressTemplates({ page: 1, pageSize: 500 }),
    ])
    const shipment = ctx.shipment || row
    const order = ctx.order
    reshipSource.value = shipment
    reshipOrder.value = order
    carrierAccounts.value = carriers.list || []
    shipperProfiles.value = shippers.list || []
    allTemplates.value = tpls.list || []
    const defaultCarrier =
      carrierAccounts.value.find((c) => {
        const code = (c.carrierCode || '').trim().toUpperCase()
        return !code || code === 'SF' || code === 'SHUNFENG'
      }) || carrierAccounts.value[0]
    const defaultShipper = shipperProfiles.value.find((s) => s.isDefault) || shipperProfiles.value[0]
    reshipForm.carrierAccountId = defaultCarrier?.id
    reshipForm.shipperProfileId = defaultShipper?.id
    reshipForm.useMonthly = defaultCarrier?.useMonthly ?? false
    prefillsReshipDraft(order, shipment)
  } catch (e) {
    ElMessage.error((e as Error).message || '加载重新发货信息失败')
    reshipVisible.value = false
  } finally {
    reshipLoading.value = false
  }
}

function goReshipShipStep() {
  if (!validateReshipEdit()) return
  reshipStep.value = 'ship'
  selectDefaultReshipTemplate()
}

function backReshipEditStep() {
  reshipStep.value = 'edit'
  confirmKdzsVisible.value = false
}

function onReshipCarrierChange(id: number | undefined) {
  const carrier = carrierAccounts.value.find((c) => c.id === id)
  if (carrier) reshipForm.useMonthly = carrier.useMonthly
  if (isSFCarrier.value) reshipSfAction.value = 'standard'
}

function onReshipTemplateChange() {
  const tpl = selectedTemplate.value
  const inferred = inferExpressCompany(tpl?.templateName)
  if (inferred) kdzsExpressCompany.value = inferred
}

function closeReshipDialog() {
  reshipVisible.value = false
  confirmKdzsVisible.value = false
  reshipStep.value = 'edit'
  reshipSource.value = null
  reshipOrder.value = null
  selectedTemplateId.value = ''
  kdzsExpressNo.value = ''
  kdzsExpressCompany.value = ''
  resetReshipDraft()
}

function goSFOrderReship(order: OMSOrder) {
  const snap = buildReshipSnapshot(order)
  saveSFOrderHandoff({
    orderId: order.id,
    sourceSystem: 'ordercore',
    carrierAccountId: reshipForm.carrierAccountId,
    shipperProfileId: reshipForm.shipperProfileId,
    useMonthly: reshipForm.useMonthly,
    reship: true,
    order: snap,
  })
  closeReshipDialog()
  detailVisible.value = false
  router.push('/sf-order')
}

async function openKdzsReshipPrint() {
  const order = reshipOrder.value
  if (!order) return
  if (!selectedTemplateId.value) {
    ElMessage.warning('请选择快递模板')
    return
  }
  reshipSubmitting.value = true
  try {
    const platform = RESHIP_KDZS_PLATFORM
    const tpl = selectedTemplate.value
    const snap = buildReshipSnapshot(order)
    const handoffOrders: KdzsHandoffOrder[] = [
      {
        orderNo: order.orderNo || '',
        platformSysTid: '',
        platformOrderId: '',
        sysTid: '',
        tid: '',
        payTime: order.payTime || '',
        orderedAt: order.orderedAt || '',
        goods: (snap.goods || []).map((g) => {
          const name = (g.skuName || g.title || '').trim()
          return {
            title: name,
            skuName: name,
            outerId: g.outerId,
            num: g.num,
          }
        }),
      },
    ]
    const payload: KdzsHandoffPayload = {
      v: 1,
      createdAt: Date.now(),
      platform,
      templateName: tpl?.templateName || '',
      templateId: tpl?.templateId,
      orders: handoffOrders,
      autoPrint: false,
    }
    const session = await shippingApi.createKdzsHelperHandoff(
      payload as unknown as Record<string, unknown>,
    )
    if (!session?.token) throw new Error('创建打单任务失败')
    const data = await shippingApi.getBatchPrintURL(platform)
    const rawUrl = data?.url
    if (!rawUrl) throw new Error('未获取到打单地址')
    const win = openKdzsWithCloudToken(rawUrl, session.token)
    if (!win) {
      ElMessage.warning('浏览器拦截了新窗口，请允许弹窗后重试')
      return
    }
    ElMessage.success(
      '已按手工单打开快递助手并上传打单任务。请确认右下角「OSMS 打单助手」出现订单后，人工选模板/打印；完成后回填新运单号。',
    )
    confirmKdzsVisible.value = true
  } catch (e) {
    ElMessage.error((e as Error).message || '打开快递助手失败')
  } finally {
    reshipSubmitting.value = false
  }
}

async function submitReship() {
  const order = reshipOrder.value
  if (!order) return
  if (!validateReshipEdit()) {
    reshipStep.value = 'edit'
    return
  }
  if (reshipPrintMode.value === 'kdzs') {
    await openKdzsReshipPrint()
    return
  }
  if (isSFCarrier.value && reshipSfAction.value === 'standard') {
    goSFOrderReship(order)
    return
  }
  if (!reshipForm.carrierAccountId || !reshipForm.shipperProfileId) {
    ElMessage.warning('请选择物流账号和寄件人')
    return
  }
  reshipSubmitting.value = true
  try {
    const savedExpress = localStorage.getItem('shippingcore.sf.expressType')
    const expressType = savedExpress === '1' || savedExpress === '2' ? savedExpress : undefined
    const shipment = await shippingApi.createShipmentFromOrder({
      carrierAccountId: reshipForm.carrierAccountId,
      shipperProfileId: reshipForm.shipperProfileId,
      useMonthly: reshipForm.useMonthly,
      expressType,
      orderId: order.id,
      sourceSystem: 'ordercore',
      reship: true,
      order: buildReshipSnapshot(order),
    })
    const waybill = await shippingApi.createShipmentWaybill(shipment.id)
    ElMessage.success(`重新发货成功${waybill.mailNo ? `，新运单号 ${waybill.mailNo}` : ''}`)
    closeReshipDialog()
    detailVisible.value = false
    await load()
    if (waybill.mailNo && canPrint(waybill)) {
      try {
        await printRow(waybill)
      } catch {
        /* 打印失败不阻断 */
      }
    }
  } catch (e) {
    ElMessage.error((e as Error).message || '重新发货失败')
  } finally {
    reshipSubmitting.value = false
  }
}

async function submitKdzsReshipConfirm() {
  const order = reshipOrder.value
  if (!order) return
  const expressNo = kdzsExpressNo.value.trim()
  const company = kdzsExpressCompany.value.trim()
  if (!company) {
    ElMessage.warning('请选择快递公司')
    return
  }
  if (!expressNo) {
    ElMessage.warning('请输入新运单号')
    return
  }
  const oldNo = (reshipSource.value?.mailNo || '').trim()
  if (oldNo && expressNo === oldNo) {
    ElMessage.warning('新运单号不能与原运单号相同')
    return
  }
  reshipSubmitting.value = true
  try {
    await shippingApi.confirmKdzsShip({
      orderId: order.id,
      expressNo,
      expressCompany: company,
      reship: true,
      order: buildReshipSnapshot(order),
    })
    ElMessage.success('已确认重新发货，新运单已追加到原订单发货记录')
    confirmKdzsVisible.value = false
    closeReshipDialog()
    detailVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '确认重新发货失败')
  } finally {
    reshipSubmitting.value = false
  }
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

/** 发货时间=取号/确认发货时间；勿回退打印时间（会因再次打印变化） */
function shipTimeOf(row: Pick<Shipment, 'shippedAt' | 'createdAt' | 'mailNo'>) {
  if (row.shippedAt) return fmtTime(row.shippedAt)
  // 历史单：有运单号时用建单时间近似取号时间
  if (row.mailNo) return fmtTime(row.createdAt)
  return '-'
}

/** 快递助手在助手侧打印，本系统不记打印时间 */
function printTimeOf(row: Pick<Shipment, 'shipVia' | 'printedAt' | 'mailNo' | 'sfOrderId' | 'carrierAccountId'>) {
  if (isKdzsShipment(row)) return '—'
  return fmtTime(row.printedAt)
}

async function load() {
  loading.value = true
  try {
    const params: Record<string, unknown> = {
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
    }
    if (filters.shippedRange?.length === 2) {
      params.shippedAtStart = filters.shippedRange[0]
      params.shippedAtEnd = filters.shippedRange[1]
    }
    const res = await shippingApi.listShipments(params)
    list.value = res.list
    total.value = res.total
    await rebindWheel()
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
  filters.shippedRange = defaultDateRange()
  search()
}

async function loadPromiseTm(ship: Shipment) {
  promiseLabel.value = ''
  promiseHint.value = ''
  if (!ship.mailNo?.trim() || !isSFManagedShipment(ship)) return
  promiseLoading.value = true
  try {
    const res = await shippingApi.searchPromiseTm(ship.id)
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
    groupDetailPeers.value = null
    const gid = Number(detail.value.groupId || 0)
    if (gid > 0) {
      try {
        const g = await shippingApi.getShipmentGroup(gid)
        groupDetailPeers.value = g.shipments || []
      } catch {
        groupDetailPeers.value = list.value.filter((x) => Number(x.groupId || 0) === gid)
      }
    }
    detailVisible.value = true
    void loadPromiseTm(detail.value)
  } catch (e) {
    ElMessage.error((e as Error).message || '加载详情失败')
  }
}

async function openLabelPreview(row: Shipment) {
  labelVisible.value = true
  labelLoading.value = true
  labelPng.value = ''
  labelError.value = ''
  labelTitle.value = row.mailNo ? `面单 ${row.mailNo}` : '面单预览'
  try {
    // 再打印后存档 URL 会变，打开前重新拉详情，避免仍用列表里旧链接
    const fresh = await shippingApi.getShipment(row.id)
    const url = (fresh.labelPdfUrl || row.labelPdfUrl || '').trim()
    labelPdfUrl.value = url
    if (detail.value?.id === fresh.id) detail.value = fresh
    const idx = list.value.findIndex((s) => s.id === fresh.id)
    if (idx >= 0) {
      list.value[idx] = { ...list.value[idx], ...fresh, items: fresh.items?.length ? fresh.items : list.value[idx].items }
    }
    if (!url) {
      labelError.value = '暂无面单存档，打印后约数秒生成，请稍后重试'
      return
    }
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
    ElMessage.success(
      channel === 'pdf'
        ? '已在浏览器打开官方 PDF 面单'
        : '已发送到本机打印机，面单存档数秒内更新',
    )
    const updated = await shippingApi.getShipment(row.id)
    if (detail.value?.id === updated.id) detail.value = updated
    return updated
  })
}

async function cancelRow(row: Shipment) {
  if (isKdzsShipment(row)) {
    ElMessage.warning('快递助手发货单请在快递助手侧取消运单，本系统不支持取消')
    return
  }
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
  return !!row.mailNo && row.status !== 'cancelled' && isSFManagedShipment(row) && !isKdzsShipment(row)
}

function canCancel(row: Shipment) {
  return row.status !== 'cancelled' && !isKdzsShipment(row)
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
  <div ref="pageRef" class="page">
    <el-card v-loading="loading" class="list-card">
      <template #header>
        <div ref="headerRef" class="card-hd">
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

      <div ref="filtersRef" class="filters">
        <el-input
          v-model="filters.keyword"
          clearable
          placeholder="综合搜索：运单号/订单号/收件人/手机"
          :prefix-icon="Search"
          style="width: 280px"
          @keyup.enter="search"
        />
        <el-date-picker
          v-model="filters.shippedRange"
          type="datetimerange"
          range-separator="至"
          start-placeholder="发货开始"
          end-placeholder="发货结束"
          value-format="YYYY-MM-DD HH:mm:ss"
          :shortcuts="dateShortcuts"
          :default-time="dateRangeDefaultTime"
          style="width: 360px"
          @change="search"
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

      <el-table
        ref="tableRef"
        :data="displayRows"
        :height="tableHeight"
        border
        stripe
        empty-text="暂无发货单"
        row-key="key"
      >
        <el-table-column label="订单号" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="order-cell">
              <span>{{ orderNoDisplay(row.primary) }}</span>
              <el-tag v-if="row.kind === 'group'" size="small" type="warning">拆分×{{ row.peers.length }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="订单类型" width="120" show-overflow-tooltip>
          <template #default="{ row }">{{ formatOrderSource(row.primary) }}</template>
        </el-table-column>
        <el-table-column label="平台" width="90">
          <template #default="{ row }">{{ labelPlatform(row.primary.platform) }}</template>
        </el-table-column>
        <el-table-column label="平台单号" min-width="180">
          <template #default="{ row }">
            <div v-if="row.primary.sourceTid">{{ row.primary.sourceTid }}</div>
            <div
              v-if="row.primary.sourceRef && row.primary.sourceRef !== row.primary.sourceTid"
              class="muted"
            >
              系统：{{ row.primary.sourceRef }}
            </div>
            <span v-if="!row.primary.sourceTid && !row.primary.sourceRef">-</span>
          </template>
        </el-table-column>
        <el-table-column label="店铺" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">{{ shopDisplay(row.primary) }}</template>
        </el-table-column>
        <el-table-column label="商品信息" min-width="280">
          <template #default="{ row }">
            <div class="goods-list">
              <template v-for="peer in row.peers" :key="peer.id">
                <div v-if="peer.items?.length">
                  <div v-for="(it, idx) in peer.items" :key="it.id || idx" class="goods-cell">
                    <div class="goods-text">
                      {{ formatGoodsLine(it) || '-' }}
                      <span v-if="row.kind === 'group'" class="muted"> · {{ peer.mailNo || '无单号' }}</span>
                    </div>
                  </div>
                </div>
                <div v-else class="goods-cell">
                  <div class="goods-text">
                    {{ peer.cargoName || '-' }}
                    <span v-if="row.kind === 'group'" class="muted"> · {{ peer.mailNo || '无单号' }}</span>
                  </div>
                </div>
              </template>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="运单号" min-width="150">
          <template #default="{ row }">
            <div v-if="row.kind === 'group'" class="mail-stack">
              <div v-for="peer in row.peers" :key="peer.id">{{ peer.mailNo || '-' }}</div>
            </div>
            <span v-else>{{ row.primary.mailNo || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.primary.status).type" size="small">{{ statusTag(row.primary.status).label }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="收件信息" min-width="200">
          <template #default="{ row }">
            <div class="cell-stack">
              <div class="primary">{{ receiverLines(row.primary).nameMobile || '-' }}</div>
              <div class="secondary addr">{{ receiverLines(row.primary).addr || '-' }}</div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="发货时间" width="170">
          <template #default="{ row }">{{ shipTimeOf(row.primary) }}</template>
        </el-table-column>
        <el-table-column label="打印时间" width="170">
          <template #default="{ row }">{{ printTimeOf(row.primary) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="300" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openDetail(row.primary)">详情</el-button>
            <el-button
              v-if="canReship(row.primary)"
              link
              type="warning"
              size="small"
              @click="openReship(row.primary)"
            >
              重新发货
            </el-button>
            <el-button
              v-if="row.primary.labelPdfUrl"
              link
              type="primary"
              size="small"
              @click="openLabelPreview(row.primary)"
            >
              面单
            </el-button>
            <el-button
              v-if="canRetry(row.primary)"
              link
              type="warning"
              size="small"
              :loading="actionLoading[row.primary.id] === 'waybill'"
              @click="retryWaybill(row.primary)"
            >
              建单
            </el-button>
            <el-button
              v-if="canPrint(row.primary)"
              link
              type="primary"
              size="small"
              :loading="actionLoading[row.primary.id] === 'print'"
              @click="printRow(row.primary)"
            >
              打印
            </el-button>
            <el-button
              v-if="canCancel(row.primary)"
              link
              type="danger"
              size="small"
              :loading="actionLoading[row.primary.id] === 'cancel'"
              @click="cancelRow(row.primary)"
            >
              {{ row.primary.mailNo ? '取消快递单' : '作废' }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div ref="pagerRef" class="pager">
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
          <el-descriptions-item v-if="detail.expressCompany || isKdzsShipment(detail)" label="发货方式">
            {{ isKdzsShipment(detail) ? `快递助手${detail.expressCompany ? ` · ${detail.expressCompany}` : ''}` : detail.expressCompany }}
          </el-descriptions-item>
          <el-descriptions-item label="发货时间">{{ shipTimeOf(detail) }}</el-descriptions-item>
          <el-descriptions-item v-if="!isKdzsShipment(detail)" label="打印时间">{{ fmtTime(detail.printedAt) }}</el-descriptions-item>
          <el-descriptions-item v-if="detail.mailNo && isSFManagedShipment(detail)" label="预计派送">
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
          <el-descriptions-item v-if="isSFManagedShipment(detail) || detail.labelPdfUrl" label="面单">
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

        <div v-if="groupDetailPeers && groupDetailPeers.length > 1" class="items-block">
          <div class="block-title">拆分发货明细</div>
          <el-table :data="groupDetailPeers" border size="small">
            <el-table-column label="运单号" min-width="140">
              <template #default="{ row }">{{ row.mailNo || '-' }}</template>
            </el-table-column>
            <el-table-column label="商品" min-width="180">
              <template #default="{ row }">
                <div v-for="(it, idx) in row.items || []" :key="it.id || idx">
                  {{ formatGoodsLine(it) || row.cargoName || '-' }}
                </div>
                <span v-if="!row.items?.length">{{ row.cargoName || '-' }}</span>
              </template>
            </el-table-column>
            <el-table-column label="快递" width="100" show-overflow-tooltip>
              <template #default="{ row }">{{ row.expressCompany || '-' }}</template>
            </el-table-column>
          </el-table>
        </div>

        <div v-if="detail.items?.length" class="items-block">
          <div class="block-title">商品明细</div>
          <el-table :data="detail.items" border size="small">
            <el-table-column prop="goodsName" label="商品" min-width="160" />
            <el-table-column prop="quantity" label="数量" width="80" />
            <el-table-column prop="outerId" label="商家编码" min-width="120" />
          </el-table>
        </div>

        <div v-if="canReship(detail) || canCancel(detail)" class="detail-actions">
          <el-button v-if="canReship(detail)" type="warning" plain @click="openReship(detail)">
            重新发货
          </el-button>
          <el-button
            v-if="canCancel(detail)"
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

    <el-dialog
      v-model="reshipVisible"
      :title="reshipDialogTitle"
      :width="reshipDialogWidth"
      destroy-on-close
      @close="closeReshipDialog"
    >
      <div v-loading="reshipLoading">
        <template v-if="reshipOrder && reshipSource">
          <div class="ship-order-info">
            <span>订单中心 #{{ reshipOrder.orderNo || reshipOrder.id }}</span>
            <el-tag size="small" type="info" class="ml8">{{ labelPlatform(reshipOrder.platform) }}</el-tag>
            <el-tag size="small" type="warning" class="ml8">追加包裹 · 手工单打单</el-tag>
          </div>
          <div class="muted reship-origin">
            原运单 {{ reshipSource.mailNo || '-' }}
            <span v-if="reshipSource.expressCompany"> · {{ reshipSource.expressCompany }}</span>
            ；不新建订单，新运单号写入原订单发货记录
          </div>

          <!-- 第一步：确认/编辑收件与商品（类似手工建单） -->
          <template v-if="reshipStep === 'edit'">
            <div class="reship-edit">
              <div class="recv-panel">
                <div class="recv-paste">
                  <el-input
                    v-model="reshipDraft.pasteText"
                    type="textarea"
                    :rows="5"
                    resize="none"
                    placeholder="粘贴收件人信息（姓名 手机 地址），点一键填充"
                  />
                  <el-button class="fill-btn" link type="primary" @click="applyReshipPaste">
                    一键填充
                  </el-button>
                </div>
                <div class="recv-fields">
                  <div class="field-row">
                    <span class="field-label">收件人</span>
                    <el-input v-model="reshipDraft.receiverName" placeholder="姓名" class="w-name" />
                    <span class="inline-label">手机</span>
                    <el-input v-model="reshipDraft.receiverMobile" placeholder="手机" class="w-phone" />
                  </div>
                  <div class="field-row">
                    <span class="field-label">省市区</span>
                    <el-input
                      v-model="reshipDraft.receiverProvince"
                      placeholder="省"
                      class="w-pca"
                    />
                    <el-input v-model="reshipDraft.receiverCity" placeholder="市" class="w-pca" />
                    <el-input v-model="reshipDraft.receiverCounty" placeholder="区" class="w-pca" />
                  </div>
                  <div class="field-row">
                    <span class="field-label">详细地址</span>
                    <el-input
                      v-model="reshipDraft.receiverAddress"
                      placeholder="详细地址"
                      class="grow-input"
                    />
                  </div>
                </div>
              </div>

              <div class="goods-block">
                <div class="goods-toolbar">
                  <span class="field-label">商品信息</span>
                  <span class="hint">可修改规格/数量，或添加商品（仅用于本次面单，不改原订单明细）</span>
                </div>
                <el-table :data="reshipGoods" border size="small" empty-text="暂无商品">
                  <el-table-column label="商品名称" min-width="140">
                    <template #default="{ row }">
                      <el-input v-model="row.productName" placeholder="商品名称" />
                    </template>
                  </el-table-column>
                  <el-table-column label="规格名称" min-width="160">
                    <template #default="{ row }">
                      <el-input v-model="row.skuSpecs" placeholder="面单托寄物优先用规格" />
                    </template>
                  </el-table-column>
                  <el-table-column label="商家编码" width="110">
                    <template #default="{ row }">
                      <el-input v-model="row.outerId" placeholder="可选" />
                    </template>
                  </el-table-column>
                  <el-table-column label="数量" width="120">
                    <template #default="{ row }">
                      <el-input-number
                        v-model="row.quantity"
                        :min="1"
                        :precision="0"
                        controls-position="right"
                        class="goods-qty"
                      />
                    </template>
                  </el-table-column>
                  <el-table-column label="操作" width="70" fixed="right">
                    <template #default="{ $index }">
                      <el-button link type="danger" @click="removeReshipGoods($index)">删除</el-button>
                    </template>
                  </el-table-column>
                </el-table>
                <div class="goods-footer">
                  <el-button link type="primary" @click="addReshipGoods">+ 添加商品</el-button>
                </div>
              </div>
            </div>
          </template>

          <!-- 第二步：打单方式（默认快递助手手工单） -->
          <template v-else>
            <div class="reship-summary">
              <div>
                <span class="muted">收件</span>
                {{ reshipDraft.receiverName }} {{ reshipDraft.receiverMobile }}
              </div>
              <div class="addr">
                {{
                  [
                    reshipDraft.receiverProvince,
                    reshipDraft.receiverCity,
                    reshipDraft.receiverCounty,
                    reshipDraft.receiverAddress,
                  ]
                    .filter(Boolean)
                    .join('')
                }}
              </div>
              <div class="goods-list">
                <div
                  v-for="g in reshipGoods.filter((x) => (x.skuSpecs || x.productName).trim())"
                  :key="g.key"
                  class="goods-text"
                >
                  {{ (g.skuSpecs || g.productName).trim() }} ×{{ g.quantity }}
                </div>
              </div>
              <el-button link type="primary" @click="backReshipEditStep">返回修改</el-button>
            </div>

            <el-form label-width="100px" class="ship-form">
              <el-form-item label="打单方式">
                <el-radio-group v-model="reshipPrintMode">
                  <el-radio value="kdzs">快递助手（手工单）</el-radio>
                  <el-radio value="sf">自建物流</el-radio>
                </el-radio-group>
              </el-form-item>

              <template v-if="reshipPrintMode === 'kdzs'">
                <el-form-item label="快递模板" required>
                  <div v-if="filteredTemplates.length" class="tpl-bar">
                    <el-radio-group
                      v-model="selectedTemplateId"
                      class="tpl-radios"
                      @change="onReshipTemplateChange"
                    >
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
                    title="暂无「菜鸟」手工单快递模板，请先到「快递模板」页同步"
                  />
                </el-form-item>
                <el-alert
                  type="info"
                  :closable="false"
                  :title="
                    selectedTemplate
                      ? `将以手工单打开快递助手（模板「${selectedTemplate.templateName}」）；完成后回填新运单号。原运单保留。`
                      : '请先选择快递模板'
                  "
                />
              </template>

              <template v-else>
                <el-form-item label="物流账号" required>
                  <el-select
                    v-model="reshipForm.carrierAccountId"
                    placeholder="选择物流账号"
                    style="width: 100%"
                    @change="onReshipCarrierChange"
                  >
                    <el-option
                      v-for="c in carrierAccounts"
                      :key="c.id"
                      :label="c.name"
                      :value="c.id!"
                    />
                  </el-select>
                </el-form-item>
                <el-form-item label="寄件人" required>
                  <el-select
                    v-model="reshipForm.shipperProfileId"
                    placeholder="选择寄件人"
                    style="width: 100%"
                  >
                    <el-option
                      v-for="s in shipperProfiles"
                      :key="s.id"
                      :label="s.isDefault ? `${s.name}（默认）` : s.name"
                      :value="s.id!"
                    />
                  </el-select>
                </el-form-item>
                <el-form-item label="月结">
                  <el-switch v-model="reshipForm.useMonthly" />
                </el-form-item>
                <el-form-item v-if="isSFCarrier" label="寄件方式">
                  <el-radio-group v-model="reshipSfAction">
                    <el-radio value="standard">顺丰标准寄件</el-radio>
                    <el-radio value="quick">快速下单打印</el-radio>
                  </el-radio-group>
                </el-form-item>
                <el-alert
                  type="info"
                  :closable="false"
                  :title="
                    isSFCarrier && reshipSfAction === 'standard'
                      ? '将进入顺丰标准寄件页下单；新运单追加到原订单发货记录。'
                      : '将快速取号并打印；新运单追加到原订单发货记录。'
                  "
                />
              </template>
            </el-form>
          </template>
        </template>
      </div>
      <template #footer>
        <el-button @click="closeReshipDialog">取消</el-button>
        <template v-if="reshipStep === 'edit'">
          <el-button
            type="primary"
            :disabled="reshipLoading || !reshipOrder"
            @click="goReshipShipStep"
          >
            下一步：打单发货
          </el-button>
        </template>
        <template v-else>
          <el-button @click="backReshipEditStep">上一步</el-button>
          <el-button
            type="primary"
            :loading="reshipSubmitting"
            :disabled="reshipLoading || !reshipOrder || (reshipPrintMode === 'kdzs' && !selectedTemplateId)"
            @click="submitReship"
          >
            {{ primaryReshipLabel }}
          </el-button>
        </template>
      </template>
    </el-dialog>

    <el-dialog v-model="confirmKdzsVisible" title="确认已打单发货" width="440px" append-to-body>
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
        <el-form-item label="新运单号" required>
          <el-input v-model="kdzsExpressNo" placeholder="快递助手打单后的新运单号" />
        </el-form-item>
        <el-alert
          type="warning"
          :closable="false"
          :title="`原运单 ${reshipSource?.mailNo || '-'} 保留；请勿填入原单号`"
        />
      </el-form>
      <template #footer>
        <el-button @click="confirmKdzsVisible = false">取消</el-button>
        <el-button type="primary" :loading="reshipSubmitting" @click="submitKdzsReshipConfirm">
          确认发货
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 56px - 32px);
  min-height: 0;
  overflow: hidden;
}
.list-card {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.list-card :deep(.el-card__body) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
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
  margin-bottom: 12px;
  flex-wrap: wrap;
  align-items: center;
  flex-shrink: 0;
}
.pager {
  display: flex;
  justify-content: flex-end;
  flex-shrink: 0;
  padding-top: 12px;
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
.goods-list { display: flex; flex-direction: column; gap: 6px; }
.goods-cell { display: flex; gap: 8px; align-items: flex-start; }
.order-cell { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.mail-stack { display: flex; flex-direction: column; gap: 2px; font-size: 13px; }
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
.ship-order-info {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: 8px;
  font-weight: 600;
}
.reship-origin {
  margin-bottom: 12px;
  line-height: 1.5;
}
.reship-edit {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.recv-panel {
  display: grid;
  grid-template-columns: 240px 1fr;
  gap: 12px;
}
.recv-paste {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.fill-btn {
  align-self: flex-start;
}
.recv-fields {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.field-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.field-label {
  width: 64px;
  flex-shrink: 0;
  color: #606266;
  font-size: 13px;
}
.inline-label {
  color: #909399;
  font-size: 12px;
}
.w-name {
  width: 120px;
}
.w-phone {
  width: 140px;
}
.w-pca {
  width: 100px;
}
.grow-input {
  flex: 1;
  min-width: 180px;
}
.goods-block {
  margin-top: 4px;
}
.goods-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
  flex-wrap: wrap;
}
.goods-toolbar .hint {
  color: #909399;
  font-size: 12px;
}
.goods-footer {
  margin-top: 8px;
}
.goods-qty {
  width: 100%;
}
.reship-summary {
  margin-bottom: 14px;
  padding: 10px 12px;
  background: #f8f9fb;
  border-radius: 6px;
  font-size: 13px;
  line-height: 1.6;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ship-form {
  margin-top: 8px;
}
.tpl-bar {
  width: 100%;
}
.tpl-radios {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.tpl-radio {
  margin-right: 0 !important;
}
.detail-actions {
  margin-top: 20px;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
@media (max-width: 720px) {
  .recv-panel {
    grid-template-columns: 1fr;
  }
}
</style>
