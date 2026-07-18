<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { api } from '../../api/wms'
import { useSessionStore } from '../../stores/session'
import StocktakeFormDialog from '../../components/StocktakeFormDialog.vue'

const sessionStore = useSessionStore()
const loading = ref(false)
const list = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const warehouses = ref<any[]>([])
const statusTab = ref('all')
const warehouseId = ref<number | undefined>()
const keyword = ref('')
const formVisible = ref(false)
const editingId = ref<number | null>(null)
const detailVisible = ref(false)
const detail = ref<any>(null)

const statusMap: Record<string, string> = {
  draft: '未审核',
  counting: '盘点中',
  review: '已审核/已盘点',
  posted: '已完结',
  cancelled: '已作废',
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
    const res = await api.listStocktakes({
      page: page.value,
      pageSize: pageSize.value,
      status: statusTab.value === 'all' ? undefined : statusTab.value,
      warehouseId: warehouseId.value,
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
  await sessionStore.load()
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
  editingId.value = null
  formVisible.value = true
}

function openEdit(row: any) {
  editingId.value = row.id
  formVisible.value = true
}

async function showDetail(row: any) {
  try {
    detail.value = await api.getStocktake(row.id)
    detailVisible.value = true
  } catch (e) {
    ElMessage.error((e as Error).message || '加载明细失败')
  }
}

async function remove(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除盘点单 ${row.docNo}？删除后不可恢复。`, '删除确认', { type: 'warning' })
    await api.deleteStocktake(row.id)
    ElMessage.success('已删除')
    if (detailVisible.value && detail.value?.id === row.id) {
      detailVisible.value = false
      detail.value = null
    }
    await load()
  } catch (e) {
    if (e === 'cancel') return
    ElMessage.error((e as Error).message || '删除失败')
  }
}
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header>
        <div class="hdr">
          <span>仓库盘点单</span>
          <el-button type="primary" :icon="Plus" @click="openCreate">新增盘点单</el-button>
        </div>
      </template>

      <el-tabs v-model="statusTab" @tab-change="onTabChange">
        <el-tab-pane label="全部" name="all" />
        <el-tab-pane label="未审核/盘点中" name="open" />
        <el-tab-pane label="已审核/已盘点" name="review" />
        <el-tab-pane label="已完结" name="posted" />
        <el-tab-pane label="已作废" name="cancelled" />
      </el-tabs>

      <div class="toolbar">
        <el-select v-model="warehouseId" clearable filterable placeholder="盘点仓库" style="width: 180px" @change="search">
          <el-option v-for="w in warehouses" :key="w.id" :label="w.name" :value="w.id" />
        </el-select>
        <el-input v-model="keyword" clearable placeholder="盘点单号" style="width: 200px" @keyup.enter="search" />
        <el-button type="primary" @click="search">查询</el-button>
      </div>

      <el-table :data="list" border stripe>
        <el-table-column prop="docNo" label="盘点单号" width="160" fixed>
          <template #default="{ row }">
            <el-link type="primary" @click="openEdit(row)">{{ row.docNo }}</el-link>
          </template>
        </el-table-column>
        <el-table-column label="盘点仓库" width="140" show-overflow-tooltip>
          <template #default="{ row }">{{ row.warehouseName || '-' }}</template>
        </el-table-column>
        <el-table-column prop="checkerName" label="盘点人" width="120" show-overflow-tooltip />
        <el-table-column label="盘点单状态" width="120">
          <template #default="{ row }">{{ statusMap[row.status] || row.status }}</template>
        </el-table-column>
        <el-table-column label="账存数量" width="100" align="right">
          <template #default="{ row }">{{ fmtNum(row.totalBookQty, 0) }}</template>
        </el-table-column>
        <el-table-column label="实盘数量" width="100" align="right">
          <template #default="{ row }">{{ fmtNum(row.totalCountQty, 0) }}</template>
        </el-table-column>
        <el-table-column label="差额数量" width="100" align="right">
          <template #default="{ row }">{{ fmtNum(row.totalDiffQty, 0) }}</template>
        </el-table-column>
        <el-table-column prop="remark" label="盘点备注" min-width="140" show-overflow-tooltip />
        <el-table-column label="制单时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column label="审核时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.postedAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="showDetail(row)">明细</el-button>
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button v-if="row.status !== 'posted'" link type="danger" @click="remove(row)">删除</el-button>
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

    <StocktakeFormDialog
      v-model="formVisible"
      :stocktake-id="editingId"
      :warehouses="warehouses"
      @saved="load"
    />

    <el-drawer v-model="detailVisible" title="盘点单商品明细" size="720px">
      <template v-if="detail">
        <el-descriptions :column="2" border class="mb">
          <el-descriptions-item label="盘点单号">{{ detail.docNo }}</el-descriptions-item>
          <el-descriptions-item label="盘点仓库">{{ detail.warehouseName || '-' }}</el-descriptions-item>
          <el-descriptions-item label="盘点人">{{ detail.checkerName || '-' }}</el-descriptions-item>
          <el-descriptions-item label="状态">{{ statusMap[detail.status] || detail.status }}</el-descriptions-item>
          <el-descriptions-item label="备注" :span="2">{{ detail.remark || '-' }}</el-descriptions-item>
        </el-descriptions>
        <el-table :data="detail.items || []" border stripe size="small" max-height="520">
          <el-table-column prop="skuCode" label="库存SKU" width="120" show-overflow-tooltip />
          <el-table-column prop="pickName" label="配货名称" min-width="120" show-overflow-tooltip />
          <el-table-column prop="locationCode" label="库位" width="90" />
          <el-table-column label="账存数量" width="90" align="right">
            <template #default="{ row }">{{ fmtNum(row.bookQty, 0) }}</template>
          </el-table-column>
          <el-table-column label="账存金额" width="100" align="right">
            <template #default="{ row }">{{ fmtNum(row.bookAmount) }}</template>
          </el-table-column>
          <el-table-column label="实盘数量" width="90" align="right">
            <template #default="{ row }">{{ fmtNum(row.countQty, 0) }}</template>
          </el-table-column>
          <el-table-column label="差额数量" width="90" align="right">
            <template #default="{ row }">{{ fmtNum(row.diffQty, 0) }}</template>
          </el-table-column>
          <el-table-column label="差额金额" width="100" align="right">
            <template #default="{ row }">{{ fmtNum(row.diffAmount) }}</template>
          </el-table-column>
          <el-table-column prop="remark" label="备注" min-width="100" show-overflow-tooltip />
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
