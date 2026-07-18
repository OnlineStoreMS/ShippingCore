<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus, Delete } from '@element-plus/icons-vue'
import { api } from '../../api/wms'
import ImageField from '../../components/ImageField.vue'
import SupplierPickerDialog from '../../components/SupplierPickerDialog.vue'

const props = defineProps<{
  modelValue: boolean
  productId?: number | null
  defaultProductType?: string
  categories: any[]
  warehouses: any[]
  packSpecs: any[]
  presetCategoryId?: number
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'saved'): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const saving = ref(false)
const activeTab = ref('base')
const form = ref<any>({})
const skus = ref<any[]>([])
const suppliers = ref<any[]>([])
const descriptions = ref<any[]>([])
const supplierPickerVisible = ref(false)

/** 对齐普源多语言商品描述常用语种 */
const languageOptions = [
  { code: 'zh-CN', name: '中文' },
  { code: 'en', name: '英语' },
  { code: 'de', name: '德语' },
  { code: 'fr', name: '法语' },
  { code: 'es', name: '西班牙语' },
  { code: 'it', name: '意大利语' },
  { code: 'pt', name: '葡萄牙语' },
  { code: 'ru', name: '俄语' },
  { code: 'ja', name: '日语' },
  { code: 'ko', name: '韩语' },
  { code: 'ar', name: '阿拉伯语' },
  { code: 'th', name: '泰语' },
  { code: 'vi', name: '越南语' },
  { code: 'id', name: '印尼语' },
  { code: 'ms', name: '马来语' },
  { code: 'pl', name: '波兰语' },
  { code: 'nl', name: '荷兰语' },
  { code: 'tr', name: '土耳其语' },
]

const selectedSupplierIds = computed(() =>
  suppliers.value.map((s) => Number(s.supplierId)).filter((id) => id > 0),
)

const goodsKindOptions = [
  { label: '普通商品', value: 'normal' },
  { label: '包材', value: 'packaging' },
  { label: '配件', value: 'accessory' },
  { label: '赠品', value: 'gift' },
]

const title = computed(() => {
  if (form.value.id) return '编辑商品'
  const t = props.defaultProductType || 'normal'
  if (t === 'combo') return '新增组合品'
  if (t === 'assembly') return '新增组装品(加工产品)'
  return '新增普通商品'
})

function emptySku(productType = 'normal') {
  return {
    id: undefined,
    skuCode: '',
    pickName: '',
    style1: '',
    style2: '',
    style3: '',
    status: 'active',
    goodsKind: 'normal',
    lastPurchasePrice: 0,
    minPurchasePrice: 0,
    retailPrice: 0,
    weightG: 0,
    upc: '',
    asin: '',
    supplierItemNo: '',
    description: '',
    pic: '',
    productType,
  }
}

function emptySupplier() {
  return {
    id: undefined,
    supplierId: undefined as number | undefined,
    supplierCode: '',
    supplierName: '',
    purchaseUrl: '',
    price: 0,
    remark: '',
    contactName: '',
    phone: '',
    isDefault: 0,
    sort: 0,
  }
}

function emptyDescription() {
  return {
    id: undefined,
    languageCode: '',
    languageName: '',
    title: '',
    description: '',
    sort: 0,
  }
}

function blankProductExtras() {
  return {
    features: '',
    aliasCn: '',
    aliasEn: '',
    declareWeightG: 0,
    declaredValue: 0,
    originCountryCode: '',
    hsCode: '',
    exportDeclaredValue: 0,
    purchaseChannel: '',
    purchaser: '',
    minPurchasePrice: 0,
    stockMinAmount: 0,
    packFee: 0,
    packageCount: 0,
    outLong: 0,
    outWide: 0,
    outHigh: 0,
    outGrossWeight: 0,
    outNetWeight: 0,
    inLong: 0,
    inWide: 0,
    inHigh: 0,
    inGrossWeight: 0,
    inNetWeight: 0,
    packMsg: '',
    shopTitle: '',
    brand: '',
    specClass: '',
    model: '',
    material: '',
    style: '',
    season: '',
    unit: '',
    retailPrice: 0,
    batchPrice: 0,
    maxSalePrice: 0,
    minSalePrice: 0,
    marketPrice: 0,
  }
}

function resetCreate() {
  const today = new Date().toISOString().slice(0, 10)
  const pt = props.defaultProductType || 'normal'
  form.value = {
    parentSku: '',
    name: '',
    categoryId: props.presetCategoryId || undefined,
    packSpecId: undefined,
    scoreFactor: 1,
    remark: '',
    status: 1,
    developedAt: today,
    defaultWarehouseId: props.warehouses.find((w) => w.isDefault)?.id || undefined,
    pic: '',
    albumPics: '',
    defaultProductType: pt,
    ...blankProductExtras(),
  }
  skus.value = [emptySku(pt)]
  suppliers.value = []
  descriptions.value = []
  activeTab.value = 'base'
}

async function loadEdit(id: number) {
  const detail = await api.getProduct(id)
  form.value = {
    ...blankProductExtras(),
    ...detail,
    developedAt: detail.developedAt ? String(detail.developedAt).slice(0, 10) : '',
    defaultProductType: detail.skus?.[0]?.productType || 'normal',
  }
  skus.value = (detail.skus || []).map((s: any) => ({
    ...emptySku(s.productType || 'normal'),
    ...s,
    goodsKind: s.goodsKind || 'normal',
  }))
  if (!skus.value.length) skus.value = [emptySku(form.value.defaultProductType)]
  suppliers.value = (detail.suppliers || []).map((s: any) => ({
    ...emptySupplier(),
    ...s,
    isDefault: s.isDefault ? 1 : 0,
  }))
  descriptions.value = (detail.descriptions || []).map((d: any) => ({
    ...emptyDescription(),
    ...d,
  }))
  activeTab.value = 'base'
}

watch(
  () => props.modelValue,
  async (open) => {
    if (!open) return
    try {
      if (props.productId) await loadEdit(props.productId)
      else resetCreate()
    } catch (e) {
      ElMessage.error((e as Error).message || '加载失败')
      visible.value = false
    }
  },
)

function addSkuRow() {
  skus.value.push(emptySku(form.value.defaultProductType || 'normal'))
}

function removeSkuRow(idx: number) {
  if (skus.value.length <= 1) {
    ElMessage.warning('至少保留一条库存SKU')
    return
  }
  skus.value.splice(idx, 1)
}

function openSupplierPicker() {
  supplierPickerVisible.value = true
}

function onSupplierSelected(s: any) {
  if (suppliers.value.some((x) => x.supplierId === s.id)) {
    ElMessage.warning('该供应商已添加')
    return
  }
  const row = emptySupplier()
  row.supplierId = s.id
  row.supplierCode = s.code || ''
  row.supplierName = s.name || ''
  row.contactName = s.contactName || ''
  row.phone = s.phone || ''
  if (!suppliers.value.length) row.isDefault = 1
  suppliers.value.push(row)
}

function removeSupplierRow(idx: number) {
  const wasDefault = suppliers.value[idx]?.isDefault
  suppliers.value.splice(idx, 1)
  if (wasDefault && suppliers.value.length) suppliers.value[0].isDefault = 1
}

function setDefaultSupplier(idx: number) {
  suppliers.value.forEach((s, i) => {
    s.isDefault = i === idx ? 1 : 0
  })
}

function addDescriptionRow() {
  descriptions.value.push(emptyDescription())
}

function removeDescriptionRow(idx: number) {
  descriptions.value.splice(idx, 1)
}

function onLanguageChange(row: any, code: string) {
  const opt = languageOptions.find((o) => o.code === code)
  row.languageCode = code || ''
  row.languageName = opt?.name || ''
}

function fillSkuFromParent() {
  if (!form.value.parentSku) return
  const first = skus.value[0]
  if (first && !first.skuCode) first.skuCode = form.value.parentSku
  if (first && !first.pickName) first.pickName = form.value.name || ''
}

function onPackSpecChange(id?: number) {
  const p = props.packSpecs.find((x) => x.id === id)
  if (!p) return
  if (!form.value.packFee) form.value.packFee = p.cost || 0
}

async function save() {
  if (!form.value.parentSku?.trim()) {
    ElMessage.warning('请填写父SKU/主SKU')
    activeTab.value = 'base'
    return
  }
  if (!form.value.name?.trim()) {
    ElMessage.warning('请填写商品名称')
    activeTab.value = 'base'
    return
  }
  const validSkus = skus.value.filter((s) => String(s.skuCode || '').trim())
  if (!validSkus.length) {
    ElMessage.warning('请至少填写一条库存SKU')
    activeTab.value = 'base'
    return
  }
  const codes = new Set<string>()
  for (const s of validSkus) {
    const c = String(s.skuCode).trim()
    if (codes.has(c)) {
      ElMessage.warning(`库存SKU重复：${c}`)
      return
    }
    codes.add(c)
  }

  const selectedSuppliers = suppliers.value.filter((s) => s.supplierId)
  const supplierIds = new Set<number>()
  for (const s of selectedSuppliers) {
    if (supplierIds.has(s.supplierId)) {
      ElMessage.warning(`供应商重复：${s.supplierName || s.supplierId}`)
      activeTab.value = 'purchase'
      return
    }
    supplierIds.add(s.supplierId)
  }

  const selectedDescriptions = descriptions.value.filter(
    (d) => String(d.languageCode || '').trim() || String(d.title || '').trim() || String(d.description || '').trim(),
  )
  const langCodes = new Set<string>()
  for (const d of selectedDescriptions) {
    const code = String(d.languageCode || '').trim()
    if (!code) {
      ElMessage.warning('多语言描述请选择语言')
      activeTab.value = 'sale'
      return
    }
    if (langCodes.has(code)) {
      ElMessage.warning(`语言重复：${d.languageName || code}`)
      activeTab.value = 'sale'
      return
    }
    langCodes.add(code)
  }

  const body = {
    ...form.value,
    parentSku: form.value.parentSku.trim(),
    name: form.value.name.trim(),
    categoryId: form.value.categoryId || 0,
    packSpecId: form.value.packSpecId || 0,
    developedAt: form.value.developedAt || '',
    defaultWarehouseId: form.value.defaultWarehouseId || 0,
    scoreFactor: form.value.scoreFactor ?? 1,
    status: form.value.status ?? 1,
    defaultProductType: form.value.defaultProductType || 'normal',
    skus: validSkus.map((s) => ({
      id: s.id || undefined,
      skuCode: String(s.skuCode).trim(),
      pickName: s.pickName || '',
      style1: s.style1 || '',
      style2: s.style2 || '',
      style3: s.style3 || '',
      status: s.status || 'active',
      productType: s.productType || form.value.defaultProductType || 'normal',
      goodsKind: s.goodsKind || 'normal',
      lastPurchasePrice: Number(s.lastPurchasePrice) || 0,
      minPurchasePrice: Number(s.minPurchasePrice) || 0,
      retailPrice: Number(s.retailPrice) || 0,
      weightG: Number(s.weightG) || 0,
      upc: s.upc || '',
      asin: s.asin || '',
      supplierItemNo: s.supplierItemNo || '',
      description: s.description || '',
      pic: s.pic || '',
    })),
    suppliers: selectedSuppliers.map((s, i) => ({
      id: s.id || undefined,
      supplierId: s.supplierId,
      supplierCode: s.supplierCode || '',
      supplierName: s.supplierName || '',
      purchaseUrl: s.purchaseUrl || '',
      price: Number(s.price) || 0,
      remark: s.remark || '',
      contactName: s.contactName || '',
      phone: s.phone || '',
      isDefault: s.isDefault ? 1 : 0,
      sort: i + 1,
    })),
    descriptions: selectedDescriptions.map((d, i) => ({
      id: d.id || undefined,
      languageCode: String(d.languageCode).trim(),
      languageName: d.languageName || '',
      title: d.title || '',
      description: d.description || '',
      sort: i + 1,
    })),
  }

  saving.value = true
  try {
    if (form.value.id) await api.updateProductWithSkus(form.value.id, body)
    else await api.createProductWithSkus(body)
    ElMessage.success('已保存')
    visible.value = false
    emit('saved')
  } catch (e) {
    ElMessage.error((e as Error).message || '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="title"
    width="1180px"
    top="3vh"
    destroy-on-close
    :close-on-click-modal="false"
  >
    <el-tabs v-model="activeTab">
      <!-- 基本信息 -->
      <el-tab-pane label="基本信息" name="base">
        <div class="section-title">基础信息</div>
        <el-form :model="form" label-width="120px">
          <el-row :gutter="12">
            <el-col :span="4">
              <el-form-item label="商品主图" label-width="80px">
                <ImageField v-model="form.pic" tip="主图" subdir="products" />
              </el-form-item>
            </el-col>
            <el-col :span="20">
              <el-row :gutter="12">
                <el-col :span="8">
                  <el-form-item label="父SKU/主SKU" required>
                    <el-input v-model="form.parentSku" :disabled="!!form.id" @blur="fillSkuFromParent" />
                  </el-form-item>
                </el-col>
                <el-col :span="8">
                  <el-form-item label="商品类别">
                    <el-select v-model="form.categoryId" clearable filterable style="width: 100%">
                      <el-option v-for="c in categories" :key="c.id" :label="c.name" :value="c.id" />
                    </el-select>
                  </el-form-item>
                </el-col>
                <el-col :span="8">
                  <el-form-item label="开发日期">
                    <el-date-picker v-model="form.developedAt" type="date" value-format="YYYY-MM-DD" style="width: 100%" />
                  </el-form-item>
                </el-col>
                <el-col :span="8">
                  <el-form-item label="默认发货仓库">
                    <el-select v-model="form.defaultWarehouseId" clearable filterable style="width: 100%">
                      <el-option v-for="w in warehouses" :key="w.id" :label="w.name" :value="w.id" />
                    </el-select>
                  </el-form-item>
                </el-col>
                <el-col :span="8">
                  <el-form-item label="分值系数">
                    <el-input-number v-model="form.scoreFactor" :min="0" :step="0.1" :controls="false" style="width: 100%" />
                  </el-form-item>
                </el-col>
                <el-col :span="8">
                  <el-form-item label="状态">
                    <el-select v-model="form.status" style="width: 100%">
                      <el-option :value="1" label="启用" />
                      <el-option :value="0" label="停用" />
                    </el-select>
                  </el-form-item>
                </el-col>
                <el-col :span="16">
                  <el-form-item label="商品名称" required>
                    <el-input v-model="form.name" @blur="fillSkuFromParent" />
                  </el-form-item>
                </el-col>
                <el-col :span="8">
                  <el-form-item label="结构类型">
                    <el-select v-model="form.defaultProductType" style="width: 100%">
                      <el-option label="普通" value="normal" />
                      <el-option label="组合品" value="combo" />
                      <el-option label="组装品" value="assembly" />
                    </el-select>
                  </el-form-item>
                </el-col>
                <el-col :span="24">
                  <el-form-item label="备注">
                    <el-input v-model="form.remark" type="textarea" :rows="2" />
                  </el-form-item>
                </el-col>
              </el-row>
            </el-col>
          </el-row>
        </el-form>

        <div class="section-title sku-title">
          <span>库存SKU明细</span>
          <el-button type="primary" link :icon="Plus" @click="addSkuRow">添加</el-button>
        </div>
        <el-table :data="skus" border size="small" class="sku-grid">
          <el-table-column label="图片" width="72" fixed>
            <template #default="{ row }">
              <ImageField v-model="row.pic" tip="SKU图" subdir="skus" compact :size="48" />
            </template>
          </el-table-column>
          <el-table-column label="库存SKU" min-width="120" fixed>
            <template #default="{ row }"><el-input v-model="row.skuCode" size="small" /></template>
          </el-table-column>
          <el-table-column label="商品类型" width="110">
            <template #default="{ row }">
              <el-select v-model="row.goodsKind" size="small" style="width: 100%">
                <el-option v-for="o in goodsKindOptions" :key="o.value" :label="o.label" :value="o.value" />
              </el-select>
            </template>
          </el-table-column>
          <el-table-column label="配货名称" min-width="110">
            <template #default="{ row }"><el-input v-model="row.pickName" size="small" /></template>
          </el-table-column>
          <el-table-column label="款式1" width="80">
            <template #default="{ row }"><el-input v-model="row.style1" size="small" /></template>
          </el-table-column>
          <el-table-column label="款式2" width="80">
            <template #default="{ row }"><el-input v-model="row.style2" size="small" /></template>
          </el-table-column>
          <el-table-column label="款式3" width="80">
            <template #default="{ row }"><el-input v-model="row.style3" size="small" /></template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-select v-model="row.status" size="small" style="width: 100%">
                <el-option label="在售" value="active" />
                <el-option label="停用" value="inactive" />
                <el-option label="清仓" value="clearance" />
              </el-select>
            </template>
          </el-table-column>
          <el-table-column label="上次采购价" width="100">
            <template #default="{ row }">
              <el-input-number v-model="row.lastPurchasePrice" :min="0" :precision="2" :controls="false" size="small" style="width: 100%" />
            </template>
          </el-table-column>
          <el-table-column label="零售价" width="90">
            <template #default="{ row }">
              <el-input-number v-model="row.retailPrice" :min="0" :precision="2" :controls="false" size="small" style="width: 100%" />
            </template>
          </el-table-column>
          <el-table-column label="重量(g)" width="80">
            <template #default="{ row }">
              <el-input-number v-model="row.weightG" :min="0" :controls="false" size="small" style="width: 100%" />
            </template>
          </el-table-column>
          <el-table-column label="UPC" width="100">
            <template #default="{ row }"><el-input v-model="row.upc" size="small" /></template>
          </el-table-column>
          <el-table-column label="ASIN" width="100">
            <template #default="{ row }"><el-input v-model="row.asin" size="small" /></template>
          </el-table-column>
          <el-table-column label="供应商货号" width="110">
            <template #default="{ row }"><el-input v-model="row.supplierItemNo" size="small" /></template>
          </el-table-column>
          <el-table-column label="备注" min-width="90">
            <template #default="{ row }"><el-input v-model="row.description" size="small" /></template>
          </el-table-column>
          <el-table-column label="" width="50" fixed="right">
            <template #default="{ $index }">
              <el-button link type="danger" :icon="Delete" @click="removeSkuRow($index)" />
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 物流及报关 -->
      <el-tab-pane label="物流及报关信息" name="logistics">
        <el-form :model="form" label-width="120px" class="tab-form">
          <el-form-item label="商品特性">
            <el-input v-model="form.features" placeholder="按空格分隔多个特性" />
          </el-form-item>
          <el-row :gutter="12">
            <el-col :span="12">
              <el-form-item label="英文申报名"><el-input v-model="form.aliasEn" /></el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="中文申报名"><el-input v-model="form.aliasCn" /></el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="申报重量(g)">
                <el-input-number v-model="form.declareWeightG" :min="0" :precision="1" :controls="false" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="申报价值(USD)">
                <el-input-number v-model="form.declaredValue" :min="0" :precision="4" :controls="false" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="出口国申报价">
                <el-input-number v-model="form.exportDeclaredValue" :min="0" :precision="4" :controls="false" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="原产国"><el-input v-model="form.originCountryCode" placeholder="如 CN / JP" /></el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="海关编码"><el-input v-model="form.hsCode" /></el-form-item>
            </el-col>
          </el-row>
        </el-form>
      </el-tab-pane>

      <!-- 采购及供应商 -->
      <el-tab-pane label="采购及供应商信息" name="purchase">
        <div class="section-title">采购信息</div>
        <el-form :model="form" label-width="130px" class="tab-form">
          <el-row :gutter="12">
            <el-col :span="12">
              <el-form-item label="采购渠道"><el-input v-model="form.purchaseChannel" /></el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="采购员"><el-input v-model="form.purchaser" /></el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="最低采购价(CNY)">
                <el-input-number v-model="form.minPurchasePrice" :min="0" :precision="4" :controls="false" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="采购最小订货量">
                <el-input-number v-model="form.stockMinAmount" :min="0" :controls="false" style="width: 100%" />
              </el-form-item>
            </el-col>
          </el-row>
        </el-form>
        <div class="section-title sku-title">
          <span>多供应商信息</span>
          <el-button type="primary" link :icon="Plus" @click="openSupplierPicker">添加</el-button>
        </div>
        <el-table :data="suppliers" border size="small" empty-text="点击「添加」搜索并选择供应商">
          <el-table-column label="供应商名称" min-width="200">
            <template #default="{ row }">
              <div>{{ row.supplierName || '—' }}</div>
              <div v-if="row.supplierCode" class="supplier-code">{{ row.supplierCode }}</div>
              <div v-if="row.isDefault" class="default-supplier-tag">默认供应商</div>
            </template>
          </el-table-column>
          <el-table-column label="供应商报价(￥)" width="130">
            <template #default="{ row }">
              <el-input-number v-model="row.price" :min="0" :precision="4" :controls="false" size="small" style="width: 100%" />
            </template>
          </el-table-column>
          <el-table-column label="进货说明" min-width="120">
            <template #default="{ row }"><el-input v-model="row.remark" size="small" /></template>
          </el-table-column>
          <el-table-column label="联系人" width="110">
            <template #default="{ row }"><el-input v-model="row.contactName" size="small" /></template>
          </el-table-column>
          <el-table-column label="联系电话" width="130">
            <template #default="{ row }"><el-input v-model="row.phone" size="small" /></template>
          </el-table-column>
          <el-table-column label="操作" width="140" fixed="right">
            <template #default="{ $index }">
              <el-button link type="primary" size="small" @click="setDefaultSupplier($index)">设默认</el-button>
              <el-button link type="danger" size="small" :icon="Delete" @click="removeSupplierRow($index)" />
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 包装信息 -->
      <el-tab-pane label="包装信息" name="pack">
        <el-form :model="form" label-width="120px" class="tab-form">
          <el-row :gutter="12">
            <el-col :span="8">
              <el-form-item label="外包装规格">
                <el-select v-model="form.packSpecId" clearable filterable style="width: 100%" @change="onPackSpecChange">
                  <el-option v-for="p in packSpecs" :key="p.id" :label="p.name" :value="p.id" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="内包装成本">
                <el-input-number v-model="form.packFee" :min="0" :precision="4" :controls="false" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="最小包装数">
                <el-input-number v-model="form.packageCount" :min="0" :controls="false" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8"><el-form-item label="外箱长(cm)"><el-input-number v-model="form.outLong" :min="0" :precision="2" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="外箱宽(cm)"><el-input-number v-model="form.outWide" :min="0" :precision="2" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="外箱高(cm)"><el-input-number v-model="form.outHigh" :min="0" :precision="2" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="外箱毛重(kg)"><el-input-number v-model="form.outGrossWeight" :min="0" :precision="3" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="外箱净重(kg)"><el-input-number v-model="form.outNetWeight" :min="0" :precision="3" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="8" />
            <el-col :span="8"><el-form-item label="内盒长(cm)"><el-input-number v-model="form.inLong" :min="0" :precision="2" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="内盒宽(cm)"><el-input-number v-model="form.inWide" :min="0" :precision="2" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="内盒高(cm)"><el-input-number v-model="form.inHigh" :min="0" :precision="2" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="内盒毛重(kg)"><el-input-number v-model="form.inGrossWeight" :min="0" :precision="3" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="内盒净重(kg)"><el-input-number v-model="form.inNetWeight" :min="0" :precision="3" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="24">
              <el-form-item label="包装事项"><el-input v-model="form.packMsg" type="textarea" :rows="2" /></el-form-item>
            </el-col>
          </el-row>
        </el-form>
      </el-tab-pane>

      <!-- 销售信息 -->
      <el-tab-pane label="销售信息" name="sale">
        <div class="section-title">销售基础信息</div>
        <el-form :model="form" label-width="110px" class="tab-form">
          <el-row :gutter="12">
            <el-col :span="12"><el-form-item label="店铺名称"><el-input v-model="form.shopTitle" /></el-form-item></el-col>
            <el-col :span="12"><el-form-item label="品牌"><el-input v-model="form.brand" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="规格"><el-input v-model="form.specClass" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="型号"><el-input v-model="form.model" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="材质"><el-input v-model="form.material" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="款式"><el-input v-model="form.style" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="季节"><el-input v-model="form.season" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="单位"><el-input v-model="form.unit" placeholder="如 PCS" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="零售价格"><el-input-number v-model="form.retailPrice" :min="0" :precision="4" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="批发价格"><el-input-number v-model="form.batchPrice" :min="0" :precision="4" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="最高售价"><el-input-number v-model="form.maxSalePrice" :min="0" :precision="4" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="最低售价"><el-input-number v-model="form.minSalePrice" :min="0" :precision="4" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="市场参考价"><el-input-number v-model="form.marketPrice" :min="0" :precision="4" :controls="false" style="width: 100%" /></el-form-item></el-col>
          </el-row>
        </el-form>
        <div class="section-title sku-title">
          <span>多语言商品描述</span>
          <el-button type="primary" link :icon="Plus" @click="addDescriptionRow">添加</el-button>
        </div>
        <el-table :data="descriptions" border size="small" empty-text="点击「添加」填写多语言标题与描述">
          <el-table-column label="语言" width="150">
            <template #default="{ row }">
              <el-select
                :model-value="row.languageCode"
                placeholder="选择语言"
                filterable
                size="small"
                style="width: 100%"
                @change="(v: string) => onLanguageChange(row, v)"
              >
                <el-option
                  v-for="o in languageOptions"
                  :key="o.code"
                  :label="o.name"
                  :value="o.code"
                  :disabled="descriptions.some((d) => d !== row && d.languageCode === o.code)"
                />
              </el-select>
            </template>
          </el-table-column>
          <el-table-column label="标题" min-width="180">
            <template #default="{ row }"><el-input v-model="row.title" size="small" /></template>
          </el-table-column>
          <el-table-column label="描述" min-width="280">
            <template #default="{ row }">
              <el-input v-model="row.description" type="textarea" :rows="2" size="small" />
            </template>
          </el-table-column>
          <el-table-column label="操作" width="70" fixed="right">
            <template #default="{ $index }">
              <el-button link type="danger" size="small" :icon="Delete" @click="removeDescriptionRow($index)" />
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存</el-button>
    </template>
  </el-dialog>

  <SupplierPickerDialog
    v-model="supplierPickerVisible"
    :exclude-ids="selectedSupplierIds"
    @select="onSupplierSelected"
  />
</template>

<style scoped>
.section-title {
  font-weight: 600;
  margin: 4px 0 12px;
  padding-left: 8px;
  border-left: 3px solid var(--el-color-primary);
}
.sku-title {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 8px;
}
.tab-form { max-width: 1000px; }
.sku-grid :deep(.el-table__cell) {
  padding: 8px 4px;
  vertical-align: middle;
}
.sku-grid :deep(.cell) {
  overflow: visible;
  line-height: normal;
}
.default-supplier-tag {
  margin-top: 4px;
  display: inline-block;
  font-size: 10px;
  color: #ff9900;
  border: 1px solid #ff9900;
  padding: 0 5px;
  height: 16px;
  line-height: 15px;
}
.supplier-code {
  margin-top: 2px;
  font-size: 12px;
  color: #909399;
}
</style>
