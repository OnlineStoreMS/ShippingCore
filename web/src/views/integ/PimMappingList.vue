<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { api } from '../../api/wms'

const loading = ref(false)
const list = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const visible = ref(false)
const form = ref<any>({})

async function load() {
  loading.value = true
  try {
    const res = await api.listPimMappings({ page: page.value, pageSize: pageSize.value })
    list.value = res.list
    total.value = res.total
  } catch (e) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(load)

function openCreate() {
  form.value = { invSkuId: undefined, pimSkuId: undefined, pimSkuCode: '', remark: '' }
  visible.value = true
}

function openEdit(row: any) {
  form.value = { ...row }
  visible.value = true
}

async function save() {
  try {
    await api.upsertPimMapping({
      invSkuId: form.value.invSkuId,
      pimSkuId: form.value.pimSkuId,
      pimSkuCode: form.value.pimSkuCode,
      remark: form.value.remark,
    })
    ElMessage.success('已保存')
    visible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '保存失败')
  }
}

async function remove(row: any) {
  await ElMessageBox.confirm('确认删除该映射？', '提示', { type: 'warning' })
  await api.deletePimMapping(row.id)
  ElMessage.success('已删除')
  await load()
}
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header>
        <div class="hdr">
          <span>PIM 映射</span>
          <el-button type="primary" :icon="Plus" @click="openCreate">新建映射</el-button>
        </div>
      </template>
      <el-table :data="list" border stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="invSkuId" label="仓配 SKU ID" width="130" />
        <el-table-column prop="pimSkuId" label="PIM SKU ID" width="130" />
        <el-table-column prop="pimSkuCode" label="PIM SKU 编码" min-width="160" />
        <el-table-column prop="remark" label="备注" min-width="140" show-overflow-tooltip />
        <el-table-column prop="updatedAt" label="更新时间" width="170" />
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

    <el-dialog v-model="visible" :title="form.id ? '编辑映射' : '新建映射'" width="480px">
      <el-form :model="form" label-width="120px">
        <el-form-item label="仓配 SKU ID" required>
          <el-input-number v-model="form.invSkuId" :min="1" style="width: 100%" />
        </el-form-item>
        <el-form-item label="PIM SKU ID" required>
          <el-input-number v-model="form.pimSkuId" :min="1" style="width: 100%" />
        </el-form-item>
        <el-form-item label="PIM SKU 编码">
          <el-input v-model="form.pimSkuCode" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" />
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
.pager { margin-top: 16px; justify-content: flex-end; }
</style>
