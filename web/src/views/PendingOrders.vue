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
  type ShipperProfile,
  type ShipPlanLine,
} from '../api/shipping'
import {
  omsOrderToSnapshot,
  remainingQtyByItem,
  saveSFOrderHandoff,
  shippedQtyByItem,
} from '../utils/sfOrderHandoff'
import { printShipmentByChannel } from '../utils/sfPrintLabel'
import {
  getSavedPrinterIndex,
  listLocalPrinters,
  savePrinterSelection,
  type LocalPrinter,
} from '../utils/sfPrintPlugin'
import {
  openKdzsWithCloudToken,
  type KdzsHandoffOrder,
  type KdzsHandoffPayload,
} from '../utils/kdzsExtension'

const TEMPLATE_MEMORY_KEY = 'shippingcore.sf.printTemplateKey'

type LabelTemplateOpt = {
  key: string
  label: string
  templateCode: string
  customTemplateCode: string
}

/** 顶层：快递助手 | 自建物流 */
type PrintMode = 'kdzs' | 'sf'
/** 自建物流 + 顺丰账号时的子方式 */
type SFShipAction = 'standard' | 'quick'

const route = useRoute()
const router = useRouter()

const loading = reactive({ orders: false, ship: false, syncWaybill: false, splitSave: false })
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
/** 顺丰：标准寄件页 / 快速下单后选模板打印机 */
const sfShipAction = ref<SFShipAction>('standard')
const selectedTemplateId = ref('')
const shipForm = reactive({
  carrierAccountId: undefined as number | undefined,
  shipperProfileId: undefined as number | undefined,
  useMonthly: false,
})
const kdzsExpressCompany = ref('')
const kdzsExpressRows = ref<{ order: OMSOrder; expressNo: string }[]>([])

/** 打单勾选：统一可发货行（计划规格行 或 未拆原商品） */
type ShipPickRow = {
  key: string
  kind: 'plan' | 'item'
  planLineId?: number
  orderItemId: number
  itemIndex?: number
  label: string
  skuName: string
  /** 待发上限 */
  maxQty: number
  /** 本次发货件数（可改，默认=待发） */
  shipQty: number
  picUrl?: string
  /** KDZS 确认时按行填运单（多选混发时也可共用一个号） */
  expressNo?: string
}
const shipPickRows = ref<ShipPickRow[]>([])
const selectedShipKeys = ref<string[]>([])
const pendingShipPlanLines = ref<ShipPlanLine[]>([])

/** 待发货「拆分」编辑 */
const splitDialogVisible = ref(false)
const splitTargetOrder = ref<OMSOrder | null>(null)
/** partial=按商品拆分；full=整单拆分（全部按新规格行发货） */
const splitEditMode = ref<'partial' | 'full'>('partial')
type SplitDraftLine = {
  key: string
  orderItemId: number
  itemIndex: number
  skuName: string
  qty: number
}
const splitDraftLines = ref<SplitDraftLine[]>([])
let splitDraftSeq = 0

/** 云打印：取单成功后面单模板 + 打印机确认 */
const printDialogVisible = ref(false)
const printDialogLoading = ref(false)
const printersLoading = ref(false)
const printers = ref<LocalPrinter[]>([])
const selectedPrinterIndex = ref<number | null>(getSavedPrinterIndex())
const templateOptions = ref<LabelTemplateOpt[]>([])
const selectedTemplateKey = ref('')
const pendingPrint = ref<{
  shipmentId: number
  mailNo: string
  printChannel: string
} | null>(null)

function buildTemplateOptions(carrier?: CarrierAccount): LabelTemplateOpt[] {
  if (!carrier) return []
  const std = (carrier.templateCode || '').trim()
  // 仅标准模板（自定义区暂不启用）
  if (!std || std.includes('_custom_')) return []
  return [
    {
      key: 'std',
      label: `标准模板（${std}）`,
      templateCode: std,
      customTemplateCode: '__none__',
    },
  ]
}

function rememberTemplateKey(key: string) {
  if (key) localStorage.setItem(TEMPLATE_MEMORY_KEY, key)
}

function loadRememberedTemplateKey(opts: LabelTemplateOpt[]): string {
  return opts[0]?.key || ''
}

async function loadPrintersForDialog() {
  printersLoading.value = true
  try {
    printers.value = await listLocalPrinters()
    const saved = getSavedPrinterIndex()
    if (saved != null && printers.value.some((p) => p.index === saved)) {
      selectedPrinterIndex.value = saved
    } else if (printers.value.length) {
      selectedPrinterIndex.value = printers.value[0].index
    } else {
      selectedPrinterIndex.value = null
    }
  } catch (e) {
    printers.value = []
    selectedPrinterIndex.value = null
    ElMessage.warning((e as Error).message || '无法读取本机打印机，请确认 C-Lodop 已启动')
  } finally {
    printersLoading.value = false
  }
}

async function openCloudPrintDialog(opts: {
  shipmentId: number
  mailNo: string
  carrier?: CarrierAccount
}) {
  const channel = (opts.carrier?.printChannel || 'plugin').toLowerCase()
  pendingPrint.value = {
    shipmentId: opts.shipmentId,
    mailNo: opts.mailNo,
    printChannel: channel,
  }
  templateOptions.value = buildTemplateOptions(opts.carrier)
  selectedTemplateKey.value = loadRememberedTemplateKey(templateOptions.value)
  printDialogVisible.value = true
  await loadPrintersForDialog()
}

async function confirmCloudPrint() {
  const pending = pendingPrint.value
  if (!pending) return
  if (selectedPrinterIndex.value == null) {
    ElMessage.warning('请选择打印机')
    return
  }
  const tpl = templateOptions.value.find((t) => t.key === selectedTemplateKey.value)
  if (!tpl && templateOptions.value.length) {
    ElMessage.warning('请选择面单模板')
    return
  }
  printDialogLoading.value = true
  try {
    const p = printers.value.find((x) => x.index === selectedPrinterIndex.value)
    savePrinterSelection(selectedPrinterIndex.value, p?.name)
    if (selectedTemplateKey.value) rememberTemplateKey(selectedTemplateKey.value)
    await printShipmentByChannel({
      shipmentId: pending.shipmentId,
      printChannel: pending.printChannel,
      printerIndex: selectedPrinterIndex.value,
      templateCode: tpl?.templateCode,
      customTemplateCode: '__none__',
    })
    ElMessage.success(`打印成功 ${pending.mailNo || ''}`)
    printDialogVisible.value = false
    pendingPrint.value = null
    closeShipDialog()
    selectedOrders.value = []
    await router.push('/shipments')
  } catch (e) {
    ElMessage.error((e as Error).message || '打印失败')
  } finally {
    printDialogLoading.value = false
  }
}

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

function formatOrderSource(row: {
  sourceChannel?: string
  manualSourceName?: string
  shopName?: string
  platform?: string
}) {
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

function labelPlatform(v?: string) {
  const key = (v || '').trim().toUpperCase()
  return (key && platformLabels[key]) || v || '-'
}

function formatGoodsLine(g?: { productName?: string; skuSpecs?: string; quantity?: number } | null): string {
  if (!g) return ''
  // 列表展示也以规格名称为主（真正发货内容）
  const spec = g.skuSpecs?.trim() || ''
  const name = g.productName?.trim() || ''
  const title = spec || name
  if (!title) return ''
      const num = g.quantity && g.quantity > 0 ? g.quantity : 1
  return `${title} x${num}`
}

/** 订单中心拆分子行（同步后出现在 items 里），列表/拆分编辑只展示原商品根行 */
function isOMSSplitChild(it?: { splitKind?: string; parentOrderItemId?: number } | null) {
  if (!it) return false
  return !!(it.splitKind || (it.parentOrderItemId && it.parentOrderItemId > 0))
}

function rootOMSItems(order?: OMSOrder | null) {
  return (order?.items || []).filter((it) => !isOMSSplitChild(it))
}

/** 订单同步后根行 id 可能变化，拆分计划上的 orderItemId 会失效；尽量重绑到当前根行 */
function rematchPlanParentId(
  order: OMSOrder,
  orderItemId?: number,
  splitOrderItemId?: number,
): number {
  const roots = rootOMSItems(order)
  const want = Number(orderItemId || 0)
  if (want > 0 && roots.some((r) => r.id === want)) return want
  if (splitOrderItemId) {
    const child = (order.items || []).find((it) => it.id === splitOrderItemId)
    const parentId = Number(child?.parentOrderItemId || 0)
    if (parentId > 0 && roots.some((r) => r.id === parentId)) return parentId
  }
  // 单商品单：旧 id 失效时整批挂到唯一根行
  if (roots.length === 1 && roots[0].id) return roots[0].id
  return 0
}

function healShipPlanLines(order: OMSOrder, lines: ShipPlanLine[]): ShipPlanLine[] {
  if (!lines?.length) return lines || []
  return lines.map((l) => {
    if (!l.orderItemId) return l
    const next = rematchPlanParentId(order, l.orderItemId, l.splitOrderItemId)
    if (!next || next === l.orderItemId) return l
    return { ...l, orderItemId: next }
  })
}

/** 付款时间：YYYY-MM-DD HH:mm:ss */
function formatPayTime(v?: string) {
  if (!v) return '-'
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) {
    return String(v).replace('T', ' ').replace(/\.\d+/, '').replace(/[Zz]|[+-]\d{2}:?\d{2}$/, '').trim().slice(0, 19) || '-'
  }
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

/** 快递助手下单时间筛选：按任务订单付款/下单日聚合为当日 00:00:00 ~ 23:59:59 */
function buildKdzsOrderTimeRange(orders: OMSOrder[]): { from: string; to: string } | null {
  const times: number[] = []
  for (const o of orders) {
    const raw = o.payTime || o.orderedAt
    if (!raw) continue
    const d = new Date(raw)
    if (!Number.isNaN(d.getTime())) times.push(d.getTime())
  }
  if (!times.length) return null
  const min = new Date(Math.min(...times))
  const max = new Date(Math.max(...times))
  const p = (n: number) => String(n).padStart(2, '0')
  const ymd = (d: Date) => `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
  return {
    from: `${ymd(min)} 00:00:00`,
    to: `${ymd(max)} 23:59:59`,
  }
}

/** 列表商品展示：有拆分计划时用规格行替换被拆原商品（原 xN 不再作为发货口径） */
function goodsRowsWithShipMark(order: OMSOrder) {
  const shippedMap = shippedQtyByItem(order)
  const plans = order.shipPlanLines || []
  const pendingPlans = plans.filter((l) => l.status === 'pending')
  const shippedPlans = plans.filter((l) => l.status === 'shipped')
  const isFullOrderPlan =
    pendingPlans.some((l) => !l.orderItemId) ||
    (pendingPlans.length === 0 &&
      shippedPlans.length > 0 &&
      shippedPlans.every((l) => !l.orderItemId))

  type Row = {
    key: string
    kind: 'plan' | 'item'
    title: string
    picUrl?: string
    shipped: number
    total: number
    fullyShipped: boolean
    isSplit: boolean
  }
  const rows: Row[] = []
  const rootItems = rootOMSItems(order)

  if (isFullOrderPlan) {
    for (const p of [...pendingPlans, ...shippedPlans]) {
      const qty = Math.max(1, p.qty || 1)
      const done = p.status === 'shipped'
      rows.push({
        key: `plan:${p.id}`,
        kind: 'plan',
        title: `${(p.skuName || '').trim() || `规格#${p.id}`} x${qty}`,
        shipped: done ? qty : 0,
        total: qty,
        fullyShipped: done,
        isSplit: true,
      })
    }
    return rows
  }

  if (plans.length) {
    const covered = new Set(plans.map((l) => l.orderItemId).filter((id) => id > 0))
    for (const p of pendingPlans) {
      const qty = Math.max(1, p.qty || 1)
      const item = rootItems.find((it) => it.id === p.orderItemId)
      rows.push({
        key: `plan:${p.id}`,
        kind: 'plan',
        title: `${(p.skuName || '').trim() || `规格#${p.id}`} x${qty}`,
        picUrl: item?.picUrl,
        shipped: 0,
        total: qty,
        fullyShipped: false,
        isSplit: true,
      })
    }
    for (const p of shippedPlans) {
      if (!p.orderItemId) continue
      const qty = Math.max(1, p.qty || 1)
      const item = rootItems.find((it) => it.id === p.orderItemId)
      rows.push({
        key: `plan-shipped:${p.id}`,
        kind: 'plan',
        title: `${(p.skuName || '').trim() || `规格#${p.id}`} x${qty}`,
        picUrl: item?.picUrl,
        shipped: qty,
        total: qty,
        fullyShipped: true,
        isSplit: true,
      })
    }
    rootItems.forEach((g, idx) => {
      if (g.id && covered.has(g.id)) return
      const shipped = g.id ? shippedMap[g.id] || 0 : 0
      const total = g.quantity || 0
      rows.push({
        key: `item:${idx}`,
        kind: 'item',
        title: formatGoodsLine(g) || '-',
        picUrl: g.picUrl,
        shipped,
        total,
        fullyShipped: shipped > 0 && total > 0 && shipped >= total,
        isSplit: false,
      })
    })
    return rows
  }

  return rootItems.map((g, idx) => {
    const shipped = g.id ? shippedMap[g.id] || 0 : 0
    const total = g.quantity || 0
    return {
      key: `item:${idx}`,
      kind: 'item' as const,
      title: formatGoodsLine(g) || '-',
      picUrl: g.picUrl,
      shipped,
      total,
      fullyShipped: shipped > 0 && total > 0 && shipped >= total,
      isSplit: false,
    }
  })
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

const selectedPickRows = computed(() => {
  const set = new Set(selectedShipKeys.value)
  return shipPickRows.value.filter((r) => set.has(r.key))
})

const shipPickAllSelected = computed(
  () =>
    shipPickRows.value.length > 0 &&
    selectedShipKeys.value.length === shipPickRows.value.length,
)

const shipPickIndeterminate = computed(() => {
  const n = selectedShipKeys.value.length
  return n > 0 && n < shipPickRows.value.length
})

/** 确认发货时：多行勾选默认同一运单；若需分票可对每行填不同号（走拆分确认） */
const needPerRowExpress = computed(
  () => !isBatchShip.value && selectedPickRows.value.length > 1,
)

const selectedCarrier = computed(() =>
  carrierAccounts.value.find((c) => c.id === shipForm.carrierAccountId),
)

/** 当前所选物流账号是否为顺丰 */
const isSFCarrier = computed(() => {
  const code = (selectedCarrier.value?.carrierCode || '').trim().toUpperCase()
  return !code || code === 'SF' || code === 'SHUNFENG' || code === '顺丰'
})

function defaultPrintMode(orders: OMSOrder[], preferred?: PrintMode): PrintMode {
  if (orders.length > 1) return 'kdzs'
  if (preferred === 'kdzs' || preferred === 'sf') return preferred
  // 单笔且有顺丰账号时，默认自建物流（内含标准寄件）
  const hasSF = carrierAccounts.value.some((c) => {
    const code = (c.carrierCode || '').trim().toUpperCase()
    return !code || code === 'SF' || code === 'SHUNFENG'
  })
  return hasSF ? 'sf' : 'kdzs'
}

async function attachPendingPlanCounts(orders: OMSOrder[]) {
  const ids = orders.map((o) => o.id).filter((id) => id > 0)
  if (!ids.length) return
  try {
    const { counts } = await shippingApi.countPendingShipPlans(ids)
    for (const o of orders) {
      o.pendingPlanCount = counts[String(o.id)] || 0
      o.shipPlanLines = o.shipPlanLines || []
    }
    // 有拆分（待发或已部分发）的订单拉取计划行，列表用规格行替换原商品展示
    const needPlans = orders.filter(
      (o) => (o.pendingPlanCount || 0) > 0 || o.shipStatus === 'partial_shipped',
    )
    await Promise.all(
      needPlans.map(async (o) => {
        try {
          const { list } = await shippingApi.getShipPlan(o.id)
          o.shipPlanLines = healShipPlanLines(o, list || [])
          o.pendingPlanCount = (o.shipPlanLines || []).filter((l) => l.status === 'pending').length
        } catch {
          o.shipPlanLines = []
        }
      }),
    )
  } catch {
    /* optional */
  }
}

async function loadOmsOrders() {
  loading.orders = true
  try {
    const data = await shippingApi.listPendingOMSOrders({
      page: omsFilters.page,
      pageSize: omsFilters.pageSize,
      shipStatus: 'need_ship',
      allocType: 'self_ship',
      sourceChannel: omsFilters.sourceChannel || undefined,
      platform: omsFilters.platform || undefined,
      keyword: omsFilters.keyword || undefined,
    })
    const list = data.list || []
    await attachPendingPlanCounts(list)
    omsOrders.value = list
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

function buildShipPickRows(order: OMSOrder, planLines: ShipPlanLine[]): ShipPickRow[] {
  const remaining = remainingQtyByItem(order)
  const pending = planLines.filter((l) => l.status === 'pending')
  const rows: ShipPickRow[] = []

  for (const line of pending) {
    const item = (order.items || []).find((it) => it.id === line.orderItemId)
    const spec = (line.skuName || '').trim()
    const maxQty = Math.max(1, line.qty || 1)
    rows.push({
      key: `plan:${line.id}`,
      kind: 'plan',
      planLineId: line.id,
      orderItemId: line.splitOrderItemId || line.orderItemId || 0,
      label: spec || item?.productName || `规格#${line.id}`,
      skuName: spec,
      maxQty,
      shipQty: maxQty,
      picUrl: item?.picUrl,
      expressNo: '',
    })
  }

  // 整单拆分（存在 orderItemId=0 的计划行）：打单只认计划行
  const isFullOrderPlan = pending.some((l) => !l.orderItemId)
  if (isFullOrderPlan) {
    return rows
  }

  const covered = new Set(pending.map((l) => l.orderItemId).filter((id) => id > 0))
  ;(order.items || []).forEach((item, index) => {
    if (!item?.id || covered.has(item.id)) return
    // 订单中心拆分子行由计划行承载，勿再作为原商品勾选
    if (isOMSSplitChild(item)) return
    const left = remaining[item.id] ?? item.quantity ?? 0
    if (left <= 0) return
    const spec = (item.skuSpecs || '').trim()
    const product = (item.productName || '').trim()
    const name = spec || product || `商品#${item.id}`
    const maxQty = left
    rows.push({
      key: `item:${index}`,
      kind: 'item',
      orderItemId: item.id,
      itemIndex: index,
      label: name,
      skuName: name,
      maxQty,
      shipQty: maxQty,
      picUrl: item.picUrl,
      expressNo: '',
    })
  })

  return rows
}

async function initShipPickSelection(order: OMSOrder) {
  try {
    const { list } = await shippingApi.getShipPlan(order.id, 'pending')
    pendingShipPlanLines.value = list || []
  } catch {
    pendingShipPlanLines.value = []
  }
  shipPickRows.value = buildShipPickRows(order, pendingShipPlanLines.value)
  selectedShipKeys.value = shipPickRows.value.map((r) => r.key)
}

function toggleShipPickAll(checked: boolean) {
  selectedShipKeys.value = checked ? shipPickRows.value.map((r) => r.key) : []
}

/** 按勾选的可发货行生成快照；拆分行须已同步到订单中心子行 orderItemId */
function buildCheckedShipSnapshot(order: OMSOrder) {
  const base = omsOrderToSnapshot(order)
  const goods: {
    orderItemId: number
    planLineId: number
    title: string
    skuName: string
    num: number
    outerId: string
    price: number
  }[] = []

  for (const r of selectedPickRows.value) {
    const spec = (r.skuName || '').trim()
    const need = Math.min(Math.max(1, r.shipQty || 1), Math.max(1, r.maxQty || 1))
    const planLineId = r.planLineId || 0
    if (!(r.orderItemId > 0)) {
      throw new Error(`「${spec || '规格'}」尚未同步订单中心子行，请重新保存拆分后再打单`)
    }
    goods.push({
      orderItemId: r.orderItemId,
      planLineId,
      title: spec,
      skuName: spec,
      num: need,
      outerId: '',
      price: 0,
    })
  }

  return { ...base, goods }
}

function snapshotForShip(order: OMSOrder) {
  if (isBatchShip.value || !shipPickRows.value.length) {
    return omsOrderToSnapshot(order)
  }
  return buildCheckedShipSnapshot(order)
}

function ensureShipItemsSelected(): boolean {
  if (isBatchShip.value) return true
  if (!shipPickRows.value.length) return true
  if (!selectedShipKeys.value.length) {
    ElMessage.warning('请至少勾选一件要发货的商品或规格')
    return false
  }
  for (const r of selectedPickRows.value) {
    if (!(r.skuName || '').trim()) {
      ElMessage.warning('发货规格名称不能为空')
      return false
    }
    if (!(r.orderItemId > 0)) {
      ElMessage.warning('拆分规格尚未同步订单中心子行，请重新保存拆分后再打单')
      return false
    }
    const maxQty = Math.max(1, r.maxQty || 1)
    if (!(r.shipQty > 0) || r.shipQty > maxQty) {
      ElMessage.warning(`发货件数须在 1～${maxQty} 之间`)
      return false
    }
  }
  return true
}

function goSFOrder(order: OMSOrder) {
  const snap = buildCheckedShipSnapshot(order)
  saveSFOrderHandoff({
    orderId: order.id,
    sourceSystem: 'ordercore',
    carrierAccountId: shipForm.carrierAccountId,
    shipperProfileId: shipForm.shipperProfileId,
    useMonthly: shipForm.useMonthly,
    order: snap,
  })
  router.push('/sf-order')
}

function normalizePrintMode(raw?: string | null): PrintMode {
  const v = (raw || '').trim().toLowerCase()
  if (v === 'sf' || v === 'carrier' || v === 'sf_standard' || v === 'standard' || v === 'print') {
    return 'sf'
  }
  if (v === 'kdzs') return 'kdzs'
  return 'sf'
}

async function prepareShipDialog(orders: OMSOrder[], preferredMode?: PrintMode) {
  shipTargets.value = orders
  selectedTemplateId.value = ''
  kdzsExpressCompany.value = ''
  kdzsExpressRows.value = orders.map((order) => ({ order, expressNo: '' }))
  shipPickRows.value = []
  selectedShipKeys.value = []
  pendingShipPlanLines.value = []
  if (orders.length === 1) await initShipPickSelection(orders[0])
  const defaultCarrier =
    carrierAccounts.value.find((c) => {
      const code = (c.carrierCode || '').trim().toUpperCase()
      return !code || code === 'SF' || code === 'SHUNFENG'
    }) || carrierAccounts.value[0]
  const defaultShipper = shipperProfiles.value.find((s) => s.isDefault) || shipperProfiles.value[0]
  shipForm.carrierAccountId = defaultCarrier?.id
  shipForm.shipperProfileId = defaultShipper?.id
  shipForm.useMonthly = defaultCarrier?.useMonthly ?? false
  printMode.value = defaultPrintMode(orders, preferredMode)
  sfShipAction.value = 'standard'

  const tpls = allTemplates.value.filter(
    (t) => t.enabled !== false && t.platform === templatePlatformGroup(orders[0]),
  )
  if (tpls.length) {
    selectedTemplateId.value = tpls[0].templateId
    kdzsExpressCompany.value = inferExpressCompany(tpls[0].templateName)
  }

  shipDialogVisible.value = true
}

/** —— 待发货拆分编辑 —— */
function splitDraftLinesForItem(itemIndex: number) {
  return splitDraftLines.value.filter((l) => l.itemIndex === itemIndex)
}

function addSplitDraftLine(itemIndex: number, orderItemId: number) {
  splitDraftSeq += 1
  splitDraftLines.value.push({
    key: `d${splitDraftSeq}`,
    itemIndex,
    orderItemId,
    skuName: '',
    qty: 1,
  })
}

function addFullSplitDraftLine() {
  addSplitDraftLine(-1, 0)
}

function removeSplitDraftLine(key: string) {
  splitDraftLines.value = splitDraftLines.value.filter((l) => l.key !== key)
}

function clearSplitDraftForItem(itemIndex: number) {
  splitDraftLines.value = splitDraftLines.value.filter((l) => l.itemIndex !== itemIndex)
}

function onSplitEditModeChange(mode: 'partial' | 'full') {
  splitEditMode.value = mode
  splitDraftLines.value = []
  splitDraftSeq = 0
  if (mode === 'full') {
    addFullSplitDraftLine()
  }
}

async function openSplitDialog(order: OMSOrder) {
  splitTargetOrder.value = order
  splitDraftLines.value = []
  splitDraftSeq = 0
  splitEditMode.value = 'partial'
  try {
    const { list } = await shippingApi.getShipPlan(order.id)
    const pending = healShipPlanLines(order, (list || []).filter((l) => l.status === 'pending'))
    const isFull = pending.length > 0 && pending.every((l) => !l.orderItemId)
    splitEditMode.value = isFull ? 'full' : 'partial'
    const roots = rootOMSItems(order)
    let needsPersistHeal = false
    for (const line of pending) {
      if (isFull || !line.orderItemId) {
        splitDraftSeq += 1
        splitDraftLines.value.push({
          key: `d${splitDraftSeq}`,
          itemIndex: -1,
          orderItemId: 0,
          skuName: line.skuName,
          qty: Math.max(1, line.qty || 1),
        })
        continue
      }
      const rawId = Number(line.orderItemId || 0)
      const parentId = rematchPlanParentId(order, rawId, line.splitOrderItemId)
      const itemIndex = roots.findIndex((it) => it.id === parentId)
      if (itemIndex < 0 || !parentId) continue
      if (parentId !== rawId) needsPersistHeal = true
      splitDraftSeq += 1
      splitDraftLines.value.push({
        key: `d${splitDraftSeq}`,
        itemIndex,
        orderItemId: parentId,
        skuName: line.skuName,
        qty: Math.max(1, line.qty || 1),
      })
    }
    // 过期父行 id 写回计划并重新同步订单中心子行，避免编辑空白、列表重复显示原商品
    if (needsPersistHeal && splitDraftLines.value.length && splitEditMode.value === 'partial') {
      try {
        await shippingApi.putShipPlan(
          order.id,
          splitDraftLines.value.map((l, i) => ({
            orderItemId: l.orderItemId,
            skuName: l.skuName.trim(),
            qty: l.qty,
            sortNo: i + 1,
          })),
        )
        const { list: refreshed } = await shippingApi.getShipPlan(order.id)
        order.shipPlanLines = healShipPlanLines(order, refreshed || [])
        order.pendingPlanCount = (order.shipPlanLines || []).filter((l) => l.status === 'pending').length
      } catch {
        /* 展示草稿优先；持久化失败不阻断编辑 */
      }
    }
  } catch (e) {
    ElMessage.error((e as Error).message || '加载拆分计划失败')
    return
  }
  splitDialogVisible.value = true
}

async function saveSplitPlan() {
  const order = splitTargetOrder.value
  if (!order) return
  for (const line of splitDraftLines.value) {
    if (!line.skuName.trim()) {
      ElMessage.warning('请填写规格名称')
      return
    }
    if (line.qty <= 0) {
      ElMessage.warning('拆分行数量须大于 0')
      return
    }
    if (splitEditMode.value === 'partial' && !line.orderItemId) {
      ElMessage.warning('按商品拆分请为每行关联原商品')
      return
    }
  }
  loading.splitSave = true
  try {
    const full = splitEditMode.value === 'full'
    await shippingApi.putShipPlan(
      order.id,
      splitDraftLines.value.map((l, i) => ({
        orderItemId: full ? 0 : l.orderItemId,
        skuName: l.skuName.trim(),
        qty: l.qty,
        sortNo: i + 1,
      })),
    )
    ElMessage.success(
      splitDraftLines.value.length
        ? full
          ? '整单拆分已保存'
          : '拆分计划已保存'
        : '已取消拆分',
    )
    splitDialogVisible.value = false
    await loadOmsOrders()
  } catch (e) {
    ElMessage.error((e as Error).message || '保存拆分失败')
  } finally {
    loading.splitSave = false
  }
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
  await prepareShipDialog([target], preferredMode)
}

function onTemplateChange(templateId: string) {
  const tpl = filteredTemplates.value.find((t) => t.templateId === templateId)
  const inferred = inferExpressCompany(tpl?.templateName)
  if (inferred) kdzsExpressCompany.value = inferred
}

function openShipDialogOms(order: OMSOrder) {
  void prepareShipDialog([order])
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
  void prepareShipDialog([...selectedOrders.value])
}

function onCarrierChange(id: number | undefined) {
  const carrier = carrierAccounts.value.find((c) => c.id === id)
  if (carrier) shipForm.useMonthly = carrier.useMonthly
  if (isSFCarrier.value) {
    sfShipAction.value = 'standard'
  }
}

function onPrintModeChange() {
  if (printMode.value === 'sf' && isBatchShip.value) {
    ElMessage.warning('自建物流暂不支持批量，请单笔操作或改用快递助手')
    printMode.value = 'kdzs'
  }
  if (printMode.value === 'sf' && isSFCarrier.value) {
    sfShipAction.value = 'standard'
  }
}

async function openKdzsBatchPrint() {
  const order = shipTargets.value[0]
  if (!order) return
  if (!ensureShipItemsSelected()) return
  if (!selectedTemplateId.value) {
    ElMessage.warning('请选择快递模板')
    return
  }
  loading.ship = true
  try {
    const platform = orderPlatformCode(order)
    const tpl = selectedTemplate.value
    const handoffOrders: KdzsHandoffOrder[] = shipTargets.value.map((o) => {
      const snap = snapshotForShip(o)
      return {
        orderNo: o.orderNo || '',
        platformSysTid: o.platformSysTid || '',
        platformOrderId: o.platformOrderId || '',
        sysTid: o.platformSysTid || '',
        tid: o.platformOrderId || '',
        payTime: o.payTime || '',
        orderedAt: o.orderedAt || '',
        goods: (snap.goods || []).map((g) => {
          const name = (g.skuName || g.title || '').trim()
          return {
            title: name,
            skuName: name,
            outerId: g.outerId,
            num: g.num,
          }
        }),
      }
    })
    const timeRange = buildKdzsOrderTimeRange(shipTargets.value)
    const payload: KdzsHandoffPayload = {
      v: 1,
      createdAt: Date.now(),
      platform,
      templateName: tpl?.templateName || '',
      templateId: tpl?.templateId,
      orders: handoffOrders,
      orderTimeFrom: timeRange?.from,
      orderTimeTo: timeRange?.to,
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
      `已打开快递助手并上传打单任务。请确认右下角「OSMS 打单助手」出现订单后，人工选模板/打印；完成后回填运单号。`,
    )
    confirmKdzsVisible.value = true
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
      if (!isBatchShip.value && selectedPickRows.value.length) {
        for (const pick of selectedPickRows.value) {
          pick.expressNo = hit.expressNo
        }
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
  if (!ensureShipItemsSelected()) return

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

  // 自建物流 + 顺丰：标准寄件 → 完整下单页（按勾选行带入）
  if (isSFCarrier.value && sfShipAction.value === 'standard') {
    goSFOrder(order)
    closeShipDialog()
    return
  }

  if (!shipForm.carrierAccountId || !shipForm.shipperProfileId) {
    ElMessage.warning('请选择物流账号和寄件人')
    return
  }
  loading.ship = true
  try {
    const savedExpress = localStorage.getItem('shippingcore.sf.expressType')
    const expressType =
      savedExpress === '1' || savedExpress === '2' ? savedExpress : undefined

    const shipment = await shippingApi.createShipmentFromOrder({
      carrierAccountId: shipForm.carrierAccountId,
      shipperProfileId: shipForm.shipperProfileId,
      useMonthly: shipForm.useMonthly,
      expressType,
      orderId: order.id,
      sourceSystem: 'ordercore',
      order: snapshotForShip(order),
    })
    const waybill = await shippingApi.createShipmentWaybill(shipment.id)
    const carrier = carrierAccounts.value.find((c) => c.id === shipForm.carrierAccountId)

    closeShipDialog()
    await openCloudPrintDialog({
      shipmentId: waybill.id,
      mailNo: waybill.mailNo || '',
      carrier,
    })
      await loadOmsOrders()
  } catch (e) {
    ElMessage.error((e as Error).message || '打单失败')
  } finally {
    loading.ship = false
  }
}

const primaryShipLabel = computed(() => {
  if (printMode.value === 'kdzs') return '打开快递助手'
  if (isSFCarrier.value && sfShipAction.value === 'standard') return '前往标准寄件'
  return '快速下单打印'
})

async function submitKdzsConfirm() {
  if (loading.ship) return
  if (!ensureShipItemsSelected()) return
  if (!kdzsExpressCompany.value.trim()) {
    ElMessage.warning('请选择快递公司')
    return
  }

  // 单笔多选：按勾选行填运单号（同号合并为一票）
  if (!isBatchShip.value && needPerRowExpress.value) {
    const picks = selectedPickRows.value
    const missing = picks.filter((r) => !(r.expressNo || '').trim())
    if (missing.length) {
      ElMessage.warning('请为每条勾选行填写运单号')
    return
  }
    const order = shipTargets.value[0]
    if (!order) return
  loading.ship = true
  try {
      const lines = picks.map((r) => {
        const spec = r.skuName.trim()
        return {
          orderItemId: r.orderItemId || 0,
          planLineId: r.planLineId || 0,
          qty: Math.min(Math.max(1, r.shipQty || 1), Math.max(1, r.maxQty || 1)),
          expressNo: (r.expressNo || '').trim(),
          title: spec,
          skuName: spec,
          outerId: '',
        }
      })
      await shippingApi.confirmKdzsSplitShip({
        orderId: order.id,
        expressCompany: kdzsExpressCompany.value.trim(),
        order: omsOrderToSnapshot(order),
        lines,
      })
      ElMessage.success(`已确认发货 ${lines.length} 行`)
    confirmKdzsVisible.value = false
    closeShipDialog()
      selectedOrders.value = []
    await loadOmsOrders()
  } catch (e) {
    ElMessage.error((e as Error).message || '确认发货失败')
  } finally {
    loading.ship = false
  }
    return
  }

  const rows = kdzsExpressRows.value
  if (!rows.length) return
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
        order: snapshotForShip(row.order),
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

function closeShipDialog() {
  shipDialogVisible.value = false
  shipTargets.value = []
  confirmKdzsVisible.value = false
  selectedTemplateId.value = ''
  shipPickRows.value = []
  selectedShipKeys.value = []
  pendingShipPlanLines.value = []
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
        <el-table-column label="订单号" min-width="180">
          <template #default="{ row }">
            <div class="order-no-cell">
              <span>{{ row.orderNo || '-' }}</span>
              <el-tag v-if="row.shipStatus === 'partial_shipped'" size="small" type="warning">部分发货</el-tag>
              <el-tag v-if="row.pendingPlanCount" size="small" type="success">
                已拆 {{ row.pendingPlanCount }} 段
              </el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="订单类型" width="100">
          <template #default="{ row }">{{ formatOrderSource(row) }}</template>
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
        <el-table-column label="商品信息" min-width="280">
          <template #default="{ row }">
            <div v-if="goodsRowsWithShipMark(row).length" class="goods-list">
              <div
                v-for="entry in goodsRowsWithShipMark(row)"
                :key="entry.key"
                class="goods-cell"
                :class="{ 'is-shipped': entry.fullyShipped }"
              >
                <img v-if="entry.picUrl" :src="entry.picUrl" class="goods-thumb" alt="" />
                <div class="goods-text">
                  <span>{{ entry.title || '-' }}</span>
                  <el-tag v-if="entry.isSplit && !entry.fullyShipped" size="small" type="success" class="ship-tag">
                    拆分
                  </el-tag>
                  <el-tag
                    v-if="entry.shipped > 0"
                    size="small"
                    :type="entry.fullyShipped ? 'info' : 'warning'"
                    class="ship-tag"
                  >
                    {{
                      entry.fullyShipped
                        ? '已发货'
                        : `已发 ${entry.shipped}/${entry.total || 0}`
                    }}
                  </el-tag>
                </div>
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
        <el-table-column label="付款时间" width="170">
          <template #default="{ row }">{{ formatPayTime(row.payTime || row.orderedAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
            <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openSplitDialog(row)">
              {{ row.pendingPlanCount ? '编辑拆分' : '拆分' }}
            </el-button>
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

        <div v-if="!isBatchShip && shipPickRows.length" class="ship-items-block">
          <div class="ship-items-hd">
            <span class="ship-items-title">发货商品</span>
            <el-checkbox
              :model-value="shipPickAllSelected"
              :indeterminate="shipPickIndeterminate"
              @change="(v: boolean | string | number) => toggleShipPickAll(!!v)"
            >
              全选
            </el-checkbox>
            <span class="muted">
              已选 {{ selectedShipKeys.length }}/{{ shipPickRows.length }}
            </span>
          </div>
          <el-checkbox-group v-model="selectedShipKeys" class="ship-items-list">
            <label v-for="row in shipPickRows" :key="row.key" class="ship-item-row">
              <el-checkbox :value="row.key" />
              <img v-if="row.picUrl" :src="row.picUrl" class="goods-thumb" alt="" />
              <div class="ship-item-text">
                <div>
                  {{ row.label || row.skuName || '-' }}
                  <el-tag v-if="row.kind === 'plan'" size="small" type="success" class="ml8">拆分</el-tag>
            </div>
                <div class="ship-item-qty" @click.stop>
                  <span class="muted">本次</span>
                  <el-input-number
                    v-model="row.shipQty"
                    :min="1"
                    :max="row.maxQty"
                size="small"
                    controls-position="right"
                  />
                  <span class="muted">待发 ×{{ row.maxQty }}</span>
        </div>
              </div>
            </label>
          </el-checkbox-group>
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
                ? `将打开快递助手并上传打单任务到云端；安装插件（extensions/kdzs-print-helper 电脑版）后自动选单/选模板「${selectedTemplate.templateName}」。请人工点打印，再回填运单号。`
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

            <el-form-item v-if="isSFCarrier" label="寄件方式">
              <el-radio-group v-model="sfShipAction" class="print-mode-radios">
                <el-radio value="standard">顺丰标准寄件</el-radio>
                <el-radio value="quick">快速下单打印</el-radio>
              </el-radio-group>
            </el-form-item>

            <el-alert
              v-if="isSFCarrier && sfShipAction === 'standard'"
              type="info"
              :closable="false"
              title="将进入顺丰标准寄件页，可完善托寄物、预约上门、运单备注后下单打印。"
            />
            <el-alert
              v-else-if="isSFCarrier && sfShipAction === 'quick'"
              type="info"
              :closable="false"
              title="将快速取号，随后选择面单模板与本机打印机完成打印。"
            />
          <el-alert
            v-else
            type="info"
            :closable="false"
              title="将按所选物流账号取号并进入打印。"
          />
          </template>
        </el-form>

      </template>

      <template #footer>
        <el-button @click="closeShipDialog">取消</el-button>
        <el-button
          v-if="printMode === 'kdzs' && shipTargets.length"
          @click="confirmKdzsVisible = true"
        >
          确认已打单发货
        </el-button>
        <el-button
          type="primary"
          :loading="loading.ship"
          :disabled="printMode === 'kdzs' && !selectedTemplateId"
          @click="submitShip"
        >
          {{ primaryShipLabel }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="printDialogVisible"
      title="打印快递单"
      width="480px"
      append-to-body
      :close-on-click-modal="false"
    >
      <div v-if="pendingPrint" class="print-mail muted">
        运单号：{{ pendingPrint.mailNo || '-' }}
      </div>
      <el-form label-width="100px">
        <el-form-item label="面单模板" required>
          <el-select
            v-model="selectedTemplateKey"
            placeholder="选择面单模板"
            style="width: 100%"
            :disabled="!templateOptions.length"
          >
            <el-option
              v-for="t in templateOptions"
              :key="t.key"
              :label="t.label"
              :value="t.key"
            />
          </el-select>
          <div v-if="!templateOptions.length" class="warn-tip">
            物流账号未配置模板编码，请先在物流账号中填写
          </div>
        </el-form-item>
        <el-form-item label="打印机" required>
          <el-select
            v-model="selectedPrinterIndex"
            placeholder="选择本机打印机"
            style="width: 100%"
            :loading="printersLoading"
            filterable
          >
            <el-option
              v-for="p in printers"
              :key="p.index"
              :label="p.name"
              :value="p.index"
            />
          </el-select>
          <div class="print-actions">
            <el-button link type="primary" :loading="printersLoading" @click="loadPrintersForDialog">
              刷新打印机
            </el-button>
            <el-button link type="primary" @click="router.push('/clodop')">C-Lodop 服务</el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="printDialogVisible = false">取消</el-button>
        <el-button
          type="primary"
          :loading="printDialogLoading"
          :disabled="selectedPrinterIndex == null || (!!templateOptions.length && !selectedTemplateKey)"
          @click="confirmCloudPrint"
        >
          打印快递单
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="confirmKdzsVisible"
      title="确认已打单发货"
      :width="needPerRowExpress ? '640px' : isBatchShip ? '560px' : '440px'"
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
        <template v-if="!isBatchShip && needPerRowExpress">
          <div class="batch-confirm-hint muted">勾选了多行：请为每行填写运单号（同号合并为一票）</div>
          <div v-for="row in selectedPickRows" :key="row.key" class="batch-row split-confirm-row">
            <div class="batch-order">
              {{ row.label || row.skuName }}
              <span class="muted">×{{ row.shipQty }}</span>
            </div>
            <el-input v-model="row.expressNo" placeholder="运单号" />
          </div>
        </template>
        <template v-else-if="!isBatchShip">
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

    <el-dialog
      v-model="splitDialogVisible"
      :title="`拆分发货 — ${splitTargetOrder?.orderNo || ''}`"
      width="720px"
      append-to-body
    >
      <div v-if="splitTargetOrder" class="split-edit">
        <div class="split-edit-mode">
          <el-radio-group
            :model-value="splitEditMode"
            @change="(v: string | number | boolean | undefined) => onSplitEditModeChange(v === 'full' ? 'full' : 'partial')"
          >
            <el-radio-button value="partial">按商品拆分</el-radio-button>
            <el-radio-button value="full">整单拆分</el-radio-button>
          </el-radio-group>
  </div>
        <div class="muted" style="margin-bottom: 10px">
          <template v-if="splitEditMode === 'full'">
            整单拆分：只填规格名称，不对应原商品；保存后打单发货全部按这些规格行勾选。
          </template>
          <template v-else>
            对需要拆分的商品点「加拆分」并填写规格名称；未拆分的商品在打单时仍按原行勾选。
          </template>
        </div>

        <template v-if="splitEditMode === 'full'">
          <div class="split-lines-hd" style="margin-bottom: 8px">
            <span class="split-lines-title">拆分规格</span>
            <el-button type="primary" link size="small" @click="addFullSplitDraftLine">加拆分</el-button>
          </div>
          <div v-for="line in splitDraftLines" :key="line.key" class="split-line-row">
            <el-input
              v-model="line.skuName"
              size="small"
              class="split-line-title-input"
              placeholder="规格名称"
              clearable
            />
            <el-input-number
              v-model="line.qty"
              :min="1"
              size="small"
              controls-position="right"
            />
            <el-button link type="danger" size="small" @click="removeSplitDraftLine(line.key)">
              删
            </el-button>
          </div>
          <div v-if="!splitDraftLines.length" class="muted" style="font-size: 12px">
            点击「加拆分」添加规格明细
          </div>
        </template>

        <template v-else>
          <div
            v-for="(item, index) in rootOMSItems(splitTargetOrder)"
            :key="item.id || index"
            class="split-edit-item"
          >
            <div class="split-edit-item-hd">
              <img v-if="item.picUrl" :src="item.picUrl" class="goods-thumb" alt="" />
              <div class="ship-item-text">
                <div>{{ formatGoodsLine(item) || item.productName || '-' }}</div>
              </div>
              <el-button
                v-if="item.id"
                link
                type="primary"
                size="small"
                @click="addSplitDraftLine(index, item.id!)"
              >
                加拆分
              </el-button>
              <el-button
                v-if="splitDraftLinesForItem(index).length"
                link
                type="danger"
                size="small"
                @click="clearSplitDraftForItem(index)"
              >
                取消拆分
              </el-button>
            </div>
            <div
              v-for="line in splitDraftLinesForItem(index)"
              :key="line.key"
              class="split-line-row"
            >
              <el-input
                v-model="line.skuName"
                size="small"
                class="split-line-title-input"
                placeholder="规格名称"
                clearable
              />
              <el-input-number
                v-model="line.qty"
                :min="1"
                size="small"
                controls-position="right"
              />
              <el-button link type="danger" size="small" @click="removeSplitDraftLine(line.key)">
                删
              </el-button>
            </div>
          </div>
        </template>
      </div>
      <template #footer>
        <el-button @click="splitDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="loading.splitSave" @click="saveSplitPlan">保存拆分</el-button>
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
.order-no-cell { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.pager { margin-top: 16px; display: flex; justify-content: flex-end; }
.ship-order-info { margin-bottom: 16px; display: flex; align-items: center; flex-wrap: wrap; gap: 4px; }
.ml8 { margin-left: 8px; }
.ship-items-block {
  margin-bottom: 14px;
  padding: 10px 12px;
  background: #fafafa;
  border: 1px solid #ebeef5;
  border-radius: 6px;
}
.ship-items-hd {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}
.ship-items-title {
  font-size: 13px;
  font-weight: 600;
  color: #303133;
}
.ship-items-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 220px;
  overflow: auto;
}
.ship-item-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 4px;
  cursor: pointer;
  background: #fff;
  border: 1px solid #eef0f3;
}
.split-lines {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px dashed #e4e7ed;
}
.split-lines-title {
  font-size: 12px;
  color: #606266;
}
.split-lines-hd {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}
.split-line-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
  flex-wrap: wrap;
}
.split-line-product {
  width: 140px;
  flex-shrink: 0;
}
.split-line-title-input {
  flex: 1;
  min-width: 140px;
}
.split-line-sku-input {
  width: 120px;
  flex-shrink: 0;
}
.split-line-name {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.split-confirm-row .batch-order {
  min-width: 160px;
  max-width: 240px;
}
.ship-item-row:hover { border-color: #d0d5dd; }
.ship-item-text {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  line-height: 1.4;
  color: #303133;
}
.ship-item-qty {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
  flex-wrap: wrap;
}
.ship-item-qty .el-input-number {
  width: 110px;
}
.ship-form { margin-top: 8px; }
.print-mode-radios {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
}
.tpl-bar { width: 100%; }
.tpl-radios { display: flex; flex-wrap: wrap; gap: 8px; }
.tpl-radio { margin-right: 0 !important; }
.batch-confirm-hint { margin-bottom: 12px; }
.batch-row { display: grid; grid-template-columns: 160px 1fr; gap: 8px; align-items: center; margin-bottom: 8px; }
.batch-order { font-size: 13px; color: #606266; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.goods-list { display: flex; flex-direction: column; gap: 8px; }
.goods-cell { display: flex; gap: 8px; align-items: flex-start; }
.goods-cell.is-shipped { opacity: 0.55; }
.goods-cell.is-shipped .goods-text > span { text-decoration: line-through; }
.goods-thumb { width: 40px; height: 40px; object-fit: cover; border-radius: 4px; flex-shrink: 0; }
.goods-text {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  line-height: 1.4;
  white-space: normal;
  word-break: break-all;
}
.ship-tag { flex-shrink: 0; }
.print-mail { margin-bottom: 12px; }
.print-actions { margin-top: 4px; display: flex; gap: 8px; }
.split-edit-item {
  margin-bottom: 14px;
  padding: 10px;
  background: #fafafa;
  border: 1px solid #ebeef5;
  border-radius: 6px;
}
.split-edit-item-hd {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-bottom: 8px;
}
.split-edit-mode {
  margin-bottom: 12px;
}
</style>
