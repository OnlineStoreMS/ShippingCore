<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox, type UploadRequestOptions } from 'element-plus'
import { Camera, Printer, RefreshRight } from '@element-plus/icons-vue'
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
  goodsParcelQty,
  goodsShipName,
  parsePastedContact,
  type SFOrderHandoff,
} from '../utils/sfOrderHandoff'
import {
  getSavedPrinterIndex,
  getSavedPrinterName,
  printWithSFPlugin,
} from '../utils/sfPrintPlugin'

const cargoPresets = ['文件', '电子产品', '日用品', '服装', '食品', '配件', '商品']

const router = useRouter()

const EXPRESS_TYPE_KEY = 'shippingcore.sf.expressType'

const expressProducts = [
  { value: '1', name: '顺丰特快', hint: '时效更快' },
  { value: '2', name: '顺丰标快', hint: '经济实惠' },
  { value: '6', name: '顺丰即日', hint: '当日达（视区域）' },
] as const

function loadSavedExpressType(): string {
  const v = localStorage.getItem(EXPRESS_TYPE_KEY)
  if (v === '1' || v === '2' || v === '6') return v
  return '2'
}

const loading = ref(false)
const submitting = ref(false)
const cancelling = ref(false)
const carriers = ref<CarrierAccount[]>([])
const shippers = ref<ShipperProfile[]>([])
const handoffMeta = ref<Pick<SFOrderHandoff, 'orderId' | 'sourceSystem'> | null>(null)
const result = ref<{ shipmentId: number; mailNo: string; cancelled?: boolean } | null>(null)

const form = reactive({
  carrierAccountId: undefined as number | undefined,
  shipperProfileId: undefined as number | undefined,
  payMode: 'monthly' as 'monthly' | 'cash',
  expressType: loadSavedExpressType(),
  cargoName: '文件',
  parcelQty: 1,
  cargoCount: 1,
  totalWeight: 1,
  lengthCm: undefined as number | undefined,
  widthCm: undefined as number | undefined,
  heightCm: undefined as number | undefined,
  pickupMode: 'self' as 'self' | 'appoint',
  remark: '',
  courierNote: '',
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
  sysTid: '',
  sourceTid: '',
  goods: [] as OrderSnapshot['goods'],
})

const uploadingImg = ref(false)
const computedVolume = computed(() => {
  const l = form.lengthCm || 0
  const w = form.widthCm || 0
  const h = form.heightCm || 0
  if (l <= 0 || w <= 0 || h <= 0) return 0
  return Math.round((l * w * h) / 1000) / 1000 // dm³ → 显示用，存 m³ 时再 /1000
})
const volumeM3 = computed(() => {
  const l = form.lengthCm || 0
  const w = form.widthCm || 0
  const h = form.heightCm || 0
  if (l <= 0 || w <= 0 || h <= 0) return 0
  return (l * w * h) / 1_000_000
})

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
  form.sysTid = o.sysTid
  form.sourceTid = o.sourceTid
  form.receiverName = o.receiverName
  form.receiverMobile = o.receiverMobile
  form.receiverProvince = o.receiverProvince
  form.receiverCity = o.receiverCity
  form.receiverCounty = o.receiverCounty
  form.receiverAddress = o.receiverAddress
  form.goods = o.goods || []
  form.cargoName = goodsCargoName(form.goods) || '文件'
  form.parcelQty = goodsParcelQty(form.goods) || 1
  form.cargoCount = form.goods.reduce((s, g) => s + (g.num > 0 ? g.num : 1), 0) || 1
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
    form.payMode = c.useMonthly ? 'monthly' : 'cash'
  },
)

watch(
  () => form.expressType,
  (v) => {
    if (v === '1' || v === '2' || v === '6') {
      localStorage.setItem(EXPRESS_TYPE_KEY, v)
    }
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
  const sysTid = form.sysTid.trim() || `SC-MANUAL-${Date.now()}`
  const sourceTid = form.sourceTid.trim() || sysTid
  const goods =
    form.goods.length > 0
      ? form.goods.map((g) => ({
          ...g,
          // 规格名称优先；无规格时把手填托寄物写入 skuName 作为发货内容
          skuName: (g.skuName || form.cargoName || g.title || '').trim(),
          title: g.title || '',
          num: g.num > 0 ? g.num : 1,
        }))
      : [
          {
            title: '',
            skuName: form.cargoName || '商品',
            num: form.parcelQty > 0 ? form.parcelQty : 1,
            outerId: '',
            price: 0,
          },
        ]
  return {
    platform: form.platform || 'manual',
    shopId: form.shopId || '',
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

function validate(): string | null {
  if (!form.carrierAccountId) return '请选择物流账号'
  if (!form.shipperProfileId) return '请选择寄件人'
  if (!form.receiverName.trim() || !form.receiverMobile.trim()) return '请填写收件人姓名与手机'
  if (!form.receiverAddress.trim()) return '请填写收件详细地址'
  if (!form.cargoName.trim()) return '请填写物品名称'
  return null
}

async function printShipmentLabel(shipmentId: number) {
  const printerIndex = getSavedPrinterIndex()
  if (printerIndex == null) {
    ElMessage.warning('请先在 C-Lodop 云打印 选择本机打印机')
    await router.push('/clodop')
    throw new Error('PRINTER_NOT_SELECTED')
  }
  const pluginData = await shippingApi.fetchShipmentPrintPluginData(shipmentId)
  await printWithSFPlugin(pluginData, { printerIndex })
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
      payMethod: 1,
      remark: form.remark.trim(),
      courierNote: form.courierNote.trim() || undefined,
      remarkImages: form.remarkImages.length ? [...form.remarkImages] : undefined,
      cargoName: form.cargoName.trim(),
      parcelQty: form.parcelQty,
      cargoCount: form.cargoCount,
      totalWeight: form.totalWeight > 0 ? form.totalWeight : undefined,
      lengthCm: form.lengthCm || undefined,
      widthCm: form.widthCm || undefined,
      heightCm: form.heightCm || undefined,
      totalVolume: volumeM3.value || undefined,
      pickupMode: form.pickupMode,
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
        ElMessage.success('已发送到本机打印机')
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

    <div v-if="handoffMeta?.orderId" class="order-banner">
      已带入订单中心 #{{ handoffMeta.orderId }}
      <span v-if="form.sourceTid" class="muted"> · {{ form.sourceTid }}</span>
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

    <section class="card">
      <div class="card-hd"><span class="sec-title">物品信息</span></div>
      <div class="cargo-grid">
        <el-form-item label="物品" required>
          <el-select
            v-model="form.cargoName"
            filterable
            allow-create
            default-first-option
            placeholder="请填写物品名称"
            style="width: 200px"
          >
            <el-option v-for="n in cargoPresets" :key="n" :label="n" :value="n" />
          </el-select>
        </el-form-item>
        <el-form-item label="包裹数">
          <el-input-number v-model="form.parcelQty" :min="1" :max="99" controls-position="right" />
        </el-form-item>
        <el-form-item label="总包裹重量">
          <div class="unit-field">
            <el-input-number
              v-model="form.totalWeight"
              :min="0.01"
              :step="0.1"
              :precision="2"
              controls-position="right"
            />
            <span class="unit">KG</span>
          </div>
        </el-form-item>
        <el-form-item label="总包裹体积">
          <div class="dim-row">
            <el-input-number
              v-model="form.lengthCm"
              :min="0"
              :precision="1"
              controls-position="right"
              placeholder="长"
            />
            <span class="dim-x">×</span>
            <el-input-number
              v-model="form.widthCm"
              :min="0"
              :precision="1"
              controls-position="right"
              placeholder="宽"
            />
            <span class="dim-x">×</span>
            <el-input-number
              v-model="form.heightCm"
              :min="0"
              :precision="1"
              controls-position="right"
              placeholder="高"
            />
            <span class="unit">CM</span>
            <span v-if="computedVolume > 0" class="vol-tip">≈ {{ computedVolume }} dm³</span>
          </div>
        </el-form-item>
        <el-form-item label="总包裹物品数">
          <el-input-number v-model="form.cargoCount" :min="1" :max="9999" controls-position="right" />
        </el-form-item>
      </div>
      <div v-if="form.goods.length" class="goods-preview">
        <div class="goods-hd">订单商品明细</div>
        <div v-for="(g, i) in form.goods" :key="i" class="goods-line">
          {{ goodsShipName(g) }} × {{ g.num }}
        </div>
      </div>
    </section>

    <section class="card">
      <div class="card-hd"><span class="sec-title">物流信息</span></div>
      <div class="logistics-row">
        <el-form-item label="物流账号" required>
          <el-select v-model="form.carrierAccountId" placeholder="选择物流账号" style="width: 260px">
            <el-option v-for="c in carriers" :key="c.id" :label="c.name" :value="c.id!" />
          </el-select>
        </el-form-item>
        <el-form-item label="物流付款方式">
          <el-select v-model="form.payMode" style="width: 220px">
            <el-option
              value="monthly"
              :label="carrierView?.custId ? `寄付月结 / ${carrierView.custId}` : '寄付月结'"
            />
            <el-option value="cash" label="寄付现结" />
          </el-select>
        </el-form-item>
        <el-form-item label="寄件方式">
          <el-radio-group v-model="form.pickupMode">
            <el-radio value="self">自行联系快递员</el-radio>
            <el-radio value="appoint">预约寄件</el-radio>
          </el-radio-group>
        </el-form-item>
      </div>

      <div class="product-label">
        物流产品
        <span class="product-hint">每次下单可选，不跟物流账号绑定</span>
      </div>
      <div class="product-cards">
        <button
          v-for="p in expressProducts"
          :key="p.value"
          type="button"
          class="product-card"
          :class="{ active: form.expressType === p.value }"
          @click="form.expressType = p.value"
        >
          <div class="pname">{{ p.name }}</div>
          <div class="phint">{{ p.hint }}</div>
          <span v-if="form.expressType === p.value" class="check">✓</span>
        </button>
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
          给快递员捎个话
          <span class="tag-warn">仅快递员可见，不显示在面单/清单上</span>
        </div>
        <el-input
          v-model="form.courierNote"
          type="textarea"
          :rows="2"
          maxlength="40"
          show-word-limit
          placeholder="请输入你要对快递员说的话……"
        />
      </div>
      <div class="remark-block">
        <div class="remark-label">上传图片 <span class="tag-muted">可选，随发货单存档</span></div>
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
  </div>
</template>

<style scoped>
.sf-page {
  max-width: 1100px;
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
.cargo-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 20px;
  align-items: flex-end;
}
.cargo-grid :deep(.el-form-item) { margin-bottom: 0; }
.unit-field {
  display: flex;
  align-items: center;
  gap: 6px;
}
.dim-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
}
.dim-row :deep(.el-input-number) { width: 100px; }
.dim-x { color: #909399; padding: 0 2px; }
.unit {
  font-size: 12px;
  color: #909399;
  margin-left: 2px;
}
.vol-tip {
  margin-left: 8px;
  font-size: 12px;
  color: #a8abb2;
}
.goods-preview {
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px dashed #ebeef5;
  font-size: 13px;
  color: #606266;
}
.goods-hd {
  font-size: 12px;
  color: #909399;
  margin-bottom: 6px;
}
.goods-line + .goods-line { margin-top: 4px; }
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
.tag-warn { color: #c8161d; font-size: 12px; }
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
.logistics-row {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 20px;
  align-items: center;
  margin-bottom: 8px;
}
.logistics-row :deep(.el-form-item) { margin-bottom: 0; }
.cust { font-size: 12px; color: #909399; }
.product-label {
  font-size: 13px;
  color: #606266;
  margin: 8px 0;
}
.product-hint {
  margin-left: 8px;
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
  width: 160px;
  text-align: left;
  border: 1px solid #dcdfe6;
  background: #fafafa;
  border-radius: 8px;
  padding: 12px 14px;
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
}
.product-card:hover { border-color: #f89898; }
.product-card.active {
  border-color: #c8161d;
  background: #fff5f5;
  box-shadow: 0 0 0 1px #c8161d inset;
}
.pname { font-weight: 700; font-size: 15px; color: #303133; }
.phint { margin-top: 4px; font-size: 12px; color: #909399; }
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
