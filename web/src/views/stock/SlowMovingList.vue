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
const days = ref(30)
const minOnHand = ref(0)

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
    const res = await api.stockSlowMoving({
      page: page.value,
      pageSize: pageSize.value,
      warehouseId: warehouseId.value,
      days: days.value,
      minOnHand: minOnHand.value || undefined,
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
      <template #header><span>滞销查询</span></template>
      <div class="toolbar">
        <el-select v-model="warehouseId" clearable filterable placeholder="仓库" style="width: 180px" @change="search">
          <el-option v-for="w in warehouses" :key="w.id" :label="w.name" :value="w.id" />
        </el-select>
        <span>闲置天数 ≥</span>
        <el-input-number v-model="days" :min="1" :max="3650" :controls="false" style="width: 100px" />
        <span>可用数量 ></span>
        <el-input-number v-model="minOnHand" :min="0" :controls="false" style="width: 100px" />
        <el-button type="primary" @click="search">查询</el-button>
      </div>
      <el-table :data="list" border stripe>
        <el-table-column prop="warehouseName" label="仓库" width="130" show-overflow-tooltip />
        <el-table-column prop="skuCode" label="库存SKU" width="140" show-overflow-tooltip />
        <el-table-column label="商品名称" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">{{ row.pickName || row.productName || '-' }}</template>
        </el-table-column>
        <el-table-column label="可用数量" width="100" align="right">
          <template #default="{ row }">{{ row.availableQty ?? row.onHand }}</template>
        </el-table-column>
        <el-table-column label="库存单价" width="100" align="right">
          <template #default="{ row }">{{ fmtNum(row.unitCost) }}</template>
        </el-table-column>
        <el-table-column label="库存金额" width="110" align="right">
          <template #default="{ row }">{{ fmtNum(row.stockAmount) }}</template>
        </el-table-column>
        <el-table-column label="商品创建时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column label="最后入库时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.lastInboundAt || row.lastMoveAt) }}</template>
        </el-table-column>
        <el-table-column label="最后入库数量" width="120" align="right">
          <template #default="{ row }">{{ row.lastInboundQty ? fmtNum(row.lastInboundQty, 0) : '-' }}</template>
        </el-table-column>
        <el-table-column prop="idleDays" label="闲置天数" width="100" align="right" />
      </el-table>
      <el-pagination
        class="pager"
        layout="total, sizes, prev, pager, next"
        :total="total"
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :page-sizes="[20, 50, 100, 500]"
        @current-change="load"
        @size-change="search"
      />
    </el-card>
  </div>
</template>

<style scoped>
.toolbar { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 12px; }
.pager { margin-top: 16px; justify-content: flex-end; }
</style>
