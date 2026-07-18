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
const dateRange = ref<[string, string] | null>(null)

function fmtNum(v?: number, digits = 2) {
  if (v == null || Number.isNaN(v)) return '-'
  return Number(v).toFixed(digits)
}

async function loadWarehouses() {
  const res = await api.listWarehouses({ page: 1, pageSize: 200 })
  warehouses.value = (res.list || []).filter((w: any) => w.status !== 0)
}

async function load() {
  loading.value = true
  try {
    const res = await api.stockSummary({
      page: page.value,
      pageSize: pageSize.value,
      warehouseId: warehouseId.value,
      skuCode: skuCode.value || undefined,
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
      <template #header><span>库存汇总账</span></template>
      <div class="toolbar">
        <el-select v-model="warehouseId" clearable filterable placeholder="仓库" style="width: 180px" @change="search">
          <el-option v-for="w in warehouses" :key="w.id" :label="w.name" :value="w.id" />
        </el-select>
        <el-input v-model="skuCode" placeholder="库存SKU" clearable style="width: 150px" @change="search" />
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
        <el-table-column prop="warehouseName" label="仓库名称" width="120" fixed show-overflow-tooltip />
        <el-table-column prop="skuCode" label="库存SKU" width="130" fixed show-overflow-tooltip />
        <el-table-column label="配货名称" min-width="140" show-overflow-tooltip>
          <template #default="{ row }">{{ row.pickName || row.productName || '-' }}</template>
        </el-table-column>
        <el-table-column prop="style1" label="款式1" width="90" show-overflow-tooltip />
        <el-table-column prop="style2" label="款式2" width="90" show-overflow-tooltip />
        <el-table-column prop="style3" label="款式3" width="90" show-overflow-tooltip />
        <el-table-column prop="purchaser" label="采购员" width="90" show-overflow-tooltip />
        <el-table-column label="成本价" width="100" align="right">
          <template #default="{ row }">{{ fmtNum(row.costPrice) }}</template>
        </el-table-column>
        <el-table-column label="期初" align="center">
          <el-table-column label="数量" width="90" align="right">
            <template #default="{ row }">{{ fmtNum(row.opening, 0) }}</template>
          </el-table-column>
          <el-table-column label="金额" width="100" align="right">
            <template #default="{ row }">{{ fmtNum(row.openingAmount) }}</template>
          </el-table-column>
        </el-table-column>
        <el-table-column label="本期入库" align="center">
          <el-table-column label="数量" width="90" align="right">
            <template #default="{ row }">{{ fmtNum(row.inbound, 0) }}</template>
          </el-table-column>
          <el-table-column label="金额" width="100" align="right">
            <template #default="{ row }">{{ fmtNum(row.inboundAmount) }}</template>
          </el-table-column>
        </el-table-column>
        <el-table-column label="本期出库" align="center">
          <el-table-column label="数量" width="90" align="right">
            <template #default="{ row }">{{ fmtNum(row.outbound, 0) }}</template>
          </el-table-column>
          <el-table-column label="金额" width="100" align="right">
            <template #default="{ row }">{{ fmtNum(row.outboundAmount) }}</template>
          </el-table-column>
        </el-table-column>
        <el-table-column label="期末" align="center">
          <el-table-column label="数量" width="90" align="right">
            <template #default="{ row }">{{ fmtNum(row.closing, 0) }}</template>
          </el-table-column>
          <el-table-column label="平均单价" width="100" align="right">
            <template #default="{ row }">{{ fmtNum(row.avgUnitCost) }}</template>
          </el-table-column>
          <el-table-column label="金额" width="100" align="right">
            <template #default="{ row }">{{ fmtNum(row.closingAmount) }}</template>
          </el-table-column>
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
