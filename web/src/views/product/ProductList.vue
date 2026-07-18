<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { api } from '../../api/wms'
import ProductFormDialog from './ProductFormDialog.vue'

type ViewTab = 'all' | 'sku' | 'combo' | 'assembly'

const router = useRouter()
const loading = ref(false)
const list = ref<any[]>([])
const skuList = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')
const viewTab = ref<ViewTab>('all')
const categoryId = ref<number | undefined>()
const uncategorized = ref(false)

const formVisible = ref(false)
const editingProductId = ref<number | null>(null)
const defaultSkuType = ref('normal')

const categories = ref<any[]>([])
const warehouses = ref<any[]>([])
const packSpecs = ref<any[]>([])

const categoryMap = computed(() => {
  const m = new Map<number, string>()
  for (const c of categories.value) m.set(c.id, c.name)
  return m
})
const warehouseMap = computed(() => {
  const m = new Map<number, string>()
  for (const w of warehouses.value) m.set(w.id, w.name)
  return m
})
const packSpecMap = computed(() => {
  const m = new Map<number, string>()
  for (const p of packSpecs.value) m.set(p.id, p.name)
  return m
})

type TreeNode = { id: string | number; label: string; children: TreeNode[] }

const treeData = computed(() => {
  const childrenOf = (pid: number): TreeNode[] =>
    categories.value
      .filter((c) => c.parentId === pid)
      .map((c) => ({
        id: c.id,
        label: c.name,
        children: childrenOf(c.id),
      }))
  const roots = categories.value.filter((c) => !c.parentId)
  return [
    { id: 'all', label: '全部类别', children: [] },
    { id: 'uncat', label: '*未设类别', children: [] },
    ...roots.map((c) => ({
      id: c.id,
      label: c.name,
      children: childrenOf(c.id),
    })),
  ] as TreeNode[]
})

const productTypeFilter = computed(() => {
  if (viewTab.value === 'combo') return 'combo'
  if (viewTab.value === 'assembly') return 'assembly'
  return ''
})

async function loadCategories() {
  try {
    const res = await api.listCategories({ page: 1, pageSize: 500 })
    categories.value = res.list || []
  } catch { /* ignore */ }
}

async function loadWarehouses() {
  try {
    const res = await api.listWarehouses({ page: 1, pageSize: 200 })
    warehouses.value = res.list || []
  } catch { /* ignore */ }
}

async function loadPackSpecs() {
  try {
    const res = await api.listPackSpecs({ page: 1, pageSize: 500 })
    packSpecs.value = res.list || []
  } catch { /* ignore */ }
}

async function load() {
  loading.value = true
  try {
    const params: Record<string, unknown> = {
      page: page.value,
      pageSize: pageSize.value,
      keyword: keyword.value || undefined,
    }
    if (uncategorized.value) params.uncategorized = 1
    else if (categoryId.value) params.categoryId = categoryId.value
    if (productTypeFilter.value) params.productType = productTypeFilter.value

    if (viewTab.value === 'sku') {
      const res = await api.listSkus(params)
      skuList.value = res.list
      total.value = res.total
      list.value = []
    } else {
      const res = await api.listProducts(params)
      list.value = res.list
      total.value = res.total
      skuList.value = []
    }
  } catch (e) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadCategories(), loadWarehouses(), loadPackSpecs()])
  await load()
})

watch(viewTab, () => {
  page.value = 1
  load()
})

function onTreeClick(data: any) {
  if (data.id === 'all') {
    categoryId.value = undefined
    uncategorized.value = false
  } else if (data.id === 'uncat') {
    categoryId.value = undefined
    uncategorized.value = true
  } else {
    categoryId.value = Number(data.id)
    uncategorized.value = false
  }
  page.value = 1
  load()
}

function search() {
  page.value = 1
  load()
}

function onPageSizeChange() {
  page.value = 1
  load()
}

function catName(id?: number) {
  if (!id) return '未设类别'
  return categoryMap.value.get(id) || String(id)
}

function whName(id?: number) {
  if (!id) return '-'
  return warehouseMap.value.get(id) || String(id)
}

function packName(id?: number) {
  if (!id) return '-'
  return packSpecMap.value.get(id) || String(id)
}

function formatDate(v?: string) {
  if (!v) return '-'
  return String(v).slice(0, 10)
}

function statusLabel(s?: string) {
  return ({ active: '在售', inactive: '停用', clearance: '清仓' } as Record<string, string>)[s || ''] || s || '-'
}

function typeLabel(t?: string) {
  return ({ normal: '普通', combo: '组合品', assembly: '组装品' } as Record<string, string>)[t || ''] || t || '-'
}

function goodsKindLabel(k?: string) {
  return ({ normal: '普通商品', packaging: '包材', accessory: '配件', gift: '赠品' } as Record<string, string>)[k || ''] || k || '普通商品'
}

function openCreateProduct(skuType = 'normal') {
  defaultSkuType.value = skuType
  editingProductId.value = null
  formVisible.value = true
}

function openEditProduct(row: any) {
  defaultSkuType.value = row.skus?.[0]?.productType || 'normal'
  editingProductId.value = row.id
  formVisible.value = true
}

function openEditByParentId(parentId: number) {
  editingProductId.value = parentId
  formVisible.value = true
}

async function removeProduct(row: any) {
  await ElMessageBox.confirm(`确认删除商品「${row.name}」？`, '提示', { type: 'warning' })
  await api.deleteProduct(row.id)
  ElMessage.success('已删除')
  await load()
}

async function removeSku(sku: any) {
  await ElMessageBox.confirm(`确认删除 SKU「${sku.skuCode}」？`, '提示', { type: 'warning' })
  await api.deleteSku(sku.id)
  ElMessage.success('已删除')
  await load()
}
</script>

<template>
  <div class="page" v-loading="loading">
    <aside class="cat-pane">
      <div class="cat-hdr">
        <span>商品类别</span>
        <el-button link type="primary" @click="router.push('/categories')">管理</el-button>
      </div>
      <el-tree
        :data="treeData"
        node-key="id"
        default-expand-all
        highlight-current
        :props="{ label: 'label', children: 'children' }"
        @node-click="onTreeClick"
      />
    </aside>

    <section class="main-pane">
      <div class="toolbar">
        <el-input
          v-model="keyword"
          placeholder="父SKU / 名称 / 库存SKU"
          clearable
          :prefix-icon="Search"
          style="width: 260px"
          @change="search"
        />
        <el-button type="primary" @click="search">查询</el-button>
        <el-radio-group v-model="viewTab" size="default">
          <el-radio-button value="all">全部</el-radio-button>
          <el-radio-button value="sku">库存SKU明细</el-radio-button>
          <el-radio-button value="assembly">组装品</el-radio-button>
          <el-radio-button value="combo">组合品</el-radio-button>
        </el-radio-group>
        <div class="spacer" />
        <el-dropdown split-button type="primary" @click="openCreateProduct('normal')">
          <el-icon><Plus /></el-icon> 新增商品
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item @click="openCreateProduct('normal')">新增普通商品(库存SKU)</el-dropdown-item>
              <el-dropdown-item @click="openCreateProduct('combo')">新增组合品</el-dropdown-item>
              <el-dropdown-item @click="openCreateProduct('assembly')">新增组装品(加工产品)</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>

      <!-- 父商品视图（对齐普源「全部」） -->
      <el-table v-if="viewTab !== 'sku'" :data="list" row-key="id" border stripe>
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="sku-wrap">
              <div class="sku-hdr">
                <span>多款式详情（库存SKU）</span>
                <el-button size="small" type="primary" link @click="openEditProduct(row)">编辑多款式</el-button>
              </div>
              <el-table :data="row.skus || []" border size="small">
                <el-table-column label="图片" width="70">
                  <template #default="{ row: sku }">
                    <el-image
                      v-if="sku.pic"
                      :src="sku.pic"
                      style="width: 40px; height: 40px"
                      fit="cover"
                      :preview-src-list="[sku.pic]"
                      preview-teleported
                    />
                    <span v-else>-</span>
                  </template>
                </el-table-column>
                <el-table-column prop="skuCode" label="库存SKU" width="130" />
                <el-table-column label="商品类型" width="100">
                  <template #default="{ row: sku }">{{ goodsKindLabel(sku.goodsKind) }}</template>
                </el-table-column>
                <el-table-column prop="pickName" label="配货名称" min-width="120" />
                <el-table-column prop="style1" label="款式1" width="90" />
                <el-table-column prop="style2" label="款式2" width="90" />
                <el-table-column prop="style3" label="款式3" width="90" />
                <el-table-column prop="status" label="商品状态" width="90">
                  <template #default="{ row: sku }">{{ statusLabel(sku.status) }}</template>
                </el-table-column>
                <el-table-column prop="lastPurchasePrice" label="上次采购价" width="110" />
                <el-table-column prop="retailPrice" label="零售价" width="90" />
                <el-table-column prop="weightG" label="重量(g)" width="90" />
                <el-table-column prop="upc" label="UPC码" width="110" />
                <el-table-column prop="asin" label="ASIN码" width="110" />
                <el-table-column prop="supplierItemNo" label="供应商货号" width="120" />
                <el-table-column prop="productType" label="结构" width="80">
                  <template #default="{ row: sku }">{{ typeLabel(sku.productType) }}</template>
                </el-table-column>
                <el-table-column prop="description" label="备注" min-width="100" show-overflow-tooltip />
                <el-table-column label="操作" width="140" fixed="right">
                  <template #default="{ row: sku }">
                    <el-button link type="primary" @click="openEditProduct(row)">编辑</el-button>
                    <el-button link type="danger" @click="removeSku(sku)">删除</el-button>
                  </template>
                </el-table-column>
              </el-table>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="图片" width="70">
          <template #default="{ row }">
            <el-image
              v-if="row.pic"
              :src="row.pic"
              style="width: 40px; height: 40px"
              fit="cover"
              :preview-src-list="[row.pic]"
              preview-teleported
            />
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="商品类别" width="120">
          <template #default="{ row }">{{ catName(row.categoryId) }}</template>
        </el-table-column>
        <el-table-column prop="parentSku" label="父SKU" width="140" />
        <el-table-column label="库存SKU" width="110">
          <template #default="{ row }">
            <span v-if="(row.skus || []).length > 1">多款式商品</span>
            <span v-else-if="(row.skus || []).length === 1">{{ row.skus[0].skuCode }}</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="商品名称" min-width="160" show-overflow-tooltip />
        <el-table-column label="配货名称" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            {{ (row.skus || []).length === 1 ? (row.skus[0].pickName || '-') : ((row.skus || []).length > 1 ? '多款式详情' : '-') }}
          </template>
        </el-table-column>
        <el-table-column label="默认发货仓库" width="130">
          <template #default="{ row }">{{ whName(row.defaultWarehouseId) }}</template>
        </el-table-column>
        <el-table-column label="外包装规格" width="120" show-overflow-tooltip>
          <template #default="{ row }">{{ packName(row.packSpecId) }}</template>
        </el-table-column>
        <el-table-column label="开发日期" width="110">
          <template #default="{ row }">{{ formatDate(row.developedAt) }}</template>
        </el-table-column>
        <el-table-column prop="scoreFactor" label="评分系数" width="90" />
        <el-table-column prop="remark" label="备注" min-width="120" show-overflow-tooltip />
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEditProduct(row)">编辑</el-button>
            <el-button link type="danger" @click="removeProduct(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 库存SKU明细视图 -->
      <el-table v-else :data="skuList" row-key="id" border stripe>
        <el-table-column label="图片" width="70">
          <template #default="{ row }">
            <el-image
              v-if="row.pic"
              :src="row.pic"
              style="width: 40px; height: 40px"
              fit="cover"
              :preview-src-list="[row.pic]"
              preview-teleported
            />
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="parentSku" label="父SKU" width="130" />
        <el-table-column prop="productName" label="商品名称" min-width="140" show-overflow-tooltip />
        <el-table-column prop="skuCode" label="库存SKU" width="130" />
        <el-table-column label="商品类型" width="100">
          <template #default="{ row }">{{ goodsKindLabel(row.goodsKind) }}</template>
        </el-table-column>
        <el-table-column prop="pickName" label="配货名称" min-width="120" />
        <el-table-column prop="style1" label="款式1" width="90" />
        <el-table-column prop="style2" label="款式2" width="90" />
        <el-table-column prop="style3" label="款式3" width="90" />
        <el-table-column prop="status" label="状态" width="80">
          <template #default="{ row }">{{ statusLabel(row.status) }}</template>
        </el-table-column>
        <el-table-column prop="lastPurchasePrice" label="上次采购价" width="110" />
        <el-table-column prop="retailPrice" label="零售价" width="90" />
        <el-table-column prop="weightG" label="重量(g)" width="90" />
        <el-table-column prop="upc" label="UPC" width="110" />
        <el-table-column prop="asin" label="ASIN" width="110" />
        <el-table-column prop="supplierItemNo" label="供应商货号" width="120" />
        <el-table-column prop="productType" label="类型" width="80">
          <template #default="{ row }">{{ typeLabel(row.productType) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEditByParentId(row.parentId)">编辑</el-button>
            <el-button link type="danger" @click="removeSku(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        class="pager"
        layout="total, prev, pager, next, sizes"
        :total="total"
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :page-sizes="[20, 50, 100]"
        @current-change="load"
        @size-change="onPageSizeChange"
      />
    </section>

    <ProductFormDialog
      v-model="formVisible"
      :product-id="editingProductId"
      :default-product-type="defaultSkuType"
      :categories="categories"
      :warehouses="warehouses"
      :pack-specs="packSpecs"
      :preset-category-id="categoryId"
      @saved="load"
    />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  gap: 12px;
  align-items: stretch;
  min-height: calc(100vh - 120px);
}
.cat-pane {
  width: 220px;
  flex-shrink: 0;
  background: #fff;
  border: 1px solid var(--el-border-color-light);
  border-radius: 6px;
  padding: 10px;
  overflow: auto;
}
.cat-hdr {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 600;
  margin-bottom: 8px;
}
.main-pane {
  flex: 1;
  min-width: 0;
  background: #fff;
  border: 1px solid var(--el-border-color-light);
  border-radius: 6px;
  padding: 12px;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  margin-bottom: 12px;
}
.spacer { flex: 1; }
.pager { margin-top: 16px; justify-content: flex-end; }
.sku-wrap { padding: 8px 16px 12px; }
.sku-hdr {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
  font-weight: 500;
}
</style>
