<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  shippingApi,
  type KdzsAccountDetail,
} from '../api/shipping'

const loading = ref(false)
const syncing = ref(false)
const items = ref<KdzsAccountDetail[]>([])
const dialogVisible = ref(false)
const editingCode = ref<string | null>(null)
const form = reactive({
  code: '',
  name: '',
  role: 'merchant',
  mobile: '',
  password: '',
  enabled: true,
})

async function load() {
  loading.value = true
  try {
    const data = await shippingApi.listKdzsAccountDetails()
    items.value = data.items || []
  } catch (e) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function syncFromSSA() {
  syncing.value = true
  try {
    const stats = await shippingApi.syncKdzsAccounts()
    ElMessage.success(`已从 StoreSyncAgent 同步 ${stats.synced ?? 0} 个账号`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '同步失败')
  } finally {
    syncing.value = false
  }
}

function openCreate() {
  editingCode.value = null
  form.code = ''
  form.name = ''
  form.role = 'merchant'
  form.mobile = ''
  form.password = ''
  form.enabled = true
  dialogVisible.value = true
}

function openEdit(row: KdzsAccountDetail) {
  editingCode.value = row.code
  form.code = row.code
  form.name = row.name
  form.role = row.role
  form.mobile = row.mobile
  form.password = ''
  form.enabled = row.enabled
  dialogVisible.value = true
}

async function submit() {
  try {
    if (editingCode.value) {
      await shippingApi.updateKdzsAccount(editingCode.value, {
        name: form.name,
        role: form.role,
        mobile: form.mobile,
        password: form.password,
        enabled: form.enabled,
      })
      ElMessage.success('已更新')
    } else {
      await shippingApi.createKdzsAccount({
        code: form.code,
        name: form.name,
        role: form.role,
        mobile: form.mobile,
        password: form.password,
        enabled: form.enabled,
      })
      ElMessage.success('已添加')
    }
    dialogVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '保存失败')
  }
}

async function onDelete(row: KdzsAccountDetail) {
  try {
    await ElMessageBox.confirm(`确定删除账号 ${row.name}？（仅删除发货中心本地，不影响 StoreSyncAgent）`, '删除确认', { type: 'warning' })
    await shippingApi.deleteKdzsAccount(row.code)
    ElMessage.success('已删除')
    await load()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error((e as Error).message || '删除失败')
  }
}

async function onSetDefault(row: KdzsAccountDetail) {
  try {
    await shippingApi.setDefaultKdzsAccount(row.code)
    ElMessage.success('已设为发货中心默认账号')
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '操作失败')
  }
}

async function onSwitch(row: KdzsAccountDetail) {
  try {
    await shippingApi.switchKdzsAccount(row.code)
    ElMessage.success('已切换当前账号')
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '切换失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <div class="title-block">
            <span>账号管理</span>
            <span class="hint">默认自动同步 StoreSyncAgent 账号；也可在此单独维护（两边可不一致）</span>
          </div>
          <div class="actions">
            <el-button :loading="syncing" @click="syncFromSSA">从 StoreSyncAgent 同步</el-button>
            <el-button type="primary" @click="openCreate">添加账号</el-button>
          </div>
        </div>
      </template>

      <el-alert
        type="info"
        :closable="false"
        show-icon
        class="tip"
        title="进入本页会自动拉取 StoreSyncAgent 已配置账号（无需先手动添加）。默认/当前账号可与 StoreSyncAgent 不同；手动同步会重新采用对方的默认设置。"
      />

      <el-table :data="items" v-loading="loading" stripe border empty-text="暂无账号，请同步或添加">
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column prop="code" label="账号 ID" min-width="160" />
        <el-table-column prop="mobile" label="手机号" width="130" />
        <el-table-column prop="roleLabel" label="类型" width="90" />
        <el-table-column prop="sourceLabel" label="来源" width="130">
          <template #default="{ row }">
            <el-tag size="small" :type="row.source === 'local' ? 'warning' : 'success'" effect="plain">
              {{ row.sourceLabel || (row.source === 'local' ? '本地' : 'StoreSyncAgent') }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="180">
          <template #default="{ row }">
            <el-tag v-if="row.active" type="success" size="small">当前使用</el-tag>
            <el-tag v-if="row.isDefault" type="primary" size="small" effect="plain">默认</el-tag>
            <el-tag v-if="!row.enabled" type="info" size="small">已禁用</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="onSwitch(row)">切换</el-button>
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button v-if="!row.isDefault" link type="primary" @click="onSetDefault(row)">设默认</el-button>
            <el-button link type="danger" @click="onDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="dialogVisible"
      :title="editingCode ? '编辑账号' : '添加快递助手账号'"
      width="520px"
      destroy-on-close
    >
      <el-form label-width="96px">
        <el-form-item label="账号 ID" required>
          <el-input v-model="form.code" :disabled="!!editingCode" placeholder="如 account_13107749258" />
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="显示名称，默认同手机号" />
        </el-form-item>
        <el-form-item label="手机号" required>
          <el-input v-model="form.mobile" />
        </el-form-item>
        <el-form-item label="密码" :required="!editingCode">
          <el-input v-model="form.password" type="password" show-password :placeholder="editingCode ? '留空则不修改' : ''" />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="form.role" style="width: 100%">
            <el-option label="商家版" value="merchant" />
            <el-option label="厂家版" value="factory" />
          </el-select>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 12px; }
.hdr { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.title-block { display: flex; flex-direction: column; gap: 4px; }
.hint { color: #909399; font-size: 12px; font-weight: 400; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; }
.tip { margin-bottom: 12px; }
</style>
