<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { maskCheckword, shippingApi, type CarrierAccount } from '../api/shipping'

/** 可选物流公司；后续扩展时在此追加即可 */
const carrierOptions = [
  { code: 'SF', name: '顺丰速运' },
]

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

const signModeOptions = [
  { value: 'standard', label: '标准MD5', hint: '先 URLEncode，再 MD5 → Base64' },
  { value: 'simple', label: '简易MD5', hint: '直接 MD5 → Base64（不做 URLEncode）' },
  { value: 'sm3', label: 'SM3', hint: '先 URLEncode，再 SM3 → Base64' },
]

const printChannelOptions = [
  {
    value: 'pdf',
    label: 'PDF 面单',
    hint: '官方体验：云打印转 PDF 后在浏览器打开，用系统打印即可，不依赖 C-Lodop（COM_RECE_CLOUD_PRINT_WAYBILLS）',
  },
  {
    value: 'plugin',
    label: '打印插件',
    hint: '云打印插件排版后经本机 C-Lodop 出纸，需先在「C-Lodop 云打印」选好打印机（COM_RECE_CLOUD_PRINT_PARSEDDATA）',
  },
]

const isSF = computed(() => form.value.carrierCode === 'SF')

function signModeLabel(code?: string) {
  const hit = signModeOptions.find((o) => o.value === code)
  return hit?.label || code || '简易MD5'
}

function printChannelLabel(code?: string) {
  const hit = printChannelOptions.find((o) => o.value === code)
  return hit?.label || code || 'PDF 面单'
}

function emptyForm(): CarrierAccount {
  return {
    carrierCode: '',
    name: '',
    partnerId: '',
    checkword: '',
    useMonthly: false,
    custId: '',
    expressType: '2',
    templateCode: '',
    customTemplateCode: '',
    signMode: 'simple',
    printChannel: 'pdf',
    env: 'sandbox',
    enabled: true,
    remark: '',
  }
}

function carrierName(code?: string) {
  return carrierOptions.find((c) => c.code === code)?.name || code || '-'
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
  form.value = {
    ...row,
    checkword: '',
    signMode: row.signMode || 'simple',
    printChannel: row.printChannel || 'pdf',
  }
  visible.value = true
}

function onCarrierChange() {
  // 切换物流公司时重置公司相关字段默认值（快件类型在「标准寄件」页选择，不在账号固定）
  if (form.value.carrierCode === 'SF') {
    form.value.expressType = form.value.expressType || '2'
    if (!form.value.signMode) form.value.signMode = 'simple'
    if (!form.value.printChannel) form.value.printChannel = 'pdf'
    if (!form.value.env) form.value.env = 'sandbox'
  }
}

async function save() {
  if (!form.value.carrierCode) {
    ElMessage.warning('请选择物流公司')
    return
  }
  if (!form.value.name.trim()) {
    ElMessage.warning('请填写账号名称')
    return
  }
  if (isSF.value) {
    if (!form.value.partnerId.trim()) {
      ElMessage.warning('请填写客户编码')
      return
    }
    if (!form.value.id && !form.value.checkword?.trim()) {
      ElMessage.warning('请填写校验码')
      return
    }
    if (form.value.useMonthly && !form.value.custId?.trim()) {
      ElMessage.warning('启用月结时请填写月结卡号')
      return
    }
    if (!form.value.templateCode?.trim()) {
      ElMessage.warning('请填写云打印模板编码（丰桥控制台面单模板，如 fm_76130_standard_XXXX）')
      return
    }
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
        <el-table-column label="物流公司" width="120">
          <template #default="{ row }">{{ carrierName(row.carrierCode) }}</template>
        </el-table-column>
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
        <el-table-column prop="templateCode" label="面单模板" min-width="180" show-overflow-tooltip />
        <el-table-column label="签名方式" width="100">
          <template #default="{ row }">{{ signModeLabel(row.signMode) }}</template>
        </el-table-column>
        <el-table-column label="打印通道" width="100">
          <template #default="{ row }">{{ printChannelLabel(row.printChannel) }}</template>
        </el-table-column>
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

    <el-dialog v-model="visible" :title="form.id ? '编辑物流账号' : '新增物流账号'" width="560px" destroy-on-close>
      <el-form label-width="100px">
        <el-form-item label="物流公司" required>
          <el-select
            v-model="form.carrierCode"
            placeholder="请选择物流公司"
            style="width: 100%"
            :disabled="!!form.id"
            @change="onCarrierChange"
          >
            <el-option
              v-for="c in carrierOptions"
              :key="c.code"
              :label="c.name"
              :value="c.code"
            />
          </el-select>
        </el-form-item>

        <template v-if="form.carrierCode">
          <el-divider content-position="left">账号信息</el-divider>
          <el-form-item label="名称" required>
            <el-input v-model="form.name" placeholder="账号名称，如：顺丰主账号" />
          </el-form-item>

          <template v-if="isSF">
            <el-form-item label="客户编码" required>
              <el-input v-model="form.partnerId" placeholder="丰桥 partnerId" />
            </el-form-item>
            <el-form-item label="校验码" :required="!form.id">
              <el-input
                v-model="form.checkword"
                type="password"
                show-password
                :placeholder="form.id ? '留空则不修改' : '丰桥 checkword'"
              />
            </el-form-item>
            <el-form-item label="月结">
              <el-switch v-model="form.useMonthly" />
            </el-form-item>
            <el-form-item v-if="form.useMonthly" label="月结卡号" required>
              <el-input v-model="form.custId" placeholder="月结卡号" />
            </el-form-item>
            <el-form-item label="面单模板" required>
              <el-input
                v-model="form.templateCode"
                placeholder="必须归属本顾客编码，如 fm_76130_standard_XSZFMAB1WY1P"
              />
              <div class="hint">
                填标准模板（含顾客编码那段）。不要把 fm_…_custom_… 填到这里，否则会报 not matched the clientCode。
              </div>
            </el-form-item>
            <el-form-item label="自定义模板">
              <el-input
                v-model="form.customTemplateCode"
                placeholder="如 fm_76130_standard_custom_10058011961_1"
              />
              <div class="hint">
                编辑器发布的自定义区模板；变量字段名填 remark。与上方标准模板规格须一致（如均为 76×130）。
              </div>
            </el-form-item>
            <el-form-item label="数字签名" required>
              <el-radio-group v-model="form.signMode">
                <el-radio v-for="opt in signModeOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</el-radio>
              </el-radio-group>
              <div class="hint">
                {{ signModeOptions.find((o) => o.value === form.signMode)?.hint }}
                ；须与丰桥创建应用时选择的方式一致（创建后不可改，不一致会报数字签名无效）。
              </div>
            </el-form-item>
            <el-form-item label="打印通道" required>
              <el-radio-group v-model="form.printChannel">
                <el-radio v-for="opt in printChannelOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</el-radio>
              </el-radio-group>
              <div class="hint">{{ printChannelOptions.find((o) => o.value === form.printChannel)?.hint }}</div>
            </el-form-item>
            <el-form-item label="环境">
              <el-radio-group v-model="form.env">
                <el-radio v-for="opt in envOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</el-radio>
              </el-radio-group>
            </el-form-item>
          </template>

          <el-form-item label="启用">
            <el-switch v-model="form.enabled" />
          </el-form-item>
          <el-form-item label="备注">
            <el-input v-model="form.remark" type="textarea" :rows="2" />
          </el-form-item>
        </template>
      </el-form>
      <template #footer>
        <el-button @click="visible = false">取消</el-button>
        <el-button type="primary" :disabled="!form.carrierCode" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.hdr { display: flex; align-items: center; justify-content: space-between; }
.toolbar { display: flex; gap: 8px; margin-bottom: 16px; }
.pager { margin-top: 16px; display: flex; justify-content: flex-end; }
.hint { margin-top: 4px; font-size: 12px; color: var(--el-text-color-secondary); line-height: 1.4; }
</style>
