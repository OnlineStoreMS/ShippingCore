<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import { api } from '../../api/wms'

const loading = ref(false)
const list = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const warehouses = ref<any[]>([])
const categories = ref<any[]>([])
const warehouseId = ref<number | undefined>()
const categoryId = ref<number | undefined>()
const keyword = ref('')
const skuCode = ref('')
const hideZero = ref(true)
const selected = ref<any[]>([])

const categoryMap = computed(() => {
  const m = new Map<number, string>()
  for (const c of categories.value) m.set(c.id, c.name)
  return m
})

const selectedStats = computed(() => {
  let qty = 0
  let available = 0
  let amount = 0
  for (const row of selected.value) {
    qty += Number(row.onHand) || 0
    available += Number(row.availableQty ?? row.onHand) || 0
    amount += Number(row.stockAmount) || 0
  }
  return {
    count: selected.value.length,
    qty,
    available,
    amount,
  }
})

function statusLabel(s?: string) {
  return ({ active: '在售', inactive: '停用', clearance: '清仓' } as Record<string, string>)[s || ''] || s || '-'
}

function fmtNum(v?: number, digits = 2) {
  if (v == null || Number.isNaN(v)) return '-'
  return Number(v).toFixed(digits)
}

function fmtSum(v: number, digits = 2) {
  return Number(v).toLocaleString('zh-CN', {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  })
}

async function loadMeta() {
  const [wh, cat] = await Promise.all([
    api.listWarehouses({ page: 1, pageSize: 200 }),
    api.listCategories({ page: 1, pageSize: 500 }),
  ])
  warehouses.value = (wh.list || []).filter((w: any) => w.status !== 0)
  categories.value = cat.list || []
}

async function load() {
  loading.value = true
  selected.value = []
  try {
    const res = await api.stockBalances({
      page: page.value,
      pageSize: pageSize.value,
      warehouseId: warehouseId.value,
      categoryId: categoryId.value,
      keyword: keyword.value || undefined,
      skuCode: skuCode.value || undefined,
      hideZero: hideZero.value || undefined,
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
  await loadMeta()
  await load()
})

function search() {
  page.value = 1
  load()
}

function onSelectionChange(rows: any[]) {
  selected.value = rows
}
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header><span>库存查询</span></template>
      <div class="toolbar">
        <el-select v-model="warehouseId" clearable filterable placeholder="发货仓库" style="width: 180px" @change="search">
          <el-option v-for="w in warehouses" :key="w.id" :label="w.name" :value="w.id" />
        </el-select>
        <el-select v-model="categoryId" clearable filterable placeholder="商品类别" style="width: 160px" @change="search">
          <el-option v-for="c in categories" :key="c.id" :label="c.name" :value="c.id" />
        </el-select>
        <el-input v-model="skuCode" placeholder="库存SKU" clearable style="width: 150px" @change="search" />
        <el-input v-model="keyword" placeholder="配货名称/商品名" clearable :prefix-icon="Search" style="width: 200px" @change="search" />
        <el-checkbox v-model="hideZero" @change="search">隐藏库存为0</el-checkbox>
        <el-button type="primary" @click="search">查询</el-button>
      </div>
      <el-table
        :data="list"
        border
        stripe
        style="width: 100%"
        table-layout="auto"
        row-key="id"
        @selection-change="onSelectionChange"
      >
        <el-table-column type="selection" width="48" fixed />
        <el-table-column label="图片" width="64" align="center" fixed>
          <template #default="{ row }">
            <el-image v-if="row.pic" :src="row.pic" fit="cover" style="width: 40px; height: 40px" :preview-src-list="[row.pic]" preview-teleported />
            <span v-else class="muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="skuCode" label="库存SKU" width="130" fixed show-overflow-tooltip />
        <el-table-column prop="pickName" label="配货名称" min-width="140" show-overflow-tooltip />
        <el-table-column label="商品类别" width="110" show-overflow-tooltip>
          <template #default="{ row }">{{ row.categoryName || categoryMap.get(row.categoryId) || '-' }}</template>
        </el-table-column>
        <el-table-column label="商品状态" width="90">
          <template #default="{ row }">{{ statusLabel(row.skuStatus) }}</template>
        </el-table-column>
        <el-table-column prop="warehouseName" label="发货仓库" width="120" show-overflow-tooltip />
        <el-table-column prop="onHand" label="库存数量" width="100" align="right" />
        <el-table-column label="占用数量" width="90" align="right">
          <template #default="{ row }">{{ fmtNum(row.reservedQty ?? 0, 0) }}</template>
        </el-table-column>
        <el-table-column label="可用数量" width="100" align="right">
          <template #default="{ row }">{{ row.availableQty ?? row.onHand }}</template>
        </el-table-column>
        <el-table-column prop="locationCode" label="库位" width="100" />
        <el-table-column label="库存单价" width="100" align="right">
          <template #default="{ row }">{{ fmtNum(row.unitCost ?? row.lastCost) }}</template>
        </el-table-column>
        <el-table-column label="库存金额" width="110" align="right">
          <template #default="{ row }">{{ fmtNum(row.stockAmount) }}</template>
        </el-table-column>
        <el-table-column label="最低采购价" width="110" align="right">
          <template #default="{ row }">{{ fmtNum(row.minPurchasePrice) }}</template>
        </el-table-column>
        <el-table-column label="上次采购价" width="110" align="right">
          <template #default="{ row }">{{ fmtNum(row.lastCost) }}</template>
        </el-table-column>
        <el-table-column label="重量(g)" width="90" align="right">
          <template #default="{ row }">{{ fmtNum(row.weightG, 1) }}</template>
        </el-table-column>
        <el-table-column prop="brand" label="品牌" width="100" show-overflow-tooltip />
        <el-table-column prop="specClass" label="规格" width="90" show-overflow-tooltip />
        <el-table-column prop="model" label="型号" width="90" show-overflow-tooltip />
        <el-table-column prop="material" label="材质" width="90" show-overflow-tooltip />
        <el-table-column prop="style1" label="款式1" width="90" show-overflow-tooltip />
        <el-table-column prop="style2" label="款式2" width="90" show-overflow-tooltip />
        <el-table-column prop="style3" label="款式3" width="90" show-overflow-tooltip />
      </el-table>
      <div class="footer-bar">
        <div class="stats" :class="{ active: selectedStats.count > 0 }">
          <span>已选 <b>{{ selectedStats.count }}</b> 条</span>
          <span>库存数量 <b>{{ fmtSum(selectedStats.qty, 0) }}</b></span>
          <span>可用数量 <b>{{ fmtSum(selectedStats.available, 0) }}</b></span>
          <span>库存金额 <b>{{ fmtSum(selectedStats.amount) }}</b></span>
        </div>
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
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.toolbar { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 12px; }
.footer-bar {
  margin-top: 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}
.stats {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  font-size: 13px;
  color: #909399;
  padding: 6px 12px;
  background: #f5f7fa;
  border-radius: 6px;
}
.stats.active {
  color: #303133;
  background: #ecf5ff;
}
.stats b {
  font-weight: 600;
  margin-left: 4px;
  color: var(--el-color-primary);
}
.pager { margin-left: auto; }
.muted { color: #bbb; }
</style>
