<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { api } from '../../api/wms'
import SkuSearchSelect from '../../components/SkuSearchSelect.vue'

const props = defineProps<{ bomType: 'combo' | 'assembly' }>()

const route = useRoute()
const router = useRouter()
const loading = ref(false)
const list = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')

const title = computed(() => (props.bomType === 'combo' ? '组合品明细' : '组装品明细'))
const typeLabel = computed(() => (props.bomType === 'combo' ? '组合品' : '组装品'))

const dialogVisible = ref(false)
const form = ref<any>({ parentSkuId: undefined, bomType: props.bomType, remark: '', items: [] })

async function load() {
  loading.value = true
  try {
    const res = await api.listSkus({
      page: page.value,
      pageSize: pageSize.value,
      keyword: keyword.value || undefined,
      productType: props.bomType,
    })
    const rows = res.list || []
    total.value = res.total
    // attach BOM if any
    const bomRes = await api.listBoms({ page: 1, pageSize: 500, bomType: props.bomType }).catch(() => ({ list: [] as any[] }))
    const bomByParent = new Map<number, any>()
    for (const b of bomRes.list || []) bomByParent.set(b.parentSkuId, b)
    list.value = rows.map((s: any) => ({
      ...s,
      bom: bomByParent.get(s.id) || null,
    }))
  } catch (e) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(() => props.bomType, () => {
  page.value = 1
  load()
})
watch(() => route.path, () => {
  page.value = 1
  load()
})

function search() {
  page.value = 1
  load()
}

function openCreate() {
  form.value = {
    parentSkuId: undefined,
    bomType: props.bomType,
    remark: '',
    status: 1,
    items: [{ childSkuId: undefined, qty: 1 }],
  }
  dialogVisible.value = true
}

async function openEditBom(row: any) {
  if (row.bom?.id) {
    try {
      const detail = await api.getBom(row.bom.id)
      form.value = {
        id: detail.id,
        parentSkuId: detail.parentSkuId,
        bomType: detail.bomType || props.bomType,
        remark: detail.remark,
        status: detail.status,
        items: (detail.items || []).map((i: any) => ({
          childSkuId: i.childSkuId,
          qty: i.qty,
          remark: i.remark,
        })),
      }
      dialogVisible.value = true
      return
    } catch (e) {
      ElMessage.error((e as Error).message || '加载 BOM 失败')
      return
    }
  }
  form.value = {
    parentSkuId: row.id,
    bomType: props.bomType,
    remark: '',
    status: 1,
    items: [{ childSkuId: undefined, qty: 1 }],
  }
  dialogVisible.value = true
}

function addItem() {
  form.value.items.push({ childSkuId: undefined, qty: 1 })
}

function removeItem(idx: number) {
  form.value.items.splice(idx, 1)
}

async function saveBom() {
  if (!form.value.parentSkuId) {
    ElMessage.warning('请选择父库存SKU')
    return
  }
  try {
    await api.saveBom({
      parentSkuId: form.value.parentSkuId,
      bomType: props.bomType,
      remark: form.value.remark,
      status: form.value.status ?? 1,
      items: form.value.items,
    })
    ElMessage.success('已保存')
    dialogVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '保存失败')
  }
}

async function removeBom(row: any) {
  if (!row.bom?.id) return
  await ElMessageBox.confirm('确认删除该 BOM？', '提示', { type: 'warning' })
  await api.deleteBom(row.bom.id)
  ElMessage.success('已删除')
  await load()
}

function goProduct(row: any) {
  if (row.parentId) router.push({ path: '/products', query: { edit: String(row.parentId) } })
}
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header>
        <div class="hdr">
          <span>{{ title }}</span>
          <el-button type="primary" :icon="Plus" @click="openCreate">维护 BOM</el-button>
        </div>
      </template>

      <div class="toolbar">
        <el-input
          v-model="keyword"
          clearable
          :placeholder="`搜索${typeLabel} SKU / 配货名`"
          :prefix-icon="Search"
          style="width: 280px"
          @keyup.enter="search"
          @clear="search"
        />
        <el-button type="primary" @click="search">查询</el-button>
      </div>

      <el-table :data="list" border stripe row-key="id">
        <el-table-column type="expand">
          <template #default="{ row }">
            <div v-if="row.bom?.items?.length" class="bom-expand">
              <div class="bom-title">子件明细</div>
              <el-table :data="row.bom.items" border size="small">
                <el-table-column prop="childSkuId" label="子SKU ID" width="120" />
                <el-table-column prop="qty" label="数量" width="100" />
                <el-table-column prop="remark" label="备注" min-width="160" />
              </el-table>
            </div>
            <div v-else class="bom-empty">暂无 BOM 子件，可点击「维护 BOM」配置</div>
          </template>
        </el-table-column>
        <el-table-column prop="skuCode" label="库存SKU" width="160" />
        <el-table-column prop="pickName" label="配货名称" min-width="160" show-overflow-tooltip />
        <el-table-column prop="parentSku" label="父SKU" width="140" />
        <el-table-column prop="productName" label="商品名称" min-width="160" show-overflow-tooltip />
        <el-table-column label="类型" width="90">
          <template #default>{{ typeLabel }}</template>
        </el-table-column>
        <el-table-column label="子件数" width="90">
          <template #default="{ row }">{{ row.bom?.items?.length || 0 }}</template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            {{ ({ active: '在售', inactive: '停用', clearance: '清仓' } as Record<string, string>)[row.status] || row.status }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEditBom(row)">维护 BOM</el-button>
            <el-button v-if="row.bom?.id" link type="danger" @click="removeBom(row)">删 BOM</el-button>
            <el-button link @click="goProduct(row)">商品</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        class="pager"
        layout="total, sizes, prev, pager, next"
        :total="total"
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :page-sizes="[10, 20, 50]"
        @current-change="load"
        @size-change="() => { page = 1; load() }"
      />
    </el-card>

    <el-dialog v-model="dialogVisible" :title="`维护${typeLabel} BOM`" width="680px" destroy-on-close>
      <el-form :model="form" label-width="110px">
        <el-form-item label="父库存SKU" required>
          <SkuSearchSelect v-model="form.parentSkuId" :placeholder="`选择${typeLabel}库存SKU`" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" />
        </el-form-item>
        <el-form-item label="子件明细">
          <div class="items">
            <div v-for="(item, idx) in form.items" :key="idx" class="item-row">
              <SkuSearchSelect v-model="item.childSkuId" placeholder="子SKU" style="flex: 1" />
              <el-input-number v-model="item.qty" :min="0.0001" :step="1" placeholder="数量" controls-position="right" />
              <el-button link type="danger" @click="removeItem(idx)">移除</el-button>
            </div>
            <el-button type="primary" link @click="addItem">+ 添加子件</el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveBom">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.toolbar { display: flex; gap: 8px; margin-bottom: 12px; }
.pager { margin-top: 16px; justify-content: flex-end; }
.bom-expand { padding: 8px 24px 16px; }
.bom-title { font-weight: 600; margin-bottom: 8px; }
.bom-empty { padding: 12px 24px; color: #909399; }
.items { width: 100%; display: flex; flex-direction: column; gap: 8px; }
.item-row { display: flex; gap: 8px; align-items: center; }
</style>
