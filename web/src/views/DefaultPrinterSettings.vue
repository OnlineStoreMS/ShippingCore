<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { shippingApi } from '../api/shipping'

const loading = ref(false)
const saving = ref(false)
const sfPrinterName = ref('')
const kdzsPrinterName = ref('')

async function load() {
  loading.value = true
  try {
    const st = await shippingApi.getPrintSettings()
    sfPrinterName.value = (st.sfPrinterName || '').trim()
    kdzsPrinterName.value = (st.kdzsPrinterName || '').trim()
  } catch (e) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const st = await shippingApi.updatePrintSettings({
      sfPrinterName: sfPrinterName.value.trim(),
      kdzsPrinterName: kdzsPrinterName.value.trim(),
    })
    sfPrinterName.value = (st.sfPrinterName || '').trim()
    kdzsPrinterName.value = (st.kdzsPrinterName || '').trim()
    ElMessage.success('已保存，手机端将自动带入')
  } catch (e) {
    ElMessage.error((e as Error).message || '保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page" v-loading="loading">
    <div class="page-hd">
      <div>
        <h1>默认打印机</h1>
        <p class="sub">
          仅配置手机端默认。保存后，手机「打印机管理」「快递助手远程打单」打开时自动带入。
        </p>
      </div>
      <el-button type="primary" :loading="saving" @click="save">保存</el-button>
    </div>

    <el-card shadow="never" class="card">
      <el-form label-width="160px" style="max-width: 640px">
        <el-form-item label="手机·自建物流打印机">
          <el-input
            v-model="sfPrinterName"
            clearable
            maxlength="200"
            placeholder="打印电脑上的打印机完整名称"
          />
          <div class="hint">对应手机「打印机管理」；按名称匹配本机列表，勿填索引号。</div>
        </el-form-item>
        <el-form-item label="手机·快递助手远程打印机">
          <el-input
            v-model="kdzsPrinterName"
            clearable
            maxlength="200"
            placeholder="远程电脑打印弹窗中的完整打印机名称"
          />
          <div class="hint">对应手机「快递助手远程打单」；名称须与打单电脑上一字不差。</div>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<style scoped>
.page {
  max-width: 900px;
  margin: 0 auto;
}
.page-hd {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}
.page-hd h1 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #1f2a37;
}
.sub {
  margin: 6px 0 0;
  font-size: 13px;
  color: #667085;
  line-height: 1.5;
}
.card {
  border-radius: 8px;
}
.hint {
  margin-top: 6px;
  font-size: 12px;
  color: #98a2b3;
  line-height: 1.5;
}
</style>
