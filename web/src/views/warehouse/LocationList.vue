<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { api } from '../../api/wms'
import SkuSearchSelect from '../../components/SkuSearchSelect.vue'

const loading = ref(false)
const list = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const warehouses = ref<any[]>([])
const warehouseId = ref<number | undefined>()
const keyword = ref('')
const visible = ref(false)
const form = ref<any>({})

const selectedLocation = ref<any>(null)
const boundSkus = ref<any[]>([])
const bindLoading = ref(false)
const bindSkuId = ref<number | null>(null)

async function loadWarehouses() {
  const res = await api.listWarehouses({ page: 1, pageSize: 200 })
  warehouses.value = (res.list || []).filter((w: any) => w.status !== 0)
  if (!warehouseId.value && warehouses.value.length) {
    warehouseId.value = warehouses.value[0].id
  }
}

async function load() {
  if (!warehouseId.value) {
    list.value = []
    total.value = 0
    return
  }
  loading.value = true
  try {
    const res = await api.listLocations({
      page: page.value,
      pageSize: pageSize.value,
      warehouseId: warehouseId.value,
      keyword: keyword.value || undefined,
    })
    list.value = res.list
    total.value = res.total
    if (selectedLocation.value) {
      const still = list.value.find((r) => r.id === selectedLocation.value.id)
      if (still) selectedLocation.value = still
      else {
        selectedLocation.value = null
        boundSkus.value = []
      }
    }
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

watch(warehouseId, () => {
  page.value = 1
  selectedLocation.value = null
  boundSkus.value = []
  load()
})

function search() {
  page.value = 1
  load()
}

function openCreate() {
  form.value = {
    warehouseId: warehouseId.value,
    code: '',
    zone: '',
    aisle: '',
    shelf: '',
    bin: '',
    pickOrder: 0,
    pickPosition: '',
    remark: '',
    status: 1,
  }
  visible.value = true
}

function openEdit(row: any) {
  form.value = { ...row }
  visible.value = true
}

async function save() {
  try {
    if (form.value.id) await api.updateLocation(form.value.id, form.value)
    else await api.createLocation(form.value)
    ElMessage.success('已保存')
    visible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '保存失败')
  }
}

async function remove(row: any) {
  await ElMessageBox.confirm(`确认删除库位「${row.code}」？`, '提示', { type: 'warning' })
  await api.deleteLocation(row.id)
  ElMessage.success('已删除')
  if (selectedLocation.value?.id === row.id) {
    selectedLocation.value = null
    boundSkus.value = []
  }
  await load()
}

async function selectRow(row: any) {
  selectedLocation.value = row
  await loadBoundSkus()
}

async function loadBoundSkus() {
  if (!selectedLocation.value?.id) {
    boundSkus.value = []
    return
  }
  bindLoading.value = true
  try {
    boundSkus.value = await api.listLocationSkus(selectedLocation.value.id)
  } catch (e) {
    ElMessage.error((e as Error).message || '加载绑定SKU失败')
  } finally {
    bindLoading.value = false
  }
}

async function bindSku() {
  if (!selectedLocation.value?.id) {
    ElMessage.warning('请先选择库位')
    return
  }
  if (!bindSkuId.value) {
    ElMessage.warning('请选择库存SKU')
    return
  }
  try {
    await api.bindLocationSku(selectedLocation.value.id, { invSkuId: bindSkuId.value })
    ElMessage.success('已绑定')
    bindSkuId.value = null
    await loadBoundSkus()
  } catch (e) {
    ElMessage.error((e as Error).message || '绑定失败')
  }
}

async function unbindSku(row: any) {
  await ElMessageBox.confirm(`确认解绑 SKU「${row.skuCode}」？`, '提示', { type: 'warning' })
  await api.unbindLocationSku(row.id)
  ElMessage.success('已解绑')
  await loadBoundSkus()
}

function whName(id: number) {
  return warehouses.value.find((w) => w.id === id)?.name || id
}
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header>
        <div class="hdr">
          <span>库位管理</span>
          <el-button type="primary" :icon="Plus" :disabled="!warehouseId" @click="openCreate">新增库位</el-button>
        </div>
      </template>
      <div class="toolbar">
        <span>所属仓库</span>
        <el-select v-model="warehouseId" placeholder="选择仓库" style="width: 240px" filterable>
          <el-option v-for="w in warehouses" :key="w.id" :label="`${w.code} - ${w.name}`" :value="w.id" />
        </el-select>
        <el-input
          v-model="keyword"
          clearable
          placeholder="库位名称/拣货位置/备注"
          :prefix-icon="Search"
          style="width: 220px"
          @change="search"
        />
        <el-button type="primary" @click="search">查询</el-button>
      </div>
      <el-table
        :data="list"
        border
        stripe
        highlight-current-row
        @current-change="(row: any) => row && selectRow(row)"
      >
        <el-table-column label="所属仓库" width="140">
          <template #default="{ row }">{{ row.warehouseName || whName(row.warehouseId) }}</template>
        </el-table-column>
        <el-table-column prop="code" label="库位名称" width="140" />
        <el-table-column prop="pickOrder" label="拣货顺序" width="100" align="right" />
        <el-table-column prop="pickPosition" label="拣货位置" min-width="140" show-overflow-tooltip />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small">
              {{ row.status === 1 ? '启用' : '停用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="remark" label="备注" min-width="120" show-overflow-tooltip />
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        class="pager"
        layout="total, prev, pager, next"
        :total="total"
        v-model:current-page="page"
        :page-size="pageSize"
        @current-change="load"
      />
    </el-card>

    <el-card class="bind-card" v-loading="bindLoading">
      <template #header>
        <div class="hdr">
          <span>
            库位绑定库存SKU
            <template v-if="selectedLocation"> — {{ selectedLocation.code }}</template>
          </span>
        </div>
      </template>
      <div v-if="!selectedLocation" class="empty-hint">请在上方表格中选择一个库位</div>
      <template v-else>
        <div class="toolbar">
          <div class="bind-sku">
            <SkuSearchSelect v-model="bindSkuId" placeholder="搜索并选择库存SKU" />
          </div>
          <el-button type="primary" @click="bindSku">绑定SKU</el-button>
        </div>
        <el-table :data="boundSkus" border stripe empty-text="暂无绑定">
          <el-table-column prop="skuCode" label="库存SKU" width="160" />
          <el-table-column prop="pickName" label="配货名称" min-width="180" />
          <el-table-column label="操作" width="100">
            <template #default="{ row }">
              <el-button link type="danger" @click="unbindSku(row)">解绑</el-button>
            </template>
          </el-table-column>
        </el-table>
      </template>
    </el-card>

    <el-dialog v-model="visible" :title="form.id ? '编辑库位' : '新增库位'" width="560px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="所属仓库" required>
          <el-select v-model="form.warehouseId" style="width: 100%">
            <el-option v-for="w in warehouses" :key="w.id" :label="`${w.code} - ${w.name}`" :value="w.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="库位名称" required>
          <el-input v-model="form.code" placeholder="如 A-01-01" />
        </el-form-item>
        <el-form-item label="拣货顺序">
          <el-input-number v-model="form.pickOrder" :min="0" :controls="false" style="width: 100%" />
        </el-form-item>
        <el-form-item label="拣货位置">
          <el-input v-model="form.pickPosition" placeholder="可直接填写，或由库区-巷道-货架-格位自动拼接" />
        </el-form-item>
        <el-form-item label="库区"><el-input v-model="form.zone" /></el-form-item>
        <el-form-item label="巷道"><el-input v-model="form.aisle" /></el-form-item>
        <el-form-item label="货架"><el-input v-model="form.shelf" /></el-form-item>
        <el-form-item label="格位"><el-input v-model="form.bin" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="form.remark" /></el-form-item>
        <el-form-item label="状态">
          <el-switch v-model="form.status" :active-value="1" :inactive-value="0" active-text="启用" inactive-text="停用" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="visible = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.toolbar { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 12px; }
.pager { margin-top: 16px; justify-content: flex-end; }
.bind-card { margin-top: 16px; }
.empty-hint { color: #909399; padding: 12px 0; }
.bind-sku { width: 320px; }
</style>
