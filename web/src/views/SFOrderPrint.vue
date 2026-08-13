<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox, type UploadRequestOptions } from 'element-plus'
import { Box, Camera, InfoFilled, Minus, Plus, Printer, RefreshRight, Van, Warning } from '@element-plus/icons-vue'
import {
  shippingApi,
  type CarrierAccount,
  type OrderSnapshot,
  type ShipperProfile,
} from '../api/shipping'
import { uploadImage } from '../api/upload'
import {
  consumeSFOrderHandoff,
  goodsCargoName,
  parsePastedContact,
  type SFOrderHandoff,
} from '../utils/sfOrderHandoff'
import { printShipmentByChannel } from '../utils/sfPrintLabel'
import {
  getSavedPrinterIndex,
  getSavedPrinterName,
} from '../utils/sfPrintPlugin'
import { forbidItemsCatalog } from '../constants/forbidItems'
import { nextSCManualOrderNo } from '../utils/manualOrderNo'

const cargoPresets = ['文件', '电子产品', '日用品', '服装', '食品', '配件', '商品']

const router = useRouter()

const EXPRESS_TYPE_KEY = 'shippingcore.sf.expressType'
/** 重量/体积/尺寸：按合计填写 | 按单件填写（对齐企服寄件页） */
type FillMode = 'total' | 'unit'

type ExpressProductCard = {
  value: string
  name: string
  tag?: string
  hint?: string
  fee?: number
  deliverLabel?: string
}

const DEFAULT_EXPRESS_PRODUCTS: ExpressProductCard[] = [
  { value: '1', name: '顺丰特快', tag: '时效最优', hint: '时效更快，适合急件' },
  { value: '2', name: '顺丰标快', tag: '经济实惠', hint: '常规时效，性价比高' },
]

const expressProducts = ref<ExpressProductCard[]>([...DEFAULT_EXPRESS_PRODUCTS])
const quoteLoading = ref(false)
const quoteError = ref('')

function loadSavedExpressType(): string {
  const v = localStorage.getItem(EXPRESS_TYPE_KEY)
  if (v === '1' || v === '2') return v
  return '2'
}

function fmtFee(fee?: number) {
  if (fee == null || !(fee > 0)) return ''
  return Number.isInteger(fee) ? String(fee) : fee.toFixed(2)
}

type PayMode = 'monthly' | 'cash' | 'receiver'

const loading = ref(false)
const submitting = ref(false)
const cancelling = ref(false)
const carriers = ref<CarrierAccount[]>([])
const shippers = ref<ShipperProfile[]>([])
const handoffMeta = ref<Pick<SFOrderHandoff, 'orderId' | 'sourceSystem'> | null>(null)
const result = ref<{ shipmentId: number; mailNo: string; cancelled?: boolean } | null>(null)

/** 物品信息多行（对齐企服：可增减行） */
type CargoLine = {
  name: string
  parcelQty: number
  /** 按合计=该行总重；按单件=单个包裹重量 */
  weight: number
  /** m³：按合计=该行总体积；按单件=单个包裹体积 */
  volume?: number
  lengthCm?: number
  widthCm?: number
  heightCm?: number
  /** 按合计=该行总件数；按单件=单个包裹物品数 */
  itemCount: number
  /** 订单中心销售行 ID，按商品发货同步必备 */
  orderItemId?: number
  title?: string
  outerId?: string
  price?: number
}

function emptyCargoLine(name = ''): CargoLine {
  return {
    name,
    parcelQty: 1,
    weight: 1,
    volume: undefined,
    lengthCm: undefined,
    widthCm: undefined,
    heightCm: undefined,
    itemCount: 1,
    orderItemId: 0,
    title: '',
    outerId: '',
    price: 0,
  }
}

function lineDimsVolumeM3(line: CargoLine): number {
  const l = line.lengthCm || 0
  const w = line.widthCm || 0
  const h = line.heightCm || 0
  if (l <= 0 || w <= 0 || h <= 0) return 0
  return (l * w * h) / 1_000_000
}

function lineVolumeM3(line: CargoLine): number {
  if (line.volume && line.volume > 0) return line.volume
  return lineDimsVolumeM3(line)
}

const form = reactive({
  carrierAccountId: undefined as number | undefined,
  shipperProfileId: undefined as number | undefined,
  payMode: 'monthly' as PayMode,
  expressType: loadSavedExpressType(),
  fillMode: 'unit' as FillMode,
  cargoLines: [emptyCargoLine('文件')] as CargoLine[],
  pickupMode: 'self' as 'self' | 'appoint',
  /** 级联值：[dayOffset, slotKey]，如 [0, '09:00'] */
  appointSlot: [] as (string | number)[],
  remark: '',
  remarkImages: [] as string[],
  receiverName: '',
  receiverMobile: '',
  receiverProvince: '',
  receiverCity: '',
  receiverCounty: '',
  receiverAddress: '',
  receiverCompany: '',
  pasteText: '',
  platform: '',
  shopId: '',
  orderNo: '',
  sysTid: '',
  sourceTid: '',
})

const itemLibVisible = ref(false)
const forbidDialogVisible = ref(false)

function addCargoLine() {
  form.cargoLines.push(emptyCargoLine())
}

function removeCargoLine(idx: number) {
  if (form.cargoLines.length <= 1) {
    form.cargoLines[0] = emptyCargoLine()
    return
  }
  form.cargoLines.splice(idx, 1)
}

function onLineDimsChange(line: CargoLine) {
  const v = lineDimsVolumeM3(line)
  if (v > 0) line.volume = Math.round(v * 1_000_000) / 1_000_000
}

function pickFromItemLib(name: string) {
  const empty = form.cargoLines.find((l) => !(l.name || '').trim())
  if (empty) empty.name = name
  else form.cargoLines.push(emptyCargoLine(name))
  itemLibVisible.value = false
}

/**
 * 切换合计/单件时换算各行数值，保证运单合计不变：
 * - 按单件：输入=单个包裹的重量/件数/体积，表头合计 = Σ(值 × 包裹数)
 * - 按合计：输入=该物品类型全部包裹的总重量/件数/体积，表头合计 = Σ(值)
 */
function setFillMode(mode: FillMode) {
  if (mode === form.fillMode) return
  const from = form.fillMode
  for (const line of form.cargoLines) {
    const pq = line.parcelQty > 0 ? line.parcelQty : 1
    if (from === 'unit' && mode === 'total') {
      // 单件 → 合计：乘以包裹数
      line.weight = Math.round(line.weight * pq * 1000) / 1000
      if (line.volume && line.volume > 0) {
        line.volume = Math.round(line.volume * pq * 1_000_000) / 1_000_000
      }
      line.itemCount = Math.max(1, Math.round(line.itemCount * pq))
    } else if (from === 'total' && mode === 'unit') {
      // 合计 → 单件：除以包裹数
      line.weight = Math.round((line.weight / pq) * 1000) / 1000
      if (line.volume && line.volume > 0) {
        line.volume = Math.round((line.volume / pq) * 1_000_000) / 1_000_000
      }
      line.itemCount = Math.max(1, Math.round(line.itemCount / pq) || 1)
    }
  }
  form.fillMode = mode
}

const uploadingImg = ref(false)

const namedCargoLines = computed(() => form.cargoLines.filter((l) => (l.name || '').trim()))

/** 各行贡献到运单合计（两种模式都支持多行） */
function lineContribWeight(line: CargoLine): number {
  const w = line.weight > 0 ? line.weight : 0
  const pq = line.parcelQty > 0 ? line.parcelQty : 1
  // 按单件：单个包裹重量 × 包裹数；按合计：已是该行总重，不再乘
  return form.fillMode === 'unit' ? w * pq : w
}

function lineContribVolume(line: CargoLine): number {
  const v = lineVolumeM3(line)
  if (!(v > 0)) return 0
  const pq = line.parcelQty > 0 ? line.parcelQty : 1
  return form.fillMode === 'unit' ? v * pq : v
}

function lineContribCount(line: CargoLine): number {
  const c = line.itemCount > 0 ? line.itemCount : 0
  const pq = line.parcelQty > 0 ? line.parcelQty : 1
  return form.fillMode === 'unit' ? c * pq : c
}

/** 表头合计：有物品名的行参与汇总（空行不计入，避免点「+」就抬高合计） */
const cargoTotals = computed(() => {
  const lines = namedCargoLines.value.length ? namedCargoLines.value : form.cargoLines
  let parcelQty = 0
  let weight = 0
  let volume = 0
  let itemCount = 0
  for (const line of lines) {
    parcelQty += line.parcelQty > 0 ? line.parcelQty : 0
    weight += lineContribWeight(line)
    volume += lineContribVolume(line)
    itemCount += lineContribCount(line)
  }
  return {
    parcelQty: parcelQty || 1,
    weight: Math.round(weight * 1000) / 1000,
    volume: Math.round(volume * 1_000_000) / 1_000_000,
    itemCount: itemCount || 0,
  }
})

function fmtTotalNum(n: number, digits = 2): string {
  if (!(n > 0)) return '0'
  const fixed = n.toFixed(digits)
  return fixed.replace(/\.?0+$/, '')
}

const weightLabel = computed(() =>
  form.fillMode === 'unit' ? '单个包裹重量' : '总包裹重量',
)
const volumeLabel = computed(() =>
  form.fillMode === 'unit' ? '单个包裹体积' : '总包裹体积',
)
const sizeLabel = computed(() =>
  form.fillMode === 'unit' ? '单个包裹尺寸' : '总包裹尺寸',
)
const countLabel = computed(() =>
  form.fillMode === 'unit' ? '单个包裹物品数' : '总包裹物品数',
)

const fillModeTips = {
  total: '每种物品类型的包裹总重量/件数/体积',
  unit: '按单个包裹填写重量件数体积',
} as const

const shipperView = computed(() => shippers.value.find((s) => s.id === form.shipperProfileId) || null)
const carrierView = computed(() => carriers.value.find((c) => c.id === form.carrierAccountId) || null)
const printerName = computed(() => getSavedPrinterName() || (getSavedPrinterIndex() != null ? `索引 ${getSavedPrinterIndex()}` : ''))
const regionText = computed(() =>
  [form.receiverProvince, form.receiverCity, form.receiverCounty].filter(Boolean).join('-'),
)

function applyShipper(id?: number) {
  form.shipperProfileId = id
}

function applyHandoff(h: SFOrderHandoff) {
  handoffMeta.value = { orderId: h.orderId, sourceSystem: h.sourceSystem }
  const o = h.order
  form.platform = o.platform
  form.shopId = o.shopId
  form.orderNo = o.orderNo || ''
  form.sysTid = o.sysTid
  form.sourceTid = o.sourceTid
  form.receiverName = o.receiverName
  form.receiverMobile = o.receiverMobile
  form.receiverProvince = o.receiverProvince
  form.receiverCity = o.receiverCity
  form.receiverCounty = o.receiverCounty
  form.receiverAddress = o.receiverAddress
  const lines = (o.goods || [])
    .map((g) => {
      const name = (g.skuName || g.title || '').trim()
      if (!name) return null
      const line = emptyCargoLine(name)
      line.orderItemId = g.orderItemId || 0
      line.title = g.title || ''
      line.itemCount = g.num > 0 ? g.num : 1
      line.parcelQty = 1
      line.outerId = g.outerId || ''
      line.price = g.price || 0
      return line
    })
    .filter((x): x is CargoLine => !!x)
  form.cargoLines = lines.length ? lines : [emptyCargoLine('文件')]
  form.fillMode = 'unit'
}

async function uploadRemarkImage(options: UploadRequestOptions) {
  uploadingImg.value = true
  try {
    const file = options.file as File
    if (!file.type.startsWith('image/')) {
      throw new Error('请上传图片文件')
    }
    if (form.remarkImages.length >= 6) {
      throw new Error('最多上传 6 张图片')
    }
    const url = await uploadImage(file, 'shipment-remark')
    form.remarkImages.push(url)
    options.onSuccess?.(url as never)
  } catch (e) {
    ElMessage.error((e as Error).message || '上传失败')
    options.onError?.(e as never)
  } finally {
    uploadingImg.value = false
  }
}

function removeRemarkImage(idx: number) {
  form.remarkImages.splice(idx, 1)
}

async function loadOptions() {
  loading.value = true
  try {
    const [cRes, sRes] = await Promise.all([
      shippingApi.listCarrierAccounts({ page: 1, pageSize: 100, enabled: true }),
      shippingApi.listShipperProfiles({ page: 1, pageSize: 100, enabled: true }),
    ])
    carriers.value = (cRes.list || []).filter((c) => c.enabled !== false)
    shippers.value = (sRes.list || []).filter((s) => s.enabled !== false)
    const defaultCarrier = carriers.value[0]
    const defaultShipper = shippers.value.find((s) => s.isDefault) || shippers.value[0]
    form.carrierAccountId = defaultCarrier?.id
    form.shipperProfileId = defaultShipper?.id
    if (defaultCarrier) {
      form.payMode = defaultCarrier.useMonthly ? 'monthly' : 'cash'
    }
    // 快件类型仅在本页选择，不跟账号绑定
    form.expressType = loadSavedExpressType()
  } catch (e) {
    ElMessage.error((e as Error).message || '加载账号失败')
  } finally {
    loading.value = false
  }
}

watch(
  () => form.carrierAccountId,
  (id) => {
    const c = carriers.value.find((x) => x.id === id)
    if (!c) return
    if (c.useMonthly && c.custId) form.payMode = 'monthly'
    else if (form.payMode === 'monthly') form.payMode = 'cash'
  },
)

watch(
  () => form.expressType,
  (v) => {
    if (expressProducts.value.some((p) => p.value === v)) {
      localStorage.setItem(EXPRESS_TYPE_KEY, v)
    }
  },
)

const selectedProduct = computed(
  () => expressProducts.value.find((p) => p.value === form.expressType) || expressProducts.value[1],
)

let quoteTimer: ReturnType<typeof setTimeout> | null = null
function scheduleRefreshDeliverQuote() {
  if (quoteTimer) clearTimeout(quoteTimer)
  quoteTimer = setTimeout(() => {
    void refreshDeliverQuote()
  }, 400)
}

async function refreshDeliverQuote() {
  const carrierId = form.carrierAccountId
  const shipper = shipperView.value
  if (
    !carrierId ||
    !shipper ||
    !form.receiverProvince ||
    !form.receiverCity ||
    !form.receiverAddress
  ) {
    expressProducts.value = [...DEFAULT_EXPRESS_PRODUCTS]
    quoteError.value = ''
    return
  }
  quoteLoading.value = true
  quoteError.value = ''
  try {
    const res = await shippingApi.queryDeliverTm({
      carrierAccountId: carrierId,
      srcProvince: shipper.province,
      srcCity: shipper.city,
      srcCounty: shipper.county,
      srcAddress: shipper.address,
      destProvince: form.receiverProvince,
      destCity: form.receiverCity,
      destCounty: form.receiverCounty,
      destAddress: form.receiverAddress,
      weightKg: cargoTotals.value.weight > 0 ? cargoTotals.value.weight : 1,
      useMonthly: form.payMode === 'monthly',
      consignedTime: resolveSendStartTm(),
      businessType: form.expressType,
    })
    const list = (res.products || []).map((p) => ({
      value: String(p.value),
      name: p.name,
      tag: p.tag,
      hint: p.hint,
      fee: p.fee,
      deliverLabel: p.deliverLabel,
    }))
    expressProducts.value = list.length ? list : [...DEFAULT_EXPRESS_PRODUCTS]
    if (!expressProducts.value.some((p) => p.value === form.expressType)) {
      form.expressType = expressProducts.value[0]?.value || '2'
    }
  } catch (e: unknown) {
    expressProducts.value = [...DEFAULT_EXPRESS_PRODUCTS]
    const msg = e instanceof Error ? e.message : String(e || '')
    quoteError.value = msg.includes('无对应服务权限') || msg.includes('A1004')
      ? '请在丰桥开通「时效标准及价格查询」(EXP_RECE_QUERY_DELIVERTM) 后重试'
      : msg || '时效/运费查询失败'
  } finally {
    quoteLoading.value = false
  }
}

watch(
  () =>
    [
      form.carrierAccountId,
      form.shipperProfileId,
      form.receiverProvince,
      form.receiverCity,
      form.receiverCounty,
      form.receiverAddress,
      form.payMode,
      form.expressType,
      form.pickupMode,
      form.appointSlot.join('|'),
      cargoTotals.value.weight,
    ] as const,
  () => scheduleRefreshDeliverQuote(),
)

const payModeOptions = computed(() => {
  const monthlyLabel = carrierView.value?.custId
    ? `寄付月结 / ${carrierView.value.custId}`
    : '寄付月结'
  return [
    { value: 'monthly' as PayMode, label: monthlyLabel, disabled: !carrierView.value?.custId },
    { value: 'cash' as PayMode, label: '寄付现结', disabled: false },
    { value: 'receiver' as PayMode, label: '收方付', disabled: false },
  ]
})

function resolvePayMethod(): number {
  return form.payMode === 'receiver' ? 2 : 1
}

const APPOINT_FALLBACK_HOURS = [8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19] as const

function pad2(n: number) {
  return String(n).padStart(2, '0')
}

function formatLocalDateTime(d: Date): string {
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`
}

type AppointCascaderNode = {
  value: number | string
  label: string
  sendStartTm?: string
  children?: AppointCascaderNode[]
}

const pickupWindow = ref<{ startTm: string; endTm: string } | null>(null)
const appointLoading = ref(false)
const appointCascaderOptions = ref<AppointCascaderNode[]>([])

function buildLocalAppointOptions(now = new Date()): AppointCascaderNode[] {
  const startMin = 8 * 60
  const endMin = 20 * 60
  const days = [
    { value: 0, label: '今天' },
    { value: 1, label: '明天' },
    { value: 2, label: '后天' },
  ]
  return days
    .map((day) => {
      const children: AppointCascaderNode[] = []
      if (day.value === 0) {
        const within = new Date(now.getTime() + 15 * 60 * 1000)
        const wm = within.getHours() * 60 + within.getMinutes()
        if (wm >= startMin && wm < endMin) {
          children.push({ value: 'within1h', label: '1小时内', sendStartTm: formatLocalDateTime(within) })
        }
      }
      let first = startMin
      if (day.value === 0) {
        const next =
          now.getMinutes() > 0 || now.getSeconds() > 0
            ? (now.getHours() + 1) * 60
            : now.getHours() * 60
        if (next > first) first = next
      }
      for (let slotStart = first; slotStart < endMin; slotStart += 60) {
        const h = Math.floor(slotStart / 60)
        if (!APPOINT_FALLBACK_HOURS.includes(h as (typeof APPOINT_FALLBACK_HOURS)[number]) && h < 8) continue
        children.push({
          value: `${pad2(h)}:00`,
          label: `${pad2(h)}:00-${pad2(h + 1)}:00`,
          sendStartTm: formatLocalDateTime(
            new Date(now.getFullYear(), now.getMonth(), now.getDate() + day.value, h, 0, 0),
          ),
        })
      }
      return { value: day.value, label: day.label, children }
    })
    .filter((d) => (d.children?.length || 0) > 0)
}

function mapApiAppointOptions(
  apiDays: Array<{
    value: number
    text: string
    children: Array<{ value: string; text: string; slotKey: string; sendStartTm: string }>
  }>,
): AppointCascaderNode[] {
  return (apiDays || [])
    .map((d) => ({
      value: d.value,
      label: d.text,
      children: (d.children || []).map((c) => ({
        value: c.slotKey,
        label: c.text,
        sendStartTm: c.sendStartTm,
      })),
    }))
    .filter((d) => (d.children?.length || 0) > 0)
}

async function refreshPickupOptions() {
  const carrierId = form.carrierAccountId
  const shipper = shipperView.value
  if (!carrierId || !shipper) {
    appointCascaderOptions.value = buildLocalAppointOptions()
    pickupWindow.value = null
    return
  }
  appointLoading.value = true
  try {
    const res = await shippingApi.checkPickupTime({
      carrierAccountId: carrierId,
      province: shipper.province,
      city: shipper.city,
      county: shipper.county,
      address: shipper.address,
    })
    pickupWindow.value = { startTm: res.startTm, endTm: res.endTm }
    const mapped = mapApiAppointOptions(res.options || [])
    appointCascaderOptions.value = mapped.length ? mapped : buildLocalAppointOptions()
    if (form.appointSlot.length >= 2) {
      const [day, slot] = form.appointSlot
      const ok = appointCascaderOptions.value.some(
        (d) => d.value === Number(day) && d.children?.some((c) => c.value === slot),
      )
      if (!ok) {
        const firstDay = appointCascaderOptions.value[0]
        const firstSlot = firstDay?.children?.[0]
        form.appointSlot = firstDay && firstSlot ? [firstDay.value as number, firstSlot.value as string] : []
      }
    }
  } catch {
    appointCascaderOptions.value = buildLocalAppointOptions()
    pickupWindow.value = null
  } finally {
    appointLoading.value = false
  }
}

function resolveSendStartTm(): string | undefined {
  if (form.pickupMode !== 'appoint') return undefined
  const [dayRaw, slot] = form.appointSlot
  if (dayRaw === undefined || dayRaw === null || !slot) return undefined
  const fromOpt = appointCascaderOptions.value
    .find((d) => d.value === Number(dayRaw))
    ?.children?.find((c) => c.value === slot)
  if (fromOpt?.sendStartTm) return fromOpt.sendStartTm
  const dayOffset = Number(dayRaw)
  const now = new Date()
  if (slot === 'within1h') {
    return formatLocalDateTime(new Date(now.getTime() + 15 * 60 * 1000))
  }
  const m = String(slot).match(/^(\d{1,2}):(\d{2})$/)
  if (!m) return undefined
  const d = new Date(now.getFullYear(), now.getMonth(), now.getDate() + dayOffset, Number(m[1]), Number(m[2]), 0)
  return formatLocalDateTime(d)
}

function setPickupMode(mode: 'self' | 'appoint') {
  form.pickupMode = mode
  if (mode === 'self') {
    form.appointSlot = []
  } else {
    void refreshPickupOptions().then(() => {
      if (!form.appointSlot.length) {
        const firstDay = appointCascaderOptions.value[0]
        const firstSlot = firstDay?.children?.[0]
        if (firstDay && firstSlot) {
          form.appointSlot = [firstDay.value as number, firstSlot.value as string]
        }
      }
    })
  }
}

watch(
  () => [form.carrierAccountId, form.shipperProfileId] as const,
  () => {
    if (form.pickupMode === 'appoint') void refreshPickupOptions()
  },
)

function recognizeReceiver() {
  const parsed = parsePastedContact(form.pasteText)
  if (parsed.name) form.receiverName = parsed.name
  if (parsed.mobile) form.receiverMobile = parsed.mobile
  if (parsed.address) {
    // 简单：整段放入详细地址；省市区需人工确认
    form.receiverAddress = parsed.address
  }
  if (parsed.name || parsed.mobile || parsed.address) {
    ElMessage.success('已识别并填入，请核对省市区')
  } else {
    ElMessage.warning('未能识别，请手动填写')
  }
}

function buildOrderSnapshot(): OrderSnapshot {
  const orderNo = form.orderNo.trim()
  // 有订单中心单号时不要生成 SC-MANUAL 占位，保证与待发货 orderNo 一致
  const sysTid =
    form.sysTid.trim() || orderNo || form.sourceTid.trim() || nextSCManualOrderNo()
  const sourceTid = form.sourceTid.trim() || orderNo || sysTid
  const goods = namedCargoLines.value.map((l) => ({
    orderItemId: l.orderItemId || 0,
    title: l.title || '',
    skuName: l.name.trim(),
    num: lineContribCount(l) || 1,
    outerId: l.outerId || '',
    price: l.price || 0,
  }))
  return {
    platform: form.platform || 'manual',
    shopId: form.shopId || '',
    orderNo: orderNo || (sourceTid.toUpperCase().startsWith('OC') ? sourceTid : ''),
    sysTid,
    sourceTid,
    receiverName: form.receiverName.trim(),
    receiverMobile: form.receiverMobile.trim(),
    receiverProvince: form.receiverProvince.trim(),
    receiverCity: form.receiverCity.trim(),
    receiverCounty: form.receiverCounty.trim(),
    receiverAddress: form.receiverAddress.trim(),
    goods,
  }
}

function firstDimsLine(): CargoLine | undefined {
  return form.cargoLines.find(
    (l) => (l.lengthCm || 0) > 0 && (l.widthCm || 0) > 0 && (l.heightCm || 0) > 0,
  )
}

function validate(): string | null {
  if (!form.carrierAccountId) return '请选择物流账号'
  if (!form.shipperProfileId) return '请选择寄件人'
  if (!form.receiverName.trim() || !form.receiverMobile.trim()) return '请填写收件人姓名与手机'
  if (!form.receiverAddress.trim()) return '请填写收件详细地址'
  if (!namedCargoLines.value.length) return '请至少填写一行物品名称'
  // 从订单中心带入时，须保留销售行 ID，否则订单会按「空明细=全部发完」误标已发货
  if (handoffMeta.value?.orderId) {
    const linked = namedCargoLines.value.filter((l) => (l.orderItemId || 0) > 0)
    if (!linked.length) {
      return '订单商品行 ID 丢失，请关闭本页后从待发货重新勾选商品进入寄件'
    }
  }
  for (const [i, line] of namedCargoLines.value.entries()) {
    if (!(line.parcelQty > 0)) return `第 ${i + 1} 行请填写包裹数`
    if (!(line.weight > 0)) return `第 ${i + 1} 行请填写重量`
    if (!(line.itemCount > 0)) return `第 ${i + 1} 行请填写物品数`
  }
  if (!(cargoTotals.value.weight > 0)) return '请填写包裹重量'
  if (!(cargoTotals.value.itemCount > 0)) return '请填写物品件数'
  if (!form.expressType) return '请选择物流产品'
  if (form.payMode === 'monthly' && !carrierView.value?.custId) {
    return '当前物流账号未配置月结卡号，请改选寄付现结或收方付'
  }
  if (form.pickupMode === 'appoint' && !resolveSendStartTm()) {
    return '请选择预约上门时间'
  }
  return null
}

async function printShipmentLabel(shipmentId: number) {
  const channel = (carrierView.value?.printChannel || 'plugin').toLowerCase()
  let printerIndex: number | null = null
  if (channel !== 'pdf') {
    printerIndex = getSavedPrinterIndex()
    if (printerIndex == null) {
      ElMessage.warning('请先在 C-Lodop 云打印 选择本机打印机')
      await router.push('/clodop')
      throw new Error('PRINTER_NOT_SELECTED')
    }
  }
  await printShipmentByChannel({
    shipmentId,
    printChannel: channel,
    printerIndex,
  })
}

async function cancelWaybill() {
  if (!result.value?.shipmentId || result.value.cancelled) return
  const mailNo = result.value.mailNo || `#${result.value.shipmentId}`
  await ElMessageBox.confirm(
    `确认取消顺丰快递单 ${mailNo}？取消后运单将作废，请确认包裹尚未揽收。`,
    '取消快递单',
    { type: 'warning', confirmButtonText: '确认取消' },
  )
  cancelling.value = true
  try {
    await shippingApi.cancelShipment(result.value.shipmentId)
    result.value = { ...result.value, cancelled: true }
    ElMessage.success('已取消顺丰快递单')
  } catch (e) {
    ElMessage.error((e as Error).message || '取消失败')
  } finally {
    cancelling.value = false
  }
}

async function submit(doPrint: boolean) {
  const err = validate()
  if (err) {
    ElMessage.warning(err)
    return
  }
  submitting.value = true
  result.value = null
  try {
    const useMonthly = form.payMode === 'monthly'
    const order = buildOrderSnapshot()
    const shipment = await shippingApi.createShipmentFromOrder({
      carrierAccountId: form.carrierAccountId!,
      shipperProfileId: form.shipperProfileId!,
      useMonthly,
      expressType: form.expressType,
      payMethod: resolvePayMethod(),
      remark: form.remark.trim(),
      remarkImages: form.remarkImages.length ? [...form.remarkImages] : undefined,
      cargoName: goodsCargoName(order.goods) || namedCargoLines.value[0]?.name.trim() || '商品',
      parcelQty: cargoTotals.value.parcelQty,
      cargoCount: cargoTotals.value.itemCount || 1,
      totalWeight: cargoTotals.value.weight > 0 ? cargoTotals.value.weight : undefined,
      lengthCm: firstDimsLine()?.lengthCm || undefined,
      widthCm: firstDimsLine()?.widthCm || undefined,
      heightCm: firstDimsLine()?.heightCm || undefined,
      totalVolume: cargoTotals.value.volume > 0 ? cargoTotals.value.volume : undefined,
      pickupMode: form.pickupMode,
      sendStartTm: resolveSendStartTm(),
      orderId: handoffMeta.value?.orderId,
      sourceSystem: handoffMeta.value?.sourceSystem || (handoffMeta.value?.orderId ? 'ordercore' : undefined),
      order,
    })
    const waybill = await shippingApi.createShipmentWaybill(shipment.id)
    result.value = { shipmentId: waybill.id, mailNo: waybill.mailNo || '' }
    ElMessage.success(`下单成功${waybill.mailNo ? `，运单号 ${waybill.mailNo}` : ''}`)
    if (doPrint) {
      try {
        await printShipmentLabel(waybill.id)
        const isPdf = (carrierView.value?.printChannel || '').toLowerCase() === 'pdf'
        ElMessage.success(isPdf ? '已在浏览器打开官方 PDF 面单' : '已发送到本机打印机')
      } catch (pe) {
        const msg = (pe as Error).message || ''
        if (msg !== 'PRINTER_NOT_SELECTED') {
          ElMessage.warning(msg || '打印失败，可稍后在发货单重试')
        }
      }
    }
  } catch (e) {
    ElMessage.error((e as Error).message || '下单失败')
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  await loadOptions()
  const handoff = consumeSFOrderHandoff()
  if (handoff?.order) applyHandoff(handoff)
})
</script>

<template>
  <div class="sf-page" v-loading="loading">
    <div class="sf-tabs">
      <div class="tab active">顺丰标准寄件</div>
      <div class="tab-spacer" />
      <el-button text type="primary" @click="router.push('/clodop')">打印机设置</el-button>
      <el-button text @click="router.push('/pending')">返回待发货</el-button>
    </div>

    <div v-if="handoffMeta?.orderId || form.orderNo" class="order-banner">
      已带入订单中心 {{ form.orderNo || `#${handoffMeta?.orderId}` }}
    </div>

    <section class="card contacts">
      <div class="contact-col">
        <div class="card-hd">
          <span class="badge ship">寄</span>
          <span>寄件人信息</span>
          <el-select
            v-model="form.shipperProfileId"
            size="small"
            placeholder="选择寄件档案"
            style="width: 180px; margin-left: auto"
            @change="applyShipper"
          >
            <el-option
              v-for="s in shippers"
              :key="s.id"
              :label="s.isDefault ? `${s.name}（默认）` : s.name"
              :value="s.id!"
            />
          </el-select>
        </div>
        <div v-if="shipperView" class="contact-body">
          <div class="line"><label>姓名</label><span>{{ shipperView.name }}</span></div>
          <div class="line"><label>电话</label><span>{{ shipperView.mobile }}</span></div>
          <div class="line">
            <label>地址</label>
            <span>
              {{ [shipperView.province, shipperView.city, shipperView.county].filter(Boolean).join('-') }}
              {{ shipperView.address }}
            </span>
          </div>
          <div v-if="shipperView.company" class="line"><label>公司</label><span>{{ shipperView.company }}</span></div>
          <p class="hint">修改寄件信息请到「寄件人」档案维护</p>
        </div>
        <el-empty v-else description="请先配置寄件人档案" :image-size="64" />
      </div>

      <div class="contact-col">
        <div class="card-hd">
          <span class="badge recv">收</span>
          <span>收件人信息</span>
        </div>
        <div class="contact-body formish">
          <div class="paste-row">
            <el-input
              v-model="form.pasteText"
              type="textarea"
              :rows="2"
              placeholder="智能识别：粘贴「姓名，手机，地址」后点识别"
            />
            <el-button @click="recognizeReceiver">识别</el-button>
          </div>
          <el-form label-position="top" size="default">
            <div class="grid2">
              <el-form-item label="姓名" required>
                <el-input v-model="form.receiverName" placeholder="收件人姓名" />
              </el-form-item>
              <el-form-item label="手机" required>
                <el-input v-model="form.receiverMobile" placeholder="手机号" />
              </el-form-item>
            </div>
            <div class="grid3">
              <el-form-item label="省">
                <el-input v-model="form.receiverProvince" placeholder="省" />
              </el-form-item>
              <el-form-item label="市">
                <el-input v-model="form.receiverCity" placeholder="市" />
              </el-form-item>
              <el-form-item label="区/县">
                <el-input v-model="form.receiverCounty" placeholder="区/县" />
              </el-form-item>
            </div>
            <el-form-item label="详细地址" required>
              <el-input v-model="form.receiverAddress" type="textarea" :rows="2" placeholder="街道门牌等" />
            </el-form-item>
            <div v-if="regionText" class="region-preview">省市区：{{ regionText }}</div>
          </el-form>
        </div>
      </div>
    </section>

    <section class="card cargo-card">
      <div class="card-hd cargo-hd">
        <div class="cargo-hd-left">
          <el-icon class="cargo-icon"><Box /></el-icon>
          <span class="sec-title">物品信息</span>
          <button type="button" class="forbid-link" @click="forbidDialogVisible = true">
            <el-icon><Warning /></el-icon>
            禁寄物品
          </button>
        </div>
        <div class="cargo-hd-right">
          <span class="fill-label">重量/体积/尺寸</span>
          <div class="fill-mode">
            <el-tooltip :content="fillModeTips.total" placement="top">
              <button
                type="button"
                class="fill-btn"
                :class="{ active: form.fillMode === 'total' }"
                @click="setFillMode('total')"
              >
                按合计填写
              </button>
            </el-tooltip>
            <el-tooltip :content="fillModeTips.unit" placement="top">
              <button
                type="button"
                class="fill-btn"
                :class="{ active: form.fillMode === 'unit' }"
                @click="setFillMode('unit')"
              >
                按单件填写
              </button>
            </el-tooltip>
          </div>
          <el-popover v-model:visible="itemLibVisible" placement="bottom-end" :width="220" trigger="click">
            <div class="item-lib">
              <button
                v-for="n in cargoPresets"
                :key="n"
                type="button"
                class="item-lib-btn"
                @click="pickFromItemLib(n)"
              >
                {{ n }}
              </button>
            </div>
            <template #reference>
              <el-button link type="primary" class="lib-link">使用物品库</el-button>
            </template>
          </el-popover>
        </div>
      </div>

      <p class="fill-mode-hint">
        {{ form.fillMode === 'unit' ? fillModeTips.unit : fillModeTips.total }}
        · 两种模式均可添加多个物品行，表头显示全部行合计
      </p>

      <div class="cargo-table-wrap">
        <div class="cargo-table-hd">
          <div class="col-name cell-label required">物品</div>
          <div class="col-parcel cell-label required">
            包裹数
            <el-tooltip content="包裹总数量=母件数量+子件数量。一单需要几个包裹，填写对应数量即可" placement="top">
              <el-icon class="tip-ico"><InfoFilled /></el-icon>
            </el-tooltip>
          </div>
          <div class="col-weight cell-label required">
            {{ weightLabel }}
            <span class="cell-sub">（合计{{ fmtTotalNum(cargoTotals.weight) }}KG）</span>
          </div>
          <div class="col-vol cell-label">
            {{ volumeLabel }}
            <span class="cell-sub">（合计{{ cargoTotals.volume > 0 ? fmtTotalNum(cargoTotals.volume, 6) : '' }}m³）</span>
            <el-tag size="small" type="danger" effect="plain" class="unit-tag">单位已调整为m³</el-tag>
          </div>
          <div class="col-size cell-label">
            {{ sizeLabel }}
            <span class="cell-sub">（CM）</span>
          </div>
          <div class="col-count cell-label required">
            {{ countLabel }}
            <span class="cell-sub">（合计{{ cargoTotals.itemCount || 0 }}件）</span>
            <el-tooltip
              :content="form.fillMode === 'unit' ? '单个包裹内的物品件数；合计=各行件数×包裹数之和' : '该物品类型全部包裹的物品总件数；合计=各行之和'"
              placement="top"
            >
              <el-icon class="tip-ico"><InfoFilled /></el-icon>
            </el-tooltip>
          </div>
          <div class="col-opt">
            <el-button circle size="small" type="primary" plain :icon="Plus" @click="addCargoLine" />
          </div>
        </div>

        <div v-for="(line, idx) in form.cargoLines" :key="idx" class="cargo-table-row">
          <div class="col-name">
            <el-input v-model="line.name" placeholder="请填写物品名称" maxlength="40" />
          </div>
          <div class="col-parcel">
            <el-input-number
              v-model="line.parcelQty"
              :min="1"
              :max="99"
              controls-position="right"
              placeholder="请输入包裹数"
            />
          </div>
          <div class="col-weight">
            <el-input-number
              v-model="line.weight"
              :min="0.01"
              :step="0.1"
              :precision="2"
              controls-position="right"
              :placeholder="form.fillMode === 'unit' ? '单个包裹重量' : '合计物品重量'"
            />
          </div>
          <div class="col-vol">
            <el-input-number
              v-model="line.volume"
              :min="0"
              :step="0.001"
              :precision="6"
              controls-position="right"
              placeholder="合计物品体积"
            />
          </div>
          <div class="col-size">
            <div class="dim-row">
              <el-input-number
                v-model="line.lengthCm"
                :min="0"
                :precision="1"
                controls-position="right"
                placeholder="长"
                @change="onLineDimsChange(line)"
              />
              <span class="dim-x">×</span>
              <el-input-number
                v-model="line.widthCm"
                :min="0"
                :precision="1"
                controls-position="right"
                placeholder="宽"
                @change="onLineDimsChange(line)"
              />
              <span class="dim-x">×</span>
              <el-input-number
                v-model="line.heightCm"
                :min="0"
                :precision="1"
                controls-position="right"
                placeholder="高"
                @change="onLineDimsChange(line)"
              />
            </div>
          </div>
          <div class="col-count">
            <el-input-number
              v-model="line.itemCount"
              :min="1"
              :max="9999"
              controls-position="right"
              :placeholder="form.fillMode === 'unit' ? '请输入件数' : '合计物品数'"
            />
          </div>
          <div class="col-opt">
            <el-button
              circle
              size="small"
              type="danger"
              plain
              :icon="Minus"
              :disabled="form.cargoLines.length <= 1"
              @click="removeCargoLine(idx)"
            />
          </div>
        </div>
      </div>
    </section>

    <section class="card logistics-card">
      <div class="card-hd logistics-hd">
        <div class="logistics-hd-left">
          <el-icon class="logistics-icon"><Van /></el-icon>
          <span class="sec-title">物流信息</span>
        </div>
        <div class="logistics-hd-right">
          <span class="carrier-label">物流账号</span>
          <el-select
            v-model="form.carrierAccountId"
            placeholder="选择物流账号"
            size="small"
            style="width: 220px"
          >
            <el-option v-for="c in carriers" :key="c.id" :label="c.name" :value="c.id!" />
          </el-select>
        </div>
      </div>

      <div class="express-form-row">
        <div class="express-field">
          <div class="cell-label required">物流付款方式</div>
          <el-select v-model="form.payMode" placeholder="请选择" style="width: 100%">
            <el-option
              v-for="opt in payModeOptions"
              :key="opt.value"
              :value="opt.value"
              :label="opt.label"
              :disabled="opt.disabled"
            />
          </el-select>
        </div>

        <div class="express-field product-field">
          <div class="cell-label required">物流产品</div>
          <el-select v-model="form.expressType" placeholder="请选择物流产品" style="width: 100%">
            <el-option
              v-for="p in expressProducts"
              :key="p.value"
              :value="p.value"
              :label="p.name"
            />
          </el-select>
        </div>

        <div class="express-field pickup-field" :class="{ 'is-appoint': form.pickupMode === 'appoint' }">
          <div class="cell-label required">寄件方式</div>
          <div class="pickup-inline">
            <div class="pickup-mode">
              <button
                type="button"
                class="pickup-btn"
                :class="{ active: form.pickupMode === 'self' }"
                @click="setPickupMode('self')"
              >
                自行联系快递员
              </button>
              <button
                type="button"
                class="pickup-btn"
                :class="{ active: form.pickupMode === 'appoint' }"
                @click="setPickupMode('appoint')"
              >
                预约寄件
              </button>
            </div>
            <el-cascader
              v-if="form.pickupMode === 'appoint'"
              v-model="form.appointSlot"
              :options="appointCascaderOptions"
              :placeholder="appointLoading ? '正在查询可约时段…' : '请选择预约上门时间'"
              class="appoint-cascader"
              :props="{ expandTrigger: 'hover' }"
            />
          </div>
          <div v-if="form.pickupMode === 'appoint'" class="pickup-tip">
            将通知快递员按所选时段上门揽收
            <span v-if="resolveSendStartTm()">（{{ resolveSendStartTm() }} 起）</span>
            <span v-if="pickupWindow"> · 当地可揽 {{ pickupWindow.startTm }}-{{ pickupWindow.endTm }}</span>
            <span v-if="appointLoading"> · 刷新时段中…</span>
          </div>
        </div>
      </div>

      <div class="product-reco-hd">
        物流产品推荐
        <span class="product-hint">
          {{ quoteLoading ? '正在查询时效与运费…' : '点击卡片即可切换，与上方「物流产品」同步' }}
        </span>
      </div>
      <div v-if="quoteError" class="quote-err muted">{{ quoteError }}</div>
      <div class="product-cards">
        <button
          v-for="p in expressProducts"
          :key="p.value"
          type="button"
          class="product-card"
          :class="{ active: form.expressType === p.value }"
          @click="form.expressType = p.value"
        >
          <span v-if="p.tag" class="ptag">{{ p.tag }}</span>
          <div class="pname">{{ p.name }}</div>
          <div v-if="p.deliverLabel" class="pdeliver">
            {{ p.deliverLabel.replace(/^预计\s*/, '预计 ') }}
          </div>
          <div v-else-if="p.hint" class="phint">{{ p.hint }}</div>
          <div v-if="fmtFee(p.fee)" class="pfee">预估: ¥{{ fmtFee(p.fee) }} 起</div>
          <span v-if="form.expressType === p.value" class="check">✓</span>
        </button>
      </div>
      <div class="product-current muted">
        当前选择：{{ selectedProduct.name }}
        <template v-if="form.payMode === 'monthly' && carrierView?.custId">
          · 月结卡号 {{ carrierView.custId }}
        </template>
      </div>

    </section>

    <section class="card">
      <div class="card-hd"><span class="sec-title">备注信息</span></div>
      <div class="remark-block">
        <div class="remark-label">
          运单备注
          <span class="tag-opt">可选，显示在面单/清单</span>
        </div>
        <el-input
          v-model="form.remark"
          type="textarea"
          :rows="2"
          maxlength="200"
          show-word-limit
          placeholder="请填写打印面单或账单备注内容"
        />
      </div>
      <div class="remark-block">
        <div class="remark-label">
          上传图片
          <span class="tag-muted">可选，存档于发货中心，可在发货单详情查看</span>
        </div>
        <div class="img-list">
          <div v-for="(url, idx) in form.remarkImages" :key="url" class="img-item">
            <el-image :src="url" fit="cover" :preview-src-list="form.remarkImages" />
            <button type="button" class="img-del" @click="removeRemarkImage(idx)">×</button>
          </div>
          <el-upload
            v-if="form.remarkImages.length < 6"
            class="img-uploader"
            :show-file-list="false"
            accept="image/*"
            :http-request="uploadRemarkImage"
            :disabled="uploadingImg"
          >
            <div class="img-add" v-loading="uploadingImg">
              <el-icon :size="22"><Camera /></el-icon>
              <span>上传图片</span>
            </div>
          </el-upload>
        </div>
      </div>
    </section>

    <div v-if="result" class="result-bar" :class="{ cancelled: result.cancelled }">
      <template v-if="result.cancelled">
        已取消快递单 <strong>{{ result.mailNo || '-' }}</strong>
      </template>
      <template v-else>
        下单成功，运单号 <strong>{{ result.mailNo || '-' }}</strong>
      </template>
      <el-button
        v-if="!result.cancelled"
        link
        type="danger"
        :loading="cancelling"
        @click="cancelWaybill"
      >
        取消快递单
      </el-button>
      <el-button link type="primary" @click="router.push('/shipments')">查看发货单</el-button>
    </div>

    <footer class="action-bar">
      <div class="printer">
        <el-icon><Printer /></el-icon>
        <span v-if="printerName">打印机：{{ printerName }}</span>
        <span v-else class="warn">未选择打印机</span>
        <el-button link type="primary" :icon="RefreshRight" @click="router.push('/clodop')">更换</el-button>
      </div>
      <div class="actions">
        <el-button size="large" :loading="submitting" @click="submit(false)">直接下单</el-button>
        <el-button class="btn-sf" size="large" type="danger" :loading="submitting" @click="submit(true)">
          下单并打印
        </el-button>
      </div>
    </footer>

    <el-dialog
      v-model="forbidDialogVisible"
      title="禁止寄递物品目录"
      width="720px"
      top="6vh"
      append-to-body
      class="forbid-dialog"
    >
      <div class="forbid-scroll">
        <p class="forbid-intro">以下物品禁止寄递，请确认托寄物不在目录范围内。</p>
        <section v-for="sec in forbidItemsCatalog" :key="sec.no" class="forbid-sec">
          <h4>{{ sec.no }}、{{ sec.title }}</h4>
          <p v-if="sec.body">{{ sec.body }}</p>
          <ol v-if="sec.items?.length">
            <li v-for="(it, i) in sec.items" :key="i">{{ it }}</li>
          </ol>
        </section>
      </div>
      <template #footer>
        <el-button type="primary" @click="forbidDialogVisible = false">我知道了</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.sf-page {
  max-width: 1280px;
  margin: 0 auto;
  padding: 0 0 96px;
}
.sf-tabs {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid #ebeef5;
}
.tab {
  padding: 8px 14px;
  font-weight: 600;
  color: #909399;
  border-bottom: 2px solid transparent;
  margin-bottom: -9px;
}
.tab.active {
  color: #c8161d;
  border-bottom-color: #c8161d;
}
.tab-spacer { flex: 1; }
.order-banner {
  background: #fff7e6;
  border: 1px solid #ffe58f;
  border-radius: 8px;
  padding: 8px 12px;
  margin-bottom: 12px;
  font-size: 13px;
}
.muted { color: #909399; }
.card {
  background: #fff;
  border: 1px solid #ebeef5;
  border-radius: 10px;
  padding: 14px 16px;
  margin-bottom: 12px;
}
.card-hd {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
  font-weight: 600;
}
.sec-title { font-size: 15px; }
.badge {
  width: 22px;
  height: 22px;
  border-radius: 4px;
  color: #fff;
  font-size: 13px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.badge.ship { background: #c8161d; }
.badge.recv { background: #1677ff; }
.contacts {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  padding: 0;
  border: none;
  background: transparent;
}
.contact-col {
  background: #fff;
  border: 1px solid #ebeef5;
  border-radius: 10px;
  padding: 14px 16px;
}
.contact-body .line {
  display: flex;
  gap: 10px;
  margin-bottom: 8px;
  font-size: 14px;
  line-height: 1.5;
}
.contact-body .line label {
  width: 40px;
  color: #909399;
  flex-shrink: 0;
}
.hint {
  margin: 10px 0 0;
  font-size: 12px;
  color: #a8abb2;
}
.paste-row {
  display: flex;
  gap: 8px;
  margin-bottom: 10px;
  align-items: flex-start;
}
.paste-row .el-input { flex: 1; }
.grid2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 12px;
}
.grid3 {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 8px 12px;
}
.formish :deep(.el-form-item) { margin-bottom: 10px; }
.region-preview { font-size: 12px; color: #909399; margin-top: -4px; }
.cargo-hd {
  flex-wrap: wrap;
  gap: 10px 16px;
}
.cargo-hd-left,
.cargo-hd-right {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.cargo-icon {
  color: #c8161d;
  font-size: 18px;
}
.forbid-link {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 12px;
  color: #909399;
  border: none;
  background: transparent;
  padding: 0;
  cursor: pointer;
}
.forbid-link:hover { color: #c8161d; }
.forbid-scroll {
  max-height: 65vh;
  overflow: auto;
  padding-right: 8px;
  font-size: 13px;
  line-height: 1.65;
  color: #303133;
}
.forbid-intro {
  margin: 0 0 12px;
  color: #c8161d;
  font-size: 13px;
}
.forbid-sec {
  margin-bottom: 14px;
}
.forbid-sec h4 {
  margin: 0 0 6px;
  font-size: 14px;
  font-weight: 600;
  color: #1f2329;
}
.forbid-sec p {
  margin: 0;
  color: #4e5969;
}
.forbid-sec ol {
  margin: 4px 0 0;
  padding-left: 1.4em;
  color: #4e5969;
}
.forbid-sec li { margin-bottom: 4px; }
.fill-label {
  font-size: 12px;
  color: #909399;
}
.fill-mode {
  display: inline-flex;
  border: 1px solid #e4e7ec;
  border-radius: 4px;
  overflow: hidden;
}
.fill-btn {
  border: none;
  background: #fff;
  padding: 4px 10px;
  font-size: 12px;
  color: #606266;
  cursor: pointer;
  line-height: 1.4;
}
.fill-btn + .fill-btn { border-left: 1px solid #e4e7ec; }
.fill-btn.active {
  background: #fff1f0;
  color: #c8161d;
  font-weight: 600;
}
.fill-mode-hint {
  margin: -4px 0 12px;
  font-size: 12px;
  color: #909399;
  line-height: 1.4;
}
.lib-link { font-size: 13px; }
.item-lib {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.item-lib-btn {
  border: none;
  background: transparent;
  text-align: left;
  padding: 8px 10px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 13px;
  color: #303133;
}
.item-lib-btn:hover {
  background: #f5f7fa;
  color: #c8161d;
}
.cargo-table-wrap {
  overflow-x: auto;
}
.cargo-table-hd,
.cargo-table-row {
  display: grid;
  grid-template-columns: minmax(140px, 1.2fr) 90px 130px 130px minmax(240px, 1.4fr) 130px 40px;
  gap: 8px;
  align-items: center;
  min-width: 980px;
}
.cargo-table-hd {
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid #f0f2f5;
}
.cargo-table-row {
  margin-bottom: 8px;
}
.cargo-table-row :deep(.el-input-number) { width: 100%; }
.col-opt {
  display: flex;
  justify-content: center;
  align-items: center;
}
.cell-label {
  font-size: 13px;
  color: #606266;
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
  min-height: 22px;
}
.cell-label.required::before {
  content: '*';
  color: #c8161d;
  margin-right: 2px;
}
.cell-sub {
  font-size: 12px;
  color: #a8abb2;
  font-weight: normal;
}
.tip-ico {
  color: #c0c4cc;
  font-size: 14px;
  cursor: help;
}
.unit-tag {
  margin-left: 2px;
}
.dim-row {
  display: flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 2px;
}
.dim-row :deep(.el-input-number) { width: 72px; }
.dim-x { color: #909399; padding: 0 1px; font-size: 12px; }
.remark-block + .remark-block { margin-top: 14px; }
.remark-label {
  font-size: 13px;
  color: #303133;
  margin-bottom: 6px;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.tag-opt { color: #c8161d; font-size: 12px; }
.tag-muted { color: #a8abb2; font-size: 12px; }
.img-list {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.img-item {
  position: relative;
  width: 88px;
  height: 88px;
  border: 1px solid #e4e7ec;
  border-radius: 6px;
  overflow: hidden;
}
.img-item :deep(.el-image) {
  width: 100%;
  height: 100%;
}
.img-del {
  position: absolute;
  top: 2px;
  right: 2px;
  width: 20px;
  height: 20px;
  border: none;
  border-radius: 50%;
  background: rgba(0, 0, 0, 0.55);
  color: #fff;
  cursor: pointer;
  line-height: 1;
  font-size: 14px;
}
.img-uploader :deep(.el-upload) { display: block; }
.img-add {
  width: 88px;
  height: 88px;
  border: 1px dashed #d0d5dd;
  border-radius: 6px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  color: #98a2b3;
  font-size: 12px;
  cursor: pointer;
  background: #fafafa;
}
.img-add:hover {
  border-color: #c8161d;
  color: #c8161d;
}
.logistics-hd {
  flex-wrap: wrap;
  gap: 10px 16px;
}
.logistics-hd-left,
.logistics-hd-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.logistics-icon {
  color: #c8161d;
  font-size: 18px;
}
.carrier-label {
  font-size: 12px;
  color: #909399;
}
.express-form-row {
  display: flex;
  flex-wrap: wrap;
  gap: 16px 24px;
  align-items: flex-start;
  margin-bottom: 14px;
  padding-bottom: 14px;
  border-bottom: 1px dashed #ebeef5;
}
.express-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 180px;
  flex: 1;
}
.express-field.product-field { min-width: 160px; max-width: 220px; flex: 0.8; }
.express-field.pickup-field { min-width: 280px; flex: 1.6; }
.express-field.pickup-field.is-appoint {
  flex: 2 1 420px;
  min-width: 420px;
}
.pickup-inline {
  display: flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 12px;
  min-width: 0;
}
.pickup-mode {
  display: inline-flex;
  flex-shrink: 0;
  border: 1px solid #e4e7ec;
  border-radius: 4px;
  overflow: hidden;
  width: fit-content;
}
.pickup-btn {
  border: none;
  background: #fff;
  padding: 8px 14px;
  font-size: 13px;
  color: #606266;
  cursor: pointer;
  line-height: 1.4;
  white-space: nowrap;
}
.pickup-btn + .pickup-btn { border-left: 1px solid #e4e7ec; }
.pickup-btn.active {
  background: #fff1f0;
  color: #c8161d;
  font-weight: 600;
}
.appoint-cascader {
  width: 280px;
  min-width: 220px;
  flex: 1 1 220px;
  max-width: 320px;
}
.pickup-tip {
  font-size: 12px;
  color: #a8abb2;
}
.product-reco-hd {
  font-size: 13px;
  color: #606266;
  margin-bottom: 10px;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}
.product-hint {
  font-size: 12px;
  color: #a8abb2;
  font-weight: 400;
}
.product-cards {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.product-card {
  position: relative;
  width: 168px;
  text-align: left;
  border: 1px solid #dcdfe6;
  background: #fff;
  border-radius: 8px;
  padding: 14px 14px 12px;
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
  overflow: hidden;
}
.product-card:hover { border-color: #f89898; }
.product-card.active {
  border-color: #c8161d;
  background: #fff5f5;
  box-shadow: 0 0 0 1px #c8161d inset;
}
.ptag {
  display: inline-block;
  font-size: 11px;
  color: #c8161d;
  background: #fff1f0;
  border-radius: 2px;
  padding: 1px 6px;
  margin-bottom: 6px;
}
.pname { font-weight: 700; font-size: 15px; color: #303133; }
.phint { margin-top: 4px; font-size: 12px; color: #909399; line-height: 1.4; }
.pdeliver { margin-top: 6px; font-size: 12px; color: #606266; line-height: 1.4; }
.pfee { margin-top: 4px; font-size: 12px; color: #c8161d; font-weight: 600; }
.quote-err { margin: 0 0 8px; font-size: 12px; color: #e6a23c; }
.product-current {
  margin-top: 10px;
  font-size: 12px;
}
.check {
  position: absolute;
  right: 8px;
  bottom: 6px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: #c8161d;
  color: #fff;
  font-size: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.result-bar {
  background: #f0f9eb;
  border: 1px solid #e1f3d8;
  border-radius: 8px;
  padding: 10px 14px;
  margin-bottom: 12px;
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.result-bar.cancelled {
  background: #f4f4f5;
  border-color: #e4e7ed;
  color: #909399;
}
.action-bar {
  position: sticky;
  bottom: 0;
  z-index: 20;
  margin: 0 -4px;
  padding: 12px 16px;
  background: #fff;
  border: 1px solid #ebeef5;
  border-radius: 10px;
  box-shadow: 0 -4px 16px rgba(0, 0, 0, 0.06);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.printer {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: #606266;
}
.printer .warn { color: #e6a23c; }
.actions { display: flex; gap: 10px; }
.btn-sf {
  --el-button-bg-color: #c8161d;
  --el-button-border-color: #c8161d;
  --el-button-hover-bg-color: #a91218;
  --el-button-hover-border-color: #a91218;
  --el-button-active-bg-color: #8f0f14;
  --el-button-active-border-color: #8f0f14;
  min-width: 128px;
}
@media (max-width: 900px) {
  .contacts { grid-template-columns: 1fr; }
  .grid2, .grid3 { grid-template-columns: 1fr; }
}
</style>
