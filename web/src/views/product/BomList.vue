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
const form = ref<any>({ parentSkuId: undefined, bomType: 'combo', remark: '', items: [] })

async function load() {
  loading.value = true
  try {
    const res = await api.listBoms({ page: page.value, pageSize: pageSize.value })
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
  form.value = { parentSkuId: undefined, bomType: 'combo', remark: '', status: 1, items: [{ childSkuId: undefined, qty: 1 }] }
  visible.value = true
}

async function openEdit(row: any) {
  try {
    const detail = await api.getBom(row.id)
    form.value = {
      id: detail.id,
      parentSkuId: detail.parentSkuId,
      bomType: detail.bomType,
      remark: detail.remark,
      status: detail.status,
      items: (detail.items || []).map((i: any) => ({ childSkuId: i.childSkuId, qty: i.qty, remark: i.remark })),
    }
    visible.value = true
  } catch (e) {
    ElMessage.error((e as Error).message || '加载详情失败')
  }
}

function addItem() {
  form.value.items.push({ childSkuId: undefined, qty: 1 })
}

function removeItem(idx: number) {
  form.value.items.splice(idx, 1)
}

async function save() {
  try {
    await api.saveBom({
      parentSkuId: form.value.parentSkuId,
      bomType: form.value.bomType,
      remark: form.value.remark,
      status: form.value.status ?? 1,
      items: form.value.items,
    })
    ElMessage.success('已保存')
    visible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '保存失败')
  }
}

async function remove(row: any) {
  await ElMessageBox.confirm('确认删除该 BOM？', '提示', { type: 'warning' })
  await api.deleteBom(row.id)
  ElMessage.success('已删除')
  await load()
}

const bomTypeLabel: Record<string, string> = { combo: '组合品', assembly: '组装品' }
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header>
        <div class="hdr">
          <span>组合/组装品</span>
          <el-button type="primary" :icon="Plus" @click="openCreate">新建 BOM</el-button>
        </div>
      </template>
      <el-table :data="list" border stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="parentSkuId" label="父SKU ID" width="120" />
        <el-table-column label="类型" width="100">
          <template #default="{ row }">{{ bomTypeLabel[row.bomType] || row.bomType }}</template>
        </el-table-column>
        <el-table-column prop="remark" label="备注" min-width="160" show-overflow-tooltip />
        <el-table-column label="明细数" width="90">
          <template #default="{ row }">{{ (row.items || []).length }}</template>
        </el-table-column>
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

    <el-dialog v-model="visible" title="保存 BOM" width="640px">
      <el-form :model="form" label-width="110px">
        <el-form-item label="父SKU ID" required>
          <el-input-number v-model="form.parentSkuId" :min="1" style="width: 100%" />
        </el-form-item>
        <el-form-item label="BOM类型" required>
          <el-radio-group v-model="form.bomType">
            <el-radio-button value="combo">组合品</el-radio-button>
            <el-radio-button value="assembly">组装品</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" />
        </el-form-item>
        <el-form-item label="子件明细">
          <div class="items">
            <div v-for="(item, idx) in form.items" :key="idx" class="item-row">
              <el-input-number v-model="item.childSkuId" :min="1" placeholder="子SKU ID" controls-position="right" />
              <el-input-number v-model="item.qty" :min="0.0001" :step="1" placeholder="数量" controls-position="right" />
              <el-button link type="danger" @click="removeItem(idx)">移除</el-button>
            </div>
            <el-button type="primary" link @click="addItem">+ 添加子件</el-button>
          </div>
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
.items { width: 100%; display: flex; flex-direction: column; gap: 8px; }
.item-row { display: flex; gap: 8px; align-items: center; }
</style>
