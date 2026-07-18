<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { shippingApi, type ShipperProfile } from '../api/shipping'

const loading = ref(false)
const list = ref<ShipperProfile[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')
const visible = ref(false)
const form = ref<ShipperProfile>(emptyForm())

function emptyForm(): ShipperProfile {
  return {
    name: '',
    company: '',
    mobile: '',
    province: '',
    city: '',
    county: '',
    address: '',
    isDefault: false,
    enabled: true,
  }
}

async function load() {
  loading.value = true
  try {
    const res = await shippingApi.listShipperProfiles({
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

function openEdit(row: ShipperProfile) {
  form.value = { ...row }
  visible.value = true
}

async function save() {
  if (!form.value.name.trim() || !form.value.mobile.trim() || !form.value.address.trim()) {
    ElMessage.warning('请填写姓名、手机号和地址')
    return
  }
  try {
    if (form.value.id) {
      await shippingApi.updateShipperProfile(form.value.id, form.value)
    } else {
      await shippingApi.createShipperProfile(form.value)
    }
    ElMessage.success('已保存')
    visible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '保存失败')
  }
}

async function remove(row: ShipperProfile) {
  await ElMessageBox.confirm(`确认删除寄件人「${row.name}」？`, '提示', { type: 'warning' })
  await shippingApi.deleteShipperProfile(row.id!)
  ElMessage.success('已删除')
  await load()
}

async function setDefault(row: ShipperProfile) {
  try {
    await shippingApi.setDefaultShipperProfile(row.id!)
    ElMessage.success('已设为默认')
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '设置失败')
  }
}

function fullAddress(row: ShipperProfile) {
  return [row.province, row.city, row.county, row.address].filter(Boolean).join('')
}

onMounted(load)
</script>

<template>
  <div class="page">
    <el-card v-loading="loading">
      <template #header>
        <div class="hdr">
          <span>寄件人</span>
          <el-button type="primary" :icon="Plus" @click="openCreate">新增寄件人</el-button>
        </div>
      </template>

      <div class="toolbar">
        <el-input
          v-model="keyword"
          clearable
          placeholder="姓名 / 手机号"
          :prefix-icon="Search"
          style="width: 220px"
          @change="search"
        />
        <el-button type="primary" @click="search">查询</el-button>
      </div>

      <el-table :data="list" border stripe>
        <el-table-column prop="name" label="姓名" width="120" />
        <el-table-column prop="company" label="公司" min-width="140" show-overflow-tooltip />
        <el-table-column prop="mobile" label="手机号" width="130" />
        <el-table-column label="地址" min-width="240" show-overflow-tooltip>
          <template #default="{ row }">{{ fullAddress(row) }}</template>
        </el-table-column>
        <el-table-column label="默认" width="80" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.isDefault" type="success" size="small">默认</el-tag>
            <el-button v-else link type="primary" size="small" @click="setDefault(row)">设为默认</el-button>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'" size="small">{{ row.enabled ? '是' : '否' }}</el-tag>
          </template>
        </el-table-column>
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

    <el-dialog v-model="visible" :title="form.id ? '编辑寄件人' : '新增寄件人'" width="520px">
      <el-form label-width="80px">
        <el-form-item label="姓名" required>
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="公司">
          <el-input v-model="form.company" />
        </el-form-item>
        <el-form-item label="手机号" required>
          <el-input v-model="form.mobile" />
        </el-form-item>
        <el-form-item label="省">
          <el-input v-model="form.province" />
        </el-form-item>
        <el-form-item label="市">
          <el-input v-model="form.city" />
        </el-form-item>
        <el-form-item label="区/县">
          <el-input v-model="form.county" />
        </el-form-item>
        <el-form-item label="详细地址" required>
          <el-input v-model="form.address" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item label="默认">
          <el-switch v-model="form.isDefault" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
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
