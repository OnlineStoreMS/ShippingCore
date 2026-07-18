<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { api } from '../../api/wms'
import SkuSearchSelect from '../../components/SkuSearchSelect.vue'
import LocationSelect from '../../components/LocationSelect.vue'

const loading = ref(false)
const list = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const warehouses = ref<any[]>([])
const statusTab = ref('all')
const warehouseId = ref<number | undefined>()
const reason = ref<string | undefined>()
const keyword = ref('')
const visible = ref(false)
const form = ref<any>({})
const detailVisible = ref(false)
const detail = ref<any>(null)

const statusMap: Record<string, string> = {
  draft: '未审核',
  posted: '已过账',
  cancelled: '已作废',
}

const reasonMap: Record<string, string> = {
  damage: '报损',
  sample: '样品',
  usage: '领用',
  adjust: '调整',
}

function fmtTime(v?: string) {
  if (!v) return '-'
  return String(v).replace('T', ' ').slice(0, 19)
}

function fmtNum(v?: number, digits = 2) {
  if (v == null || Number.isNaN(Number(v))) return '-'
  return Number(v).toFixed(digits)
}

async function loadWarehouses() {
  const res = await api.listWarehouses({ page: 1, pageSize: 200 })
  warehouses.value = (res.list || []).filter((w: any) => w.status !== 0)
}

async function load() {
  loading.value = true
  try {
    const res = await api.listOtherOut({
      page: page.value,
      pageSize: pageSize.value,
      status: statusTab.value === 'all' ? undefined : statusTab.value,
      warehouseId: warehouseId.value,
      reason: reason.value || undefined,
      keyword: keyword.value || undefined,
    })
    list.value = res.list
    total.value = res.total
  } catch (e) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await loadWarehouses()
  await load()
})

function search() {
  page.value = 1
  load()
}

function onTabChange() {
  page.value = 1
  load()
}

function openCreate() {
  form.value = {
    warehouseId: warehouses.value[0]?.id,
    locationId: null,
    reason: 'damage',
    remark: '',
    items: [{ invSkuId: undefined, qty: 1 }],
  }
  visible.value = true
}

function addItem() {
  form.value.items.push({ invSkuId: undefined, qty: 1 })
}

function removeItem(idx: number) {
  form.value.items.splice(idx, 1)
}

async function create() {
  try {
    const body = {
      ...form.value,
      locationId: form.value.locationId || 0,
      items: (form.value.items || []).filter((i: any) => i.invSkuId),
    }
    if (!body.warehouseId) {
      ElMessage.warning('请选择仓库')
      return
    }
    if (!body.items.length) {
      ElMessage.warning('请添加明细')
      return
    }
    await api.createOtherOut(body)
    ElMessage.success('已创建')
    visible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '创建失败')
  }
}

async function showDetail(row: any) {
  try {
    detail.value = await api.getOtherOut(row.id)
    detailVisible.value = true
  } catch (e) {
    ElMessage.error((e as Error).message || '加载明细失败')
  }
}

async function post(row: any) {
  await ElMessageBox.confirm('确认仓库审核过账出库？', '提示')
  await api.postOtherOut(row.id)
  ElMessage.success('已过账')
  await load()
}

async function cancel(row: any) {
  await ElMessageBox.confirm('确认作废？', '提示', { type: 'warning' })
  await api.cancelOtherOut(row.id)
  ElMessage.success('已作废')
  await load()
}
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header>
        <div class="hdr">
          <span>其它出库单</span>
          <el-button type="primary" :icon="Plus" @click="openCreate">新增其它出库单</el-button>
        </div>
      </template>

      <el-tabs v-model="statusTab" @tab-change="onTabChange">
        <el-tab-pane label="全部" name="all" />
        <el-tab-pane label="未审核" name="draft" />
        <el-tab-pane label="已过账" name="posted" />
        <el-tab-pane label="已作废" name="cancelled" />
      </el-tabs>

      <div class="toolbar">
        <el-select v-model="warehouseId" clearable filterable placeholder="出库仓库" style="width: 180px" @change="search">
          <el-option v-for="w in warehouses" :key="w.id" :label="w.name" :value="w.id" />
        </el-select>
        <el-select v-model="reason" clearable placeholder="出库类别" style="width: 140px" @change="search">
          <el-option v-for="(label, key) in reasonMap" :key="key" :label="label" :value="key" />
        </el-select>
        <el-input v-model="keyword" clearable placeholder="出库单号" style="width: 180px" @keyup.enter="search" />
        <el-button type="primary" @click="search">查询</el-button>
      </div>

      <el-table :data="list" border stripe>
        <el-table-column prop="docNo" label="出库单号" width="160" fixed show-overflow-tooltip />
        <el-table-column label="出库仓库" width="140" show-overflow-tooltip>
          <template #default="{ row }">{{ row.warehouseName || '-' }}</template>
        </el-table-column>
        <el-table-column label="出库类别" width="100">
          <template #default="{ row }">{{ reasonMap[row.reason] || row.reason }}</template>
        </el-table-column>
        <el-table-column label="总数量" width="100" align="right">
          <template #default="{ row }">{{ fmtNum(row.totalQty, 0) }}</template>
        </el-table-column>
        <el-table-column label="总金额" width="110" align="right">
          <template #default="{ row }">{{ fmtNum(row.totalAmount) }}</template>
        </el-table-column>
        <el-table-column label="制单时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column label="审核时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.postedAt) }}</template>
        </el-table-column>
        <el-table-column prop="remark" label="出库备注" min-width="140" show-overflow-tooltip />
        <el-table-column label="出库单状态" width="100">
          <template #default="{ row }">{{ statusMap[row.status] || row.status }}</template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="showDetail(row)">明细</el-button>
            <el-button v-if="row.status === 'draft'" link type="primary" @click="post(row)">仓库审核</el-button>
            <el-button v-if="row.status === 'draft'" link type="danger" @click="cancel(row)">作废</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        class="pager"
        layout="total, sizes, prev, pager, next"
        :total="total"
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :page-sizes="[20, 50, 100]"
        @current-change="load"
        @size-change="search"
      />
    </el-card>

    <el-dialog v-model="visible" title="新增其它出库单" width="720px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="出库仓库" required>
          <el-select v-model="form.warehouseId" style="width: 100%" @change="form.locationId = null">
            <el-option v-for="w in warehouses" :key="w.id" :label="w.name" :value="w.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="库位">
          <LocationSelect v-model="form.locationId" :warehouse-id="form.warehouseId" />
        </el-form-item>
        <el-form-item label="出库类别">
          <el-select v-model="form.reason" style="width: 100%">
            <el-option v-for="(label, key) in reasonMap" :key="key" :label="label" :value="key" />
          </el-select>
        </el-form-item>
        <el-form-item label="出库备注"><el-input v-model="form.remark" /></el-form-item>
        <el-form-item label="明细">
          <el-table :data="form.items" border size="small" style="width: 100%">
            <el-table-column label="库存SKU" min-width="220">
              <template #default="{ row }">
                <SkuSearchSelect v-model="row.invSkuId" />
              </template>
            </el-table-column>
            <el-table-column label="出库数量" width="130">
              <template #default="{ row }">
                <el-input-number v-model="row.qty" :min="0.0001" :controls="false" style="width: 110px" />
              </template>
            </el-table-column>
            <el-table-column label="" width="70">
              <template #default="{ $index }">
                <el-button link type="danger" @click="removeItem($index)">移除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-button type="primary" link style="margin-top: 8px" @click="addItem">+ 添加</el-button>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="visible = false">取消</el-button>
        <el-button type="primary" @click="create">创建</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="detailVisible" title="出库单商品明细" size="720px">
      <template v-if="detail">
        <el-descriptions :column="2" border class="mb">
          <el-descriptions-item label="出库单号">{{ detail.docNo }}</el-descriptions-item>
          <el-descriptions-item label="出库仓库">{{ detail.warehouseName || '-' }}</el-descriptions-item>
          <el-descriptions-item label="出库类别">{{ reasonMap[detail.reason] || detail.reason }}</el-descriptions-item>
          <el-descriptions-item label="状态">{{ statusMap[detail.status] || detail.status }}</el-descriptions-item>
          <el-descriptions-item label="总数量">{{ fmtNum(detail.totalQty, 0) }}</el-descriptions-item>
          <el-descriptions-item label="总金额">{{ fmtNum(detail.totalAmount) }}</el-descriptions-item>
          <el-descriptions-item label="备注" :span="2">{{ detail.remark || '-' }}</el-descriptions-item>
        </el-descriptions>
        <el-table :data="detail.items || []" border stripe size="small" max-height="480">
          <el-table-column prop="skuCode" label="库存SKU" width="120" show-overflow-tooltip />
          <el-table-column prop="pickName" label="配货名称" min-width="120" show-overflow-tooltip />
          <el-table-column prop="qty" label="出库数量" width="90" align="right" />
          <el-table-column label="出库单价" width="90" align="right">
            <template #default="{ row }">{{ fmtNum(row.unitCost) }}</template>
          </el-table-column>
          <el-table-column label="出库金额" width="100" align="right">
            <template #default="{ row }">{{ fmtNum(row.amount) }}</template>
          </el-table-column>
          <el-table-column prop="brand" label="品牌" width="80" show-overflow-tooltip />
          <el-table-column prop="style1" label="款式1" width="80" show-overflow-tooltip />
          <el-table-column prop="remark" label="商品备注" min-width="100" show-overflow-tooltip />
        </el-table>
      </template>
    </el-drawer>
  </div>
</template>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.toolbar { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 12px; }
.pager { margin-top: 16px; justify-content: flex-end; }
.mb { margin-bottom: 16px; }
</style>
