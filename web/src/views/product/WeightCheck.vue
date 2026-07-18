<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { api } from '../../api/wms'

const skuCode = ref('')
const oldWeight = ref<number | null>(null)
const newWeight = ref<number | null>(null)
const pic = ref('')
const pickName = ref('')
const minRate = ref(10)
const maxRate = ref(10)
const autoUpdate = ref(false)
const msg = ref('')
const msgOk = ref(false)
const loading = ref(false)

function speak(text: string) {
  try {
    const u = new SpeechSynthesisUtterance(text)
    u.lang = 'zh-CN'
    window.speechSynthesis.cancel()
    window.speechSynthesis.speak(u)
  } catch { /* ignore */ }
}

function setMsg(text: string, ok = false) {
  msg.value = text
  msgOk.value = ok
  if (text) speak(ok ? '成功' : '失败')
}

async function lookupSku() {
  const code = skuCode.value.trim()
  if (!code) {
    setMsg('请输入库存SKU')
    return
  }
  loading.value = true
  setMsg('')
  try {
    const data = await api.getSkuByCode(code)
    oldWeight.value = Number(data.weightG) || 0
    newWeight.value = null
    pic.value = data.pic || ''
    pickName.value = data.pickName || data.productName || ''
    await nextTick()
    const el = document.getElementById('new-weight-input')?.querySelector('input') as HTMLInputElement | null
    el?.focus()
  } catch (e) {
    oldWeight.value = null
    pic.value = ''
    pickName.value = ''
    setMsg((e as Error).message || 'SKU 不存在')
  } finally {
    loading.value = false
  }
}

function withinTolerance(newW: number, oldW: number) {
  if (!oldW && oldW !== 0) return false
  const lo = oldW * (1 - (Number(minRate.value) || 0) / 100)
  const hi = oldW * (1 + (Number(maxRate.value) || 0) / 100)
  return newW >= lo && newW <= hi
}

async function onWeightEnter() {
  const code = skuCode.value.trim()
  if (!code) {
    setMsg('请先输入SKU')
    return
  }
  if (newWeight.value == null || Number.isNaN(Number(newWeight.value))) {
    setMsg('请输入重量')
    return
  }
  if (oldWeight.value == null) {
    setMsg('原始重量异常，请先查询SKU')
    return
  }
  const nw = Number(newWeight.value)
  const ow = Number(oldWeight.value)
  if ((minRate.value || maxRate.value) && !withinTolerance(nw, ow)) {
    const diff = ow ? (((nw - ow) / ow) * 100).toFixed(1) : '-'
    setMsg(`重量偏差 ${diff}%，超出允许范围（过轻${minRate.value}% / 过重${maxRate.value}%）`)
    return
  }
  if (autoUpdate.value) {
    await updateWeight()
  } else {
    setMsg(`检测通过（${nw}g），可点「更新至原始重量」写入`, true)
  }
}

async function updateWeight() {
  const code = skuCode.value.trim()
  if (!code || newWeight.value == null) {
    setMsg('请先输入SKU与新重量')
    return
  }
  loading.value = true
  try {
    const data = await api.updateSkuWeight({ skuCode: code, weightG: Number(newWeight.value) })
    oldWeight.value = Number(data.weightG) || 0
    setMsg('重量已更新', true)
    newWeight.value = null
    skuCode.value = ''
    await nextTick()
    const el = document.getElementById('sku-input')?.querySelector('input') as HTMLInputElement | null
    el?.focus()
  } catch (e) {
    setMsg((e as Error).message || '更新失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="page" v-loading="loading">
    <el-card>
      <template #header>商品重量检测</template>
      <el-row :gutter="24">
        <el-col :span="10">
          <div class="pic-box">
            <img v-if="pic" :src="pic" alt="" />
            <div v-else class="pic-empty">SKU 图片</div>
          </div>
          <div v-if="pickName" class="name">{{ pickName }}</div>
        </el-col>
        <el-col :span="14">
          <el-form label-width="120px" class="form">
            <el-form-item label="过轻比例(%)">
              <el-input-number v-model="minRate" :min="0" :max="100" />
            </el-form-item>
            <el-form-item label="过重比例(%)">
              <el-input-number v-model="maxRate" :min="0" :max="100" />
            </el-form-item>
            <el-form-item label="自动更新重量">
              <el-switch v-model="autoUpdate" active-text="检测通过后自动写入" />
            </el-form-item>
            <el-form-item label="库存SKU" required>
              <el-input
                id="sku-input"
                v-model="skuCode"
                clearable
                placeholder="扫码或输入后回车"
                style="width: 280px"
                @keyup.enter="lookupSku"
              />
              <el-button type="primary" style="margin-left: 8px" @click="lookupSku">查询</el-button>
            </el-form-item>
            <el-form-item label="原始重量(g)">
              <el-input :model-value="oldWeight == null ? '' : String(oldWeight)" readonly style="width: 200px" />
            </el-form-item>
            <el-form-item label="称重重量(g)" required>
              <el-input-number
                id="new-weight-input"
                v-model="newWeight"
                :min="0"
                :precision="3"
                :controls="false"
                style="width: 200px"
                @keyup.enter="onWeightEnter"
              />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="onWeightEnter">检测</el-button>
              <el-button type="success" @click="updateWeight">更新至原始重量</el-button>
            </el-form-item>
            <div class="msg" :class="{ ok: msgOk }">{{ msg }}</div>
          </el-form>
        </el-col>
      </el-row>
    </el-card>
  </div>
</template>

<style scoped>
.pic-box {
  width: 100%;
  max-width: 280px;
  aspect-ratio: 1;
  border: 1px dashed #dcdfe6;
  border-radius: 8px;
  overflow: hidden;
  background: #fafafa;
  display: flex;
  align-items: center;
  justify-content: center;
}
.pic-box img { width: 100%; height: 100%; object-fit: cover; }
.pic-empty { color: #c0c4cc; }
.name { margin-top: 8px; color: #606266; }
.form { max-width: 520px; }
.msg { min-height: 24px; color: #f56c6c; font-weight: 600; }
.msg.ok { color: #67c23a; }
</style>
