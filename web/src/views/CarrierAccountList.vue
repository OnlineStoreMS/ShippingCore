<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { maskCheckword, shippingApi, type CarrierAccount } from '../api/shipping'

const loading = ref(false)
const list = ref<CarrierAccount[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')
const visible = ref(false)
const form = ref<CarrierAccount>(emptyForm())

const envOptions = [
  { label: '沙箱', value: 'sandbox' },
  { label: '生产', value: 'prod' },
]

function emptyForm(): CarrierAccount {
  return {
    carrierCode: 'SF',
    name: '',
    partnerId: '',
    checkword: '',
    useMonthly: false,
    custId: '',
    expressType: '2',
    env: 'sandbox',
    enabled: true,
    remark: '',
  }
}

async function load() {
  loading.value = true
  try {
    const res = await shippingApi.listCarrierAccounts({
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

function search() {
  page.value = 1
  load()
}

function openCreate() {
  form.value = emptyForm()
  visible.value = true
}

function openEdit(row: CarrierAccount) {
  form.value = { ...row, checkword: '' }
  visible.value = true
}

async function save() {
  if (!form.value.name.trim() || !form.value.partnerId.trim()) {
    ElMessage.warning('请填写名称和客户编码')
    return
  }
  if (!form.value.id && !form.value.checkword?.trim()) {
    ElMessage.warning('请填写校验码')
    return
  }
  try {
    const payload = { ...form.value }
    if (form.value.id && !payload.checkword) {
      delete payload.checkword
    }
    if (form.value.id) {
      await shippingApi.updateCarrierAccount(form.value.id, payload)
    } else {
      await shippingApi.createCarrierAccount(payload)
    }
    ElMessage.success('已保存')
    visible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '保存失败')
  }
}

async function remove(row: CarrierAccount) {
  await ElMessageBox.confirm(`确认删除物流账号「${row.name}」？`, '提示', { type: 'warning' })
  await shippingApi.deleteCarrierAccount(row.id!)
  ElMessage.success('已删除')
  await load()
}

function envLabel(env: string) {
  return envOptions.find((o) => o.value === env)?.label || env
}

onMounted(load)
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header>
        <div class="hdr">
          <span>物流账号</span>
          <el-button type="primary" :icon="Plus" @click="openCreate">新增账号</el-button>
        </div>
      </template>

      <div class="toolbar">
        <el-input
          v-model="keyword"
          clearable
          placeholder="名称 / 客户编码"
          :prefix-icon="Search"
          style="width: 220px"
          @change="search"
        />
        <el-button type="primary" @click="search">查询</el-button>
      </div>

      <el-table :data="list" border stripe>
        <el-table-column prop="name" label="名称" min-width="140" />
        <el-table-column prop="partnerId" label="客户编码" min-width="140" />
        <el-table-column label="校验码" min-width="120">
          <template #default="{ row }">{{ maskCheckword(row.checkword) }}</template>
        </el-table-column>
        <el-table-column label="月结" width="80" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.useMonthly" type="success" size="small">是</el-tag>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="custId" label="月结卡号" min-width="120" show-overflow-tooltip />
        <el-table-column prop="expressType" label="快件类型" width="100" />
        <el-table-column label="环境" width="80">
          <template #default="{ row }">{{ envLabel(row.env) }}</template>
        </el-table-column>
        <el-table-column label="启用" width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'" size="small">{{ row.enabled ? '是' : '否' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="remark" label="备注" min-width="140" show-overflow-tooltip />
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" size="small" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pager">
        <el-pagination
          v-model:current-page="page"
          :page-size="pageSize"
          :total="total"
          layout="total, prev, pager, next"
          @current-change="load"
        />
      </div>
    </el-card>

    <el-dialog v-model="visible" :title="form.id ? '编辑物流账号' : '新增物流账号'" width="520px">
      <el-form label-width="100px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="账号名称" />
        </el-form-item>
        <el-form-item label="客户编码" required>
          <el-input v-model="form.partnerId" placeholder="partnerId" />
        </el-form-item>
        <el-form-item :label="form.id ? '校验码' : '校验码'" :required="!form.id">
          <el-input
            v-model="form.checkword"
            type="password"
            show-password
            :placeholder="form.id ? '留空则不修改' : 'checkword'"
          />
        </el-form-item>
        <el-form-item label="月结">
          <el-switch v-model="form.useMonthly" />
        </el-form-item>
        <el-form-item label="月结卡号">
          <el-input v-model="form.custId" placeholder="月结时必填" />
        </el-form-item>
        <el-form-item label="快件类型">
          <el-input v-model="form.expressType" placeholder="默认 2" />
        </el-form-item>
        <el-form-item label="环境">
          <el-radio-group v-model="form.env">
            <el-radio v-for="opt in envOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" type="textarea" :rows="2" />
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
.hdr { display: flex; align-items: center; justify-content: space-between; }
.toolbar { display: flex; gap: 8px; margin-bottom: 16px; }
.pager { margin-top: 16px; display: flex; justify-content: flex-end; }
</style>
