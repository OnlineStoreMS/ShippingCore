<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Plus } from '@element-plus/icons-vue'
import { api } from '../api/wms'
import { useSessionStore } from '../stores/session'
import SkuPickerDialog from './SkuPickerDialog.vue'

const props = defineProps<{
  modelValue: boolean
  /** 编辑时传入盘点单 id；新建不传 */
  stocktakeId?: number | null
  warehouses: any[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'saved'): void
}>()

const sessionStore = useSessionStore()
const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const loading = ref(false)
const saving = ref(false)
const orderId = ref<number | null>(null)
const status = ref('draft')
const form = ref({
  warehouseId: undefined as number | undefined,
  checkerName: '',
  remark: '',
})
const items = ref<LocalItem[]>([])
const selectedItems = ref<LocalItem[]>([])
const skuPickerVisible = ref(false)
const tableRef = ref<any>()

type LocalItem = {
  _key: string
  id?: number
  invSkuId: number
  skuCode: string
  pickName: string
  pic?: string
  locationId: number
  locationCode: string
  bookQty: number
  countQty: number
  unitCost: number
  remark: string
  style1?: string
  style2?: string
  style3?: string
}

const statusMap: Record<string, string> = {
  draft: '未审核',
  counting: '盘点中',
  review: '已审核/已盘点',
  posted: '已完结',
  cancelled: '已作废',
}

const canEdit = computed(() => ['draft', 'counting'].includes(status.value))
const isNew = computed(() => !orderId.value)
const dialogTitle = computed(() => (isNew.value ? '新增盘点单' : `编辑盘点单 ${statusMap[status.value] || ''}`))
const excludeSkuIds = computed(() => items.value.map((i) => i.invSkuId))

const stepActive = computed(() => {
  if (status.value === 'posted') return 3
  if (status.value === 'review') return 2
  return 1
})

function fmtNum(v?: number, digits = 2) {
  if (v == null || Number.isNaN(Number(v))) return '-'
  return Number(v).toFixed(digits)
}

function diffOf(row: LocalItem) {
  return (Number(row.countQty) || 0) - (Number(row.bookQty) || 0)
}

function genKey(invSkuId: number, locationId: number) {
  return `${invSkuId}-${locationId}-${Math.random().toString(36).slice(2, 8)}`
}

async function resetNew() {
  orderId.value = null
  status.value = 'draft'
  form.value = {
    warehouseId: props.warehouses[0]?.id,
    checkerName: sessionStore.session?.user.displayName || '',
    remark: '',
  }
  items.value = []
  selectedItems.value = []
}

async function loadExisting(id: number) {
  loading.value = true
  try {
    const detail = await api.getStocktake(id)
    orderId.value = detail.id
    status.value = detail.status
    form.value = {
      warehouseId: detail.warehouseId,
      checkerName: detail.checkerName || '',
      remark: detail.remark || '',
    }
    items.value = (detail.items || []).map((it: any) => ({
      _key: genKey(it.invSkuId, it.locationId),
      id: it.id,
      invSkuId: it.invSkuId,
      skuCode: it.skuCode,
      pickName: it.pickName,
      locationId: it.locationId,
      locationCode: it.locationCode || '',
      bookQty: Math.round(Number(it.bookQty) || 0),
      countQty: Math.round(Number(it.countQty) || 0),
      unitCost: Number(it.unitCost) || 0,
      remark: it.remark || '',
      style1: it.style1,
      style2: it.style2,
      style3: it.style3,
    }))
  } catch (e) {
    ElMessage.error((e as Error).message || '加载盘点单失败')
    visible.value = false
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  async (open) => {
    if (!open) return
    await sessionStore.load()
    if (props.stocktakeId) {
      await loadExisting(props.stocktakeId)
    } else {
      await resetNew()
    }
  },
)

function openAddSku() {
  if (!form.value.warehouseId) {
    ElMessage.warning('请先选择盘点仓库')
    return
  }
  if (!canEdit.value) {
    ElMessage.warning('当前状态不可添加商品')
    return
  }
  skuPickerVisible.value = true
}

async function onSkusPicked(skus: any[]) {
  if (!form.value.warehouseId || !skus.length) return
  loading.value = true
  try {
    for (const sku of skus) {
      if (items.value.some((i) => i.invSkuId === sku.id)) continue
      let bals: any[] = []
      try {
        const res = await api.stockBalances({
          warehouseId: form.value.warehouseId,
          invSkuId: sku.id,
          page: 1,
          pageSize: 50,
        })
        bals = res.list || []
      } catch {
        bals = []
      }
      if (!bals.length) {
        items.value.push({
          _key: genKey(sku.id, 0),
          invSkuId: sku.id,
          skuCode: sku.skuCode,
          pickName: sku.pickName || '',
          pic: sku.pic,
          locationId: 0,
          locationCode: '',
          bookQty: 0,
          countQty: 0,
          unitCost: Number(sku.lastPurchasePrice) || 0,
          remark: '',
          style1: sku.style1,
          style2: sku.style2,
          style3: sku.style3,
        })
      } else {
        for (const b of bals) {
          if (items.value.some((i) => i.invSkuId === sku.id && i.locationId === b.locationId)) continue
          items.value.push({
            _key: genKey(sku.id, b.locationId),
            invSkuId: sku.id,
            skuCode: sku.skuCode || b.skuCode,
            pickName: sku.pickName || b.pickName || '',
            pic: sku.pic,
            locationId: b.locationId,
            locationCode: b.locationCode || '',
            bookQty: Math.round(Number(b.onHand) || 0),
            countQty: Math.round(Number(b.onHand) || 0),
            unitCost: Number(b.unitCost ?? b.lastCost ?? sku.lastPurchasePrice) || 0,
            remark: '',
            style1: sku.style1,
            style2: sku.style2,
            style3: sku.style3,
          })
        }
      }
    }
    ElMessage.success(`已添加 ${skus.length} 个SKU`)
  } finally {
    loading.value = false
  }
}

function onSelectionChange(rows: LocalItem[]) {
  selectedItems.value = rows
}

async function removeSelected() {
  if (!selectedItems.value.length) {
    ElMessage.warning('请先勾选要删除的商品')
    return
  }
  await ElMessageBox.confirm(`确认删除选中的 ${selectedItems.value.length} 行商品？`, '删除商品', { type: 'warning' })
  const keys = new Set(selectedItems.value.map((r) => r._key))
  const toDeleteServer = selectedItems.value.filter((r) => r.id)
  if (orderId.value && toDeleteServer.length) {
    for (const row of toDeleteServer) {
      await api.deleteStocktakeItem(orderId.value, row.id!)
    }
  }
  items.value = items.value.filter((r) => !keys.has(r._key))
  selectedItems.value = []
  ElMessage.success('已删除')
}

async function ensureOrderSaved(): Promise<number> {
  if (!form.value.warehouseId) {
    throw new Error('请选择盘点仓库')
  }
  if (orderId.value) {
    await api.updateStocktake(orderId.value, {
      checkerName: form.value.checkerName || '',
      remark: form.value.remark || '',
    })
    return orderId.value
  }
  const { data } = await api.createStocktake({
    warehouseId: form.value.warehouseId,
    checkerName: form.value.checkerName || '',
    remark: form.value.remark || '',
    fillAllBalances: false,
  })
  const id = data?.data?.id
  if (!id) throw new Error('创建盘点单失败')
  orderId.value = id
  status.value = data?.data?.status || 'draft'
  return id
}

async function syncItems(id: number) {
  const pending = items.value.filter((i) => !i.id)
  if (pending.length) {
    await api.addStocktakeItems(id, {
      items: pending.map((i) => ({
        invSkuId: i.invSkuId,
        locationId: i.locationId || 0,
        countQty: Math.round(Number(i.countQty) || 0),
        remark: i.remark || '',
      })),
    })
  }
  const detail = await api.getStocktake(id)
  status.value = detail.status
  const serverItems = detail.items || []
  const countPayload = serverItems.map((si: any) => {
    const local = items.value.find(
      (l) => l.invSkuId === si.invSkuId && (l.locationId || 0) === (si.locationId || 0),
    )
    return {
      id: si.id,
      countQty: local ? Math.round(Number(local.countQty) || 0) : Math.round(Number(si.countQty) || 0),
      remark: local?.remark ?? si.remark ?? '',
    }
  })
  if (countPayload.length && ['draft', 'counting'].includes(detail.status)) {
    await api.saveStocktakeCounts(id, { items: countPayload })
  }
  await loadExisting(id)
}

async function save(andPost = false) {
  if (!form.value.warehouseId) {
    ElMessage.warning('请选择盘点仓库')
    return
  }
  if (!items.value.length) {
    ElMessage.warning('请先添加盘点商品')
    return
  }
  saving.value = true
  try {
    const id = await ensureOrderSaved()
    await syncItems(id)
    if (andPost) {
      await ElMessageBox.confirm('确认仓库审核过账？将按盘点差异调整库存。', '仓库审核', { type: 'warning' })
      await api.postStocktake(id)
      ElMessage.success('已保存并仓库审核')
      await loadExisting(id)
    } else {
      ElMessage.success('已保存')
    }
    emit('saved')
  } catch (e) {
    if (e === 'cancel') return
    ElMessage.error((e as Error).message || '保存失败')
  } finally {
    saving.value = false
  }
}

function close() {
  visible.value = false
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="dialogTitle"
    width="1180px"
    top="4vh"
    destroy-on-close
    :close-on-click-modal="false"
    class="stocktake-form-dialog"
  >
    <div v-loading="loading" class="body">
      <el-steps :active="stepActive" align-center simple class="steps">
        <el-step title="编辑盘点单" description="仅未审核状态可编辑" />
        <el-step title="仓库审核" description="审核后库存数量变化" />
        <el-step title="完结" description="盘点完结" />
      </el-steps>

      <div class="section">
        <div class="section-title">基础信息</div>
        <el-form :model="form" label-width="90px" class="base-form">
          <el-form-item label="盘点仓库" required>
            <el-select
              v-model="form.warehouseId"
              filterable
              placeholder="请选择盘点仓库"
              style="width: 280px"
              :disabled="!canEdit || !isNew"
            >
              <el-option v-for="w in warehouses" :key="w.id" :label="w.name" :value="w.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="盘点人">
            <el-input v-model="form.checkerName" :disabled="!canEdit" style="width: 220px" placeholder="盘点人" />
          </el-form-item>
          <el-form-item label="备注" style="flex: 1; min-width: 280px">
            <el-input v-model="form.remark" :disabled="!canEdit" placeholder="盘点备注" />
          </el-form-item>
          <el-form-item v-if="!isNew" label="状态">
            <el-tag>{{ statusMap[status] || status }}</el-tag>
          </el-form-item>
        </el-form>
      </div>

      <div class="section">
        <div class="section-hdr">
          <div class="section-title">商品信息</div>
          <div v-if="canEdit" class="section-actions">
            <el-button type="primary" :icon="Plus" @click="openAddSku">添加商品</el-button>
            <el-button type="danger" plain :icon="Delete" :disabled="!selectedItems.length" @click="removeSelected">
              删除商品
            </el-button>
          </div>
        </div>

        <el-table
          ref="tableRef"
          :data="items"
          border
          stripe
          size="small"
          max-height="420"
          row-key="_key"
          @selection-change="onSelectionChange"
        >
          <el-table-column v-if="canEdit" type="selection" width="48" />
          <el-table-column label="图片" width="56" align="center">
            <template #default="{ row }">
              <el-image
                v-if="row.pic"
                :src="row.pic"
                fit="cover"
                style="width: 32px; height: 32px"
                :preview-src-list="[row.pic]"
                preview-teleported
              />
              <span v-else class="muted">-</span>
            </template>
          </el-table-column>
          <el-table-column prop="skuCode" label="库存SKU" width="130" show-overflow-tooltip />
          <el-table-column prop="pickName" label="配货名称" min-width="130" show-overflow-tooltip />
          <el-table-column prop="locationCode" label="库位" width="100" show-overflow-tooltip>
            <template #default="{ row }">{{ row.locationCode || '-' }}</template>
          </el-table-column>
          <el-table-column label="账存数量" width="100" align="right">
            <template #default="{ row }">{{ fmtNum(row.bookQty, 0) }}</template>
          </el-table-column>
          <el-table-column label="账存金额" width="100" align="right">
            <template #default="{ row }">{{ fmtNum(row.bookQty * row.unitCost) }}</template>
          </el-table-column>
          <el-table-column label="实盘数量" width="130">
            <template #default="{ row }">
              <el-input-number
                v-if="canEdit"
                v-model="row.countQty"
                :min="0"
                :step="1"
                :precision="0"
                :controls="false"
                style="width: 110px"
              />
              <span v-else>{{ fmtNum(row.countQty, 0) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="差额数量" width="100" align="right">
            <template #default="{ row }">
              <span :class="{ neg: diffOf(row) < 0, pos: diffOf(row) > 0 }">{{ fmtNum(diffOf(row), 0) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="差额金额" width="100" align="right">
            <template #default="{ row }">{{ fmtNum(diffOf(row) * row.unitCost) }}</template>
          </el-table-column>
          <el-table-column prop="style1" label="款式1" width="90" show-overflow-tooltip />
          <el-table-column label="备注" min-width="120">
            <template #default="{ row }">
              <el-input v-if="canEdit" v-model="row.remark" size="small" />
              <span v-else>{{ row.remark || '-' }}</span>
            </template>
          </el-table-column>
        </el-table>
        <div v-if="!items.length" class="empty">请点击「添加商品」弹出选择库存SKU</div>
      </div>
    </div>

    <template #footer>
      <div class="footer">
        <el-button @click="close">关闭</el-button>
        <template v-if="canEdit">
          <el-button type="primary" :loading="saving" @click="save(false)">保存</el-button>
          <el-button type="warning" :loading="saving" @click="save(true)">保存并仓库审核</el-button>
        </template>
      </div>
    </template>
  </el-dialog>

  <SkuPickerDialog v-model="skuPickerVisible" :exclude-ids="excludeSkuIds" multiple title="添加商品" @confirm="onSkusPicked" />
</template>

<style scoped>
.body { min-height: 360px; }
.steps { margin-bottom: 16px; }
.section { margin-bottom: 16px; }
.section-title {
  font-weight: 600;
  margin-bottom: 10px;
  padding-left: 8px;
  border-left: 3px solid var(--el-color-primary);
}
.section-hdr {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}
.section-hdr .section-title { margin-bottom: 0; }
.section-actions { display: flex; gap: 8px; }
.base-form {
  display: flex;
  flex-wrap: wrap;
  gap: 0 12px;
}
.empty {
  text-align: center;
  color: #909399;
  padding: 28px 0;
  border: 1px dashed #dcdfe6;
  border-top: none;
}
.muted { color: #bbb; }
.neg { color: #f56c6c; }
.pos { color: #67c23a; }
.footer { display: flex; justify-content: flex-end; gap: 8px; width: 100%; }
</style>
