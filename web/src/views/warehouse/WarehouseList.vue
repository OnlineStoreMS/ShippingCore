<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { api } from '../../api/wms'

const loading = ref(false)
const list = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const visible = ref(false)
const form = ref<any>({})
const showDisabled = ref(false)
const keyword = ref('')

/** 对齐普源：自建仓库 / 退货仓 / 中转仓 */
const typeOptions = [
  { label: '自建仓库', value: 'central' },
  { label: '退货仓', value: 'return' },
  { label: '中转仓', value: 'transit' },
]

const displayList = computed(() => {
  if (showDisabled.value) return list.value
  return list.value.filter((w) => w.status !== 0)
})

async function load() {
  loading.value = true
  try {
    const res = await api.listWarehouses({
      page: page.value,
      pageSize: pageSize.value,
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

onMounted(load)

function search() {
  page.value = 1
  load()
}

function openCreate() {
  form.value = {
    code: '',
    name: '',
    type: 'central',
    address: '',
    contact: '',
    phone: '',
    country: '',
    remark: '',
    isDefault: 0,
    allowCalcFee: 0,
    allowNegativeStock: 0,
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
    if (form.value.id) await api.updateWarehouse(form.value.id, form.value)
    else await api.createWarehouse(form.value)
    ElMessage.success('已保存')
    visible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '保存失败')
  }
}

async function remove(row: any) {
  await ElMessageBox.confirm(`确认删除仓库「${row.name}」？`, '提示', { type: 'warning' })
  await api.deleteWarehouse(row.id)
  ElMessage.success('已删除')
  await load()
}

function typeLabel(t: string) {
  return typeOptions.find((o) => o.value === t)?.label || t
}
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header>
        <div class="hdr">
          <span>仓库设置</span>
          <el-button type="primary" :icon="Plus" @click="openCreate">新增仓库</el-button>
        </div>
      </template>
      <div class="toolbar">
        <el-checkbox v-model="showDisabled" @change="search">显示停用仓库</el-checkbox>
        <el-input
          v-model="keyword"
          clearable
          placeholder="仓库编码/名称"
          :prefix-icon="Search"
          style="width: 220px"
          @change="search"
        />
        <el-button type="primary" @click="search">查询</el-button>
      </div>
      <el-table :data="displayList" border stripe>
        <el-table-column label="仓库类型" width="110">
          <template #default="{ row }">{{ typeLabel(row.type) }}</template>
        </el-table-column>
        <el-table-column prop="code" label="仓库编码" width="120" />
        <el-table-column prop="name" label="仓库名称" min-width="140" />
        <el-table-column label="默认仓库" width="90" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.isDefault" type="success" size="small">是</el-tag>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="允许负库存" width="100" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.allowNegativeStock" type="danger" size="small">是</el-tag>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="计算仓库费用" width="120" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.allowCalcFee" type="warning" size="small">是</el-tag>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="country" label="所在国家" width="100" show-overflow-tooltip />
        <el-table-column prop="address" label="仓库地址" min-width="160" show-overflow-tooltip />
        <el-table-column prop="locationInfo" label="库位信息" width="140" />
        <el-table-column prop="remark" label="备注" min-width="120" show-overflow-tooltip />
        <el-table-column label="仓库状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small">
              {{ row.status === 1 ? '启用' : '停用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
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

    <el-dialog v-model="visible" :title="form.id ? '编辑仓库' : '新增仓库'" width="580px">
      <el-form :model="form" label-width="120px">
        <el-form-item label="仓库编码" required><el-input v-model="form.code" /></el-form-item>
        <el-form-item label="仓库名称" required><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="仓库类型">
          <el-select v-model="form.type" style="width: 100%">
            <el-option v-for="o in typeOptions" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="所在国家"><el-input v-model="form.country" placeholder="如 CN" /></el-form-item>
        <el-form-item label="仓库地址"><el-input v-model="form.address" /></el-form-item>
        <el-form-item label="联系人"><el-input v-model="form.contact" /></el-form-item>
        <el-form-item label="电话"><el-input v-model="form.phone" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="form.remark" type="textarea" :rows="2" /></el-form-item>
        <el-form-item label="默认仓库">
          <el-switch v-model="form.isDefault" :active-value="1" :inactive-value="0" />
        </el-form-item>
        <el-form-item label="允许负库存">
          <el-switch v-model="form.allowNegativeStock" :active-value="1" :inactive-value="0" />
        </el-form-item>
        <el-form-item label="计算仓库费用">
          <el-switch v-model="form.allowCalcFee" :active-value="1" :inactive-value="0" />
        </el-form-item>
        <el-form-item label="仓库状态">
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
</style>
