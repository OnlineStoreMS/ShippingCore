<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '../../api/wms'

const loading = ref(false)
const list = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const warehouses = ref<any[]>([])
const warehouseId = ref<number | undefined>()
const skuCode = ref('')
const docNo = ref('')
const moveType = ref<string | undefined>()
const dateRange = ref<[string, string] | null>(null)

const moveTypeOptions = [
  { label: '其它入库', value: 'other_in' },
  { label: '其它出库', value: 'other_out' },
  { label: '调拨入库', value: 'transfer_in' },
  { label: '调拨出库', value: 'transfer_out' },
  { label: '盘盈', value: 'stocktake_gain' },
  { label: '盘亏', value: 'stocktake_loss' },
  { label: '采购入库', value: 'purchase_in' },
  { label: '销售出库', value: 'sale_out' },
]

const moveTypeMap: Record<string, string> = Object.fromEntries(
  moveTypeOptions.map((o) => [o.value, o.label]),
)

function fmtNum(v?: number, digits = 2) {
  if (v == null || Number.isNaN(v)) return '-'
  return Number(v).toFixed(digits)
}

function fmtTime(v?: string) {
  if (!v) return '-'
  return String(v).replace('T', ' ').slice(0, 19)
}

async function loadWarehouses() {
  const res = await api.listWarehouses({ page: 1, pageSize: 200 })
  warehouses.value = (res.list || []).filter((w: any) => w.status !== 0)
}

async function load() {
  loading.value = true
  try {
    const res = await api.stockMovements({
      page: page.value,
      pageSize: pageSize.value,
      warehouseId: warehouseId.value,
      skuCode: skuCode.value || undefined,
      moveType: moveType.value || undefined,
      docNo: docNo.value || undefined,
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
      <template #header><span>库存明细表</span></template>
      <div class="toolbar">
        <el-select v-model="warehouseId" clearable filterable placeholder="所在仓库" style="width: 180px" @change="search">
          <el-option v-for="w in warehouses" :key="w.id" :label="w.name" :value="w.id" />
        </el-select>
        <el-input v-model="skuCode" placeholder="库存SKU" clearable style="width: 150px" @change="search" />
        <el-input v-model="docNo" placeholder="单据号" clearable style="width: 150px" @change="search" />
        <el-select v-model="moveType" clearable placeholder="移动类型" style="width: 140px" @change="search">
          <el-option v-for="o in moveTypeOptions" :key="o.value" :label="o.label" :value="o.value" />
        </el-select>
        <el-date-picker
          v-model="dateRange"
          type="daterange"
          value-format="YYYY-MM-DD"
          start-placeholder="起始日期"
          end-placeholder="结束日期"
          style="width: 260px"
          @change="search"
        />
        <el-button type="primary" @click="search">查询</el-button>
      </div>
      <el-table :data="list" border stripe>
        <el-table-column label="出入库时间" width="170" fixed>
          <template #default="{ row }">{{ fmtTime(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column prop="docNo" label="单据号" width="150" show-overflow-tooltip />
        <el-table-column prop="remark" label="单据备注" min-width="120" show-overflow-tooltip />
        <el-table-column prop="warehouseName" label="仓库" width="120" show-overflow-tooltip />
        <el-table-column prop="locationCode" label="库位" width="100" />
        <el-table-column prop="skuCode" label="库存SKU" width="130" show-overflow-tooltip />
        <el-table-column label="配货名称" min-width="130" show-overflow-tooltip>
          <template #default="{ row }">{{ row.pickName || row.productName || '-' }}</template>
        </el-table-column>
        <el-table-column label="移动类型" width="110">
          <template #default="{ row }">{{ moveTypeMap[row.moveType] || row.moveType }}</template>
        </el-table-column>
        <el-table-column label="入库数量" width="100" align="right">
          <template #default="{ row }">{{ row.inboundQty ? fmtNum(row.inboundQty, 0) : '' }}</template>
        </el-table-column>
        <el-table-column label="出库数量" width="100" align="right">
          <template #default="{ row }">{{ row.outboundQty ? fmtNum(row.outboundQty, 0) : '' }}</template>
        </el-table-column>
        <el-table-column label="单价" width="100" align="right">
          <template #default="{ row }">{{ fmtNum(row.unitCost) }}</template>
        </el-table-column>
        <el-table-column label="金额" width="110" align="right">
          <template #default="{ row }">{{ fmtNum(row.amount) }}</template>
        </el-table-column>
        <el-table-column label="结存数量" width="100" align="right">
          <template #default="{ row }">{{ fmtNum(row.balanceAfter, 0) }}</template>
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
.toolbar { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 12px; }
.pager { margin-top: 16px; justify-content: flex-end; }
</style>
