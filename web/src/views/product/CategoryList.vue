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
const keyword = ref('')
const visible = ref(false)
const form = ref<any>({})

const categoryMap = computed(() => {
  const m = new Map<number, string>()
  for (const c of list.value) m.set(c.id, c.name)
  return m
})

async function load() {
  loading.value = true
  try {
    const res = await api.listCategories({
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

function openCreate() {
  form.value = { code: '', name: '', aliasCn: '', aliasEn: '', parentId: 0, sort: 0, status: 1 }
  visible.value = true
}

function openEdit(row: any) {
  form.value = { ...row }
  visible.value = true
}

async function save() {
  if (!String(form.value.name || '').trim()) {
    ElMessage.warning('请填写商品类别名称')
    return
  }
  try {
    if (form.value.id) {
      await api.updateCategory(form.value.id, form.value)
    } else {
      await api.createCategory(form.value)
    }
    ElMessage.success('已保存')
    visible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '保存失败')
  }
}

async function remove(row: any) {
  await ElMessageBox.confirm(`确认删除类别「${row.name}」？`, '提示', { type: 'warning' })
  await api.deleteCategory(row.id)
  ElMessage.success('已删除')
  await load()
}

function parentName(id?: number) {
  if (!id) return '-'
  return categoryMap.value.get(id) || String(id)
}

function search() {
  page.value = 1
  load()
}
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header>
        <div class="hdr">
          <span>商品类别</span>
          <el-button type="primary" :icon="Plus" @click="openCreate">新增</el-button>
        </div>
      </template>
      <div class="toolbar">
        <el-input
          v-model="keyword"
          placeholder="类别编码 / 名称"
          clearable
          :prefix-icon="Search"
          style="width: 240px"
          @change="search"
        />
        <el-button type="primary" @click="search">查询</el-button>
      </div>
      <el-table :data="list" border stripe>
        <el-table-column prop="code" label="编码" width="120" />
        <el-table-column prop="name" label="商品类别" min-width="160" />
        <el-table-column prop="aliasCn" label="中文品名" min-width="140" show-overflow-tooltip />
        <el-table-column prop="aliasEn" label="英文品名" min-width="140" show-overflow-tooltip />
        <el-table-column label="父级类别" width="140">
          <template #default="{ row }">{{ parentName(row.parentId) }}</template>
        </el-table-column>
        <el-table-column prop="sort" label="排序" width="80" />
        <el-table-column label="状态" width="80">
          <template #default="{ row }">{{ row.status === 1 ? '启用' : '停用' }}</template>
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

    <el-dialog v-model="visible" :title="form.id ? '编辑商品类别' : '新增商品类别'" width="520px" destroy-on-close>
      <el-form :model="form" label-width="110px">
        <el-form-item label="编码">
          <el-input
            v-model="form.code"
            :placeholder="form.id ? '' : '留空自动生成，如 CAT0001'"
            :disabled="!!form.id"
          />
        </el-form-item>
        <el-form-item label="商品类别" required><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="中文品名"><el-input v-model="form.aliasCn" /></el-form-item>
        <el-form-item label="英文品名"><el-input v-model="form.aliasEn" /></el-form-item>
        <el-form-item label="父级类别">
          <el-select v-model="form.parentId" clearable style="width: 100%">
            <el-option :value="0" label="无（顶级）" />
            <el-option
              v-for="c in list.filter((x) => x.id !== form.id)"
              :key="c.id"
              :label="c.name"
              :value="c.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="排序"><el-input-number v-model="form.sort" :min="0" style="width: 100%" /></el-form-item>
        <el-form-item label="状态">
          <el-select v-model="form.status" style="width: 100%">
            <el-option :value="1" label="启用" />
            <el-option :value="0" label="停用" />
          </el-select>
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
.toolbar { display: flex; gap: 8px; margin-bottom: 12px; }
.pager { margin-top: 16px; justify-content: flex-end; }
</style>
