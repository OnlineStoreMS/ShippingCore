<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import { api } from '../../api/wms'

const router = useRouter()
const loading = ref(false)
const list = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')

async function load() {
  loading.value = true
  try {
    const res = await api.listPimMappings({
      page: page.value,
      pageSize: pageSize.value,
    })
    let rows = res.list || []
    if (keyword.value.trim()) {
      const kw = keyword.value.trim().toLowerCase()
      rows = rows.filter((r: any) =>
        String(r.pimSkuCode || '').toLowerCase().includes(kw)
        || String(r.pimSkuId || '').includes(kw)
        || String(r.invSkuId || '').includes(kw)
        || String(r.remark || '').toLowerCase().includes(kw),
      )
    }
    list.value = rows
    total.value = keyword.value.trim() ? rows.length : (res.total || 0)
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
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header>
        <div class="hdr">
          <span>店铺SKU明细</span>
          <span class="hint">对齐普源：店铺/渠道 SKU 与库存 SKU 对照（当前数据来自 PIM 映射，可在「对接预留」维护）</span>
        </div>
      </template>

      <div class="toolbar">
        <el-input
          v-model="keyword"
          clearable
          placeholder="搜索店铺SKU编码 / ID / 库存SKU ID"
          :prefix-icon="Search"
          style="width: 320px"
          @keyup.enter="search"
          @clear="search"
        />
        <el-button type="primary" @click="search">查询</el-button>
        <el-button @click="router.push('/pim-mappings')">维护映射</el-button>
      </div>

      <el-table :data="list" border stripe>
        <el-table-column prop="invSkuId" label="库存SKU ID" width="120" />
        <el-table-column prop="pimSkuId" label="店铺/PIM SKU ID" width="150" />
        <el-table-column prop="pimSkuCode" label="店铺SKU编码" min-width="160" />
        <el-table-column prop="remark" label="备注" min-width="160" show-overflow-tooltip />
        <el-table-column prop="updatedAt" label="更新时间" width="170">
          <template #default="{ row }">
            {{ row.updatedAt ? String(row.updatedAt).slice(0, 19).replace('T', ' ') : '-' }}
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
  </div>
</template>

<style scoped>
.hdr { display: flex; flex-direction: column; gap: 4px; }
.hint { font-size: 12px; color: #909399; font-weight: 400; }
.toolbar { display: flex; gap: 8px; margin-bottom: 12px; flex-wrap: wrap; }
.pager { margin-top: 16px; justify-content: flex-end; }
</style>
