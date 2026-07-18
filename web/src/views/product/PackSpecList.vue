<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { api } from '../../api/wms'
import SkuSearchSelect from '../../components/SkuSearchSelect.vue'

const loading = ref(false)
const list = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')
const visible = ref(false)
const form = ref<any>({})

const selectedId = ref<number | null>(null)
const selectedName = ref('')
const bindList = ref<any[]>([])
const bindVisible = ref(false)
const bindForm = ref<any>({})

async function load() {
  loading.value = true
  try {
    const res = await api.listPackSpecs({
      page: page.value,
      pageSize: pageSize.value,
      keyword: keyword.value || undefined,
    })
    list.value = res.list
    total.value = res.total
    if (selectedId.value && !list.value.some((x) => x.id === selectedId.value)) {
      selectedId.value = null
      selectedName.value = ''
      bindList.value = []
    }
  } catch (e) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function loadBinds(row: any) {
  selectedId.value = row.id
  selectedName.value = row.name
  try {
    bindList.value = await api.listPackSpecSkus(row.id)
  } catch (e) {
    ElMessage.error((e as Error).message || '加载绑定失败')
  }
}

onMounted(load)

function openCreate() {
  form.value = { name: '', cost: 0, weightG: 0, remark: '', status: 1 }
  visible.value = true
}

function openEdit(row: any) {
  form.value = { ...row }
  visible.value = true
}

async function save() {
  try {
    if (form.value.id) {
      await api.updatePackSpec(form.value.id, form.value)
    } else {
      await api.createPackSpec(form.value)
    }
    ElMessage.success('已保存')
    visible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '保存失败')
  }
}

async function remove(row: any) {
  await ElMessageBox.confirm(`确认删除包装规格「${row.name}」？`, '提示', { type: 'warning' })
  await api.deletePackSpec(row.id)
  ElMessage.success('已删除')
  if (selectedId.value === row.id) {
    selectedId.value = null
    selectedName.value = ''
    bindList.value = []
  }
  await load()
}

function openBind() {
  if (!selectedId.value) {
    ElMessage.warning('请先选择左侧包装规格')
    return
  }
  bindForm.value = { packSpecId: selectedId.value, invSkuId: undefined, qtyMin: 0, qtyMax: 0, remark: '' }
  bindVisible.value = true
}

async function saveBind() {
  try {
    if (!bindForm.value.invSkuId) {
      ElMessage.warning('请选择库存SKU')
      return
    }
    await api.bindPackSpecSku(selectedId.value!, bindForm.value)
    ElMessage.success('已绑定')
    bindVisible.value = false
    await loadBinds({ id: selectedId.value, name: selectedName.value })
  } catch (e) {
    ElMessage.error((e as Error).message || '绑定失败')
  }
}

async function unbind(row: any) {
  await ElMessageBox.confirm(`确认解绑 SKU「${row.skuCode}」？`, '提示', { type: 'warning' })
  await api.unbindPackSpecSku(row.id)
  ElMessage.success('已解绑')
  await loadBinds({ id: selectedId.value, name: selectedName.value })
}

function search() {
  page.value = 1
  load()
}
</script>

<template>
  <div class="page" v-loading="loading">
    <div class="split">
      <el-card class="left">
        <template #header>
          <div class="hdr">
            <span>包装规格</span>
            <el-button type="primary" :icon="Plus" @click="openCreate">新增包装规格</el-button>
          </div>
        </template>
        <div class="toolbar">
          <el-input
            v-model="keyword"
            placeholder="包装规格名称"
            clearable
            :prefix-icon="Search"
            style="width: 200px"
            @change="search"
          />
          <el-button type="primary" @click="search">查询</el-button>
        </div>
        <el-table
          :data="list"
          border
          stripe
          highlight-current-row
          @current-change="(row: any) => row && loadBinds(row)"
        >
          <el-table-column prop="name" label="包装规格名称" min-width="140" />
          <el-table-column prop="cost" label="成本价(¥)" width="110" />
          <el-table-column prop="weightG" label="重量(g)" width="100" />
          <el-table-column prop="remark" label="备注" min-width="120" show-overflow-tooltip />
          <el-table-column label="操作" width="140" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click.stop="openEdit(row)">编辑</el-button>
              <el-button link type="danger" @click.stop="remove(row)">删除</el-button>
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

      <el-card class="right">
        <template #header>
          <div class="hdr">
            <span>绑定商品{{ selectedName ? ` · ${selectedName}` : '' }}</span>
            <el-button type="primary" :disabled="!selectedId" @click="openBind">绑定库存SKU</el-button>
          </div>
        </template>
        <el-empty v-if="!selectedId" description="请选择左侧包装规格" />
        <el-table v-else :data="bindList" border stripe>
          <el-table-column prop="id" label="ID" width="80" />
          <el-table-column prop="skuCode" label="库存SKU" width="140" />
          <el-table-column prop="pickName" label="配货名称" min-width="120" />
          <el-table-column prop="numRange" label="数量范围" width="120" />
          <el-table-column label="操作" width="100">
            <template #default="{ row }">
              <el-button link type="danger" @click="unbind(row)">解绑</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </div>

    <el-dialog v-model="visible" :title="form.id ? '编辑包装规格' : '新增包装规格'" width="480px" destroy-on-close>
      <el-form :model="form" label-width="120px">
        <el-form-item label="包装规格名称" required><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="成本价(¥)"><el-input-number v-model="form.cost" :min="0" :precision="4" style="width: 100%" /></el-form-item>
        <el-form-item label="重量(g)"><el-input-number v-model="form.weightG" :min="0" :precision="1" style="width: 100%" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="form.remark" type="textarea" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="visible = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="bindVisible" title="绑定库存SKU" width="480px" destroy-on-close>
      <el-form :model="bindForm" label-width="100px">
        <el-form-item label="库存SKU" required>
          <SkuSearchSelect v-model="bindForm.invSkuId" />
        </el-form-item>
        <el-form-item label="数量下限"><el-input-number v-model="bindForm.qtyMin" :min="0" style="width: 100%" /></el-form-item>
        <el-form-item label="数量上限">
          <el-input-number v-model="bindForm.qtyMax" :min="0" style="width: 100%" />
          <div class="hint">0 表示不限上限</div>
        </el-form-item>
        <el-form-item label="备注"><el-input v-model="bindForm.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="bindVisible = false">取消</el-button>
        <el-button type="primary" @click="saveBind">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.split { display: flex; gap: 12px; align-items: stretch; }
.left { flex: 3; min-width: 0; }
.right { flex: 2; min-width: 0; }
.hdr { display: flex; justify-content: space-between; align-items: center; }
.toolbar { display: flex; gap: 8px; margin-bottom: 12px; }
.pager { margin-top: 16px; justify-content: flex-end; }
.hint { font-size: 12px; color: #999; margin-top: 4px; }
</style>
