<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { api } from '../../api/wms'

const loading = ref(false)
const list = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')
const selected = ref<any[]>([])
const dialogVisible = ref(false)
const form = ref<any>({})

async function load() {
  loading.value = true
  try {
    const res = await api.listProfitTrials({
      page: page.value,
      pageSize: pageSize.value,
      keyword: keyword.value || undefined,
    })
    list.value = res.list || []
    total.value = res.total || 0
  } catch (e) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(load)

function onSelectionChange(rows: any[]) {
  selected.value = rows
}

function search() {
  page.value = 1
  load()
}

function openCreate() {
  form.value = {
    parentSku: '',
    sku: '',
    shopSku: '',
    shopName: '',
    skuName: '',
    retailPrice: 0,
    priceUs: 0,
    price: 0,
    costPrice: 0,
    platformFreight: 0,
    headFreight: 0,
    freight: 0,
    packageFee: 0,
    tariff: 0,
    profitMargin: 0,
    asin: '',
    remark: '',
  }
  dialogVisible.value = true
}

function openEdit(row: any) {
  form.value = { ...row }
  dialogVisible.value = true
}

async function save() {
  if (!String(form.value.sku || '').trim()) {
    ElMessage.warning('请填写库存SKU')
    return
  }
  try {
    if (form.value.id) await api.updateProfitTrial(form.value.id, form.value)
    else await api.createProfitTrial(form.value)
    ElMessage.success('已保存')
    dialogVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '保存失败')
  }
}

async function removeSelected() {
  if (!selected.value.length) {
    ElMessage.warning('请先勾选')
    return
  }
  await ElMessageBox.confirm(`确认删除 ${selected.value.length} 条？`, '提示', { type: 'warning' })
  await api.deleteProfitTrials(selected.value.map((r) => r.id))
  ElMessage.success('已删除')
  await load()
}

async function calc(mode: 'by_cost' | 'by_margin') {
  if (!selected.value.length) {
    ElMessage.warning('请先勾选要计算的行')
    return
  }
  try {
    await api.calcProfitTrials({
      ids: selected.value.map((r) => r.id),
      mode,
    })
    ElMessage.success(mode === 'by_margin' ? '已按利润率反推售价' : '已计算利润')
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '计算失败')
  }
}

async function fillFromSku() {
  const code = String(form.value.sku || '').trim()
  if (!code) return
  try {
    const s = await api.getSkuByCode(code)
    form.value.parentSku = s.parentSku || form.value.parentSku
    form.value.skuName = s.pickName || s.productName || form.value.skuName
    form.value.costPrice = Number(s.lastPurchasePrice) || form.value.costPrice
    form.value.price = Number(s.retailPrice) || form.value.price
    form.value.asin = s.asin || form.value.asin
  } catch {
    /* optional */
  }
}
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header>
        <div class="hdr">
          <span>商品利润试算</span>
          <div class="actions">
            <el-button type="primary" :icon="Plus" @click="openCreate">新增</el-button>
            <el-button type="danger" plain @click="removeSelected">删除</el-button>
            <el-button type="success" @click="calc('by_cost')">计算利润</el-button>
            <el-button @click="calc('by_margin')">根据利润率计算售价</el-button>
          </div>
        </div>
      </template>

      <div class="toolbar">
        <el-input
          v-model="keyword"
          clearable
          placeholder="父SKU / 库存SKU / 店铺SKU / 名称"
          :prefix-icon="Search"
          style="width: 320px"
          @keyup.enter="search"
          @clear="search"
        />
        <el-button type="primary" @click="search">查询</el-button>
      </div>

      <el-table :data="list" border stripe @selection-change="onSelectionChange">
        <el-table-column type="selection" width="48" />
        <el-table-column prop="parentSku" label="父SKU" width="110" />
        <el-table-column prop="sku" label="库存SKU" width="120" />
        <el-table-column prop="shopSku" label="店铺SKU" width="120" />
        <el-table-column prop="shopName" label="店铺简称" width="110" />
        <el-table-column prop="skuName" label="商品名称" min-width="140" show-overflow-tooltip />
        <el-table-column prop="retailPrice" label="零售价格($)" width="110" />
        <el-table-column prop="priceUs" label="产品售价($)" width="110" />
        <el-table-column prop="price" label="产品售价(￥)" width="110" />
        <el-table-column prop="costPrice" label="商品成本(￥)" width="110" />
        <el-table-column prop="platformFreight" label="平台交易费" width="100" />
        <el-table-column prop="headFreight" label="头程运费" width="100" />
        <el-table-column prop="freight" label="运费" width="90" />
        <el-table-column prop="packageFee" label="包装费" width="90" />
        <el-table-column prop="tariff" label="关税" width="90" />
        <el-table-column prop="profit" label="利润(￥)" width="100" />
        <el-table-column prop="profitMargin" label="利润率(%)" width="100" />
        <el-table-column prop="asin" label="ASIN" width="120" />
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        class="pager"
        layout="total, sizes, prev, pager, next"
        :total="total"
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :page-sizes="[20, 50, 100]"
        @current-change="load"
        @size-change="() => { page = 1; load() }"
      />
    </el-card>

    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑试算' : '新增试算'" width="720px" destroy-on-close>
      <el-form :model="form" label-width="120px">
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="库存SKU" required>
              <el-input v-model="form.sku" @blur="fillFromSku" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="父SKU"><el-input v-model="form.parentSku" /></el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="店铺SKU"><el-input v-model="form.shopSku" /></el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="店铺简称"><el-input v-model="form.shopName" /></el-form-item>
          </el-col>
          <el-col :span="24">
            <el-form-item label="商品名称"><el-input v-model="form.skuName" /></el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="零售价格($)">
              <el-input-number v-model="form.retailPrice" :min="0" :precision="4" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="产品售价($)">
              <el-input-number v-model="form.priceUs" :min="0" :precision="4" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="产品售价(￥)">
              <el-input-number v-model="form.price" :min="0" :precision="4" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="商品成本(￥)">
              <el-input-number v-model="form.costPrice" :min="0" :precision="4" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="平台交易费">
              <el-input-number v-model="form.platformFreight" :min="0" :precision="4" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="头程运费">
              <el-input-number v-model="form.headFreight" :min="0" :precision="4" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="运费">
              <el-input-number v-model="form.freight" :min="0" :precision="4" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="包装费">
              <el-input-number v-model="form.packageFee" :min="0" :precision="4" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="关税">
              <el-input-number v-model="form.tariff" :min="0" :precision="4" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="目标利润率(%)">
              <el-input-number v-model="form.profitMargin" :precision="4" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="ASIN"><el-input v-model="form.asin" /></el-form-item>
          </el-col>
          <el-col :span="24">
            <el-form-item label="备注"><el-input v-model="form.remark" /></el-form-item>
          </el-col>
        </el-row>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; }
.toolbar { display: flex; gap: 8px; margin-bottom: 12px; }
.pager { margin-top: 16px; justify-content: flex-end; }
</style>
