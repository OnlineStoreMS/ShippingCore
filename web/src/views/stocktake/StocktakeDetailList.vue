<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '../../api/wms'

const loading = ref(false)
const list = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')
const warehouseId = ref<number | undefined>()
const status = ref<string | undefined>()
const warehouses = ref<any[]>([])
const dateRange = ref<[string, string] | null>(null)

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
    const res = await api.listStocktakeDetails({
      page: page.value,
      pageSize: pageSize.value,
      keyword: keyword.value || undefined,
      warehouseId: warehouseId.value,
      status: status.value || undefined,
      from: dateRange.value?.[0],
      to: dateRange.value?.[1],
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
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header><span>盘点明细表</span></template>
      <div class="toolbar">
        <el-date-picker
          v-model="dateRange"
          type="daterange"
          value-format="YYYY-MM-DD"
          start-placeholder="起始日期"
          end-placeholder="结束日期"
          clearable
          style="width: 260px"
          @change="search"
        />
        <el-select v-model="warehouseId" clearable filterable placeholder="盘点仓库" style="width: 180px" @change="search">
          <el-option v-for="w in warehouses" :key="w.id" :label="w.name" :value="w.id" />
        </el-select>
        <el-select v-model="status" clearable placeholder="单据状态（默认全部）" style="width: 180px" @change="search">
          <el-option label="未审核/盘点中" value="open" />
          <el-option label="已审核/已盘点" value="review" />
          <el-option label="已完结" value="posted" />
          <el-option label="已作废" value="cancelled" />
        </el-select>
        <el-input v-model="keyword" clearable placeholder="盘点单号/SKU" style="width: 200px" @keyup.enter="search" />
        <el-button type="primary" @click="search">查询</el-button>
      </div>
      <el-alert
        class="hint"
        type="info"
        :closable="false"
        show-icon
        title="本页按「盘点单商品行」汇总查询：在「仓库盘点单」中保存/审核后才会出现明细。筛选状态请与盘点单一致（已完结选「已完结」，不要选「未审核/盘点中」）。"
      />
      <el-table :data="list" border stripe>
        <el-table-column prop="docNo" label="盘点单号" width="150" fixed show-overflow-tooltip />
        <el-table-column label="制单日期" width="170">
          <template #default="{ row }">{{ fmtTime(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column label="单据状态" width="120">
          <template #default="{ row }">{{ statusMap[row.status] || row.status || '-' }}</template>
        </el-table-column>
        <el-table-column prop="skuCode" label="库存SKU" width="130" show-overflow-tooltip />
        <el-table-column prop="pickName" label="配货名称" min-width="130" show-overflow-tooltip />
        <el-table-column prop="style1" label="款式1" width="90" show-overflow-tooltip />
        <el-table-column prop="style2" label="款式2" width="90" show-overflow-tooltip />
        <el-table-column prop="style3" label="款式3" width="90" show-overflow-tooltip />
        <el-table-column prop="specClass" label="规格" width="90" show-overflow-tooltip />
        <el-table-column prop="model" label="型号" width="90" show-overflow-tooltip />
        <el-table-column prop="warehouseName" label="仓库" width="120" show-overflow-tooltip />
        <el-table-column prop="locationCode" label="库位" width="100" />
        <el-table-column prop="unit" label="单位" width="70" />
        <el-table-column label="盘点成本价" width="110" align="right">
          <template #default="{ row }">{{ fmtNum(row.unitCost) }}</template>
        </el-table-column>
        <el-table-column label="账存数量" width="100" align="right">
          <template #default="{ row }">{{ fmtNum(row.bookQty, 0) }}</template>
        </el-table-column>
        <el-table-column label="实盘数量" width="100" align="right">
          <template #default="{ row }">{{ fmtNum(row.countQty, 0) }}</template>
        </el-table-column>
        <el-table-column label="差额数量" width="100" align="right">
          <template #default="{ row }">{{ fmtNum(row.diffQty, 0) }}</template>
        </el-table-column>
        <el-table-column label="差额金额" width="110" align="right">
          <template #default="{ row }">{{ fmtNum(row.diffAmount) }}</template>
        </el-table-column>
        <el-table-column label="盘点金额" width="110" align="right">
          <template #default="{ row }">{{ fmtNum(row.countAmount) }}</template>
        </el-table-column>
        <el-table-column prop="orderRemark" label="盘点单备注" min-width="120" show-overflow-tooltip />
        <el-table-column prop="remark" label="商品备注" min-width="120" show-overflow-tooltip />
        <el-table-column label="审核时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.postedAt) }}</template>
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
  </div>
</template>

<style scoped>
.toolbar { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 12px; }
.hint { margin-bottom: 12px; }
.pager { margin-top: 16px; justify-content: flex-end; }
</style>
