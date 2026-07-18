<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus, Delete } from '@element-plus/icons-vue'
import { api } from '../../api/wms'

const loading = ref(false)
const saving = ref(false)
const storeFee = ref(0)
const fixedStoreFee = ref(0)
const packFee = ref(0)
const scoreRules = ref<any[]>([])
const qtyCoeffs = ref<any[]>([])

async function load() {
  loading.value = true
  try {
    const data = await api.getGoodsFeeSettings()
    storeFee.value = data.storeFee || 0
    fixedStoreFee.value = data.fixedStoreFee || 0
    packFee.value = data.packFee || 0
    scoreRules.value = (data.scoreRules || []).map((r: any) => ({ ...r }))
    qtyCoeffs.value = (data.qtyCoeffs || []).map((r: any) => ({ ...r }))
  } catch (e) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(load)

function addScore() {
  scoreRules.value.push({ weightMinG: 0, weightMaxG: 0, scoreFactor: 1 })
}
function removeScore(i: number) {
  scoreRules.value.splice(i, 1)
}
function addQty() {
  qtyCoeffs.value.push({ qtyMin: 0, qtyMax: 0, coeff: 1 })
}
function removeQty(i: number) {
  qtyCoeffs.value.splice(i, 1)
}

async function save() {
  saving.value = true
  try {
    await api.saveGoodsFeeSettings({
      storeFee: Number(storeFee.value) || 0,
      fixedStoreFee: Number(fixedStoreFee.value) || 0,
      packFee: Number(packFee.value) || 0,
      scoreRules: scoreRules.value,
      qtyCoeffs: qtyCoeffs.value,
    })
    ElMessage.success('已保存')
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="page" v-loading="loading">
    <el-row :gutter="20">
      <el-col :span="12">
        <el-card>
          <template #header>
            <div class="hdr">
              <span>单个商品分值系数</span>
              <el-button type="primary" link :icon="Plus" @click="addScore">添加</el-button>
            </div>
          </template>
          <el-table :data="scoreRules" border size="small" empty-text="按重量区间设置分值系数">
            <el-table-column label="重量下限(g)" width="130">
              <template #default="{ row }">
                <el-input-number v-model="row.weightMinG" :min="0" :controls="false" size="small" style="width: 100%" />
              </template>
            </el-table-column>
            <el-table-column label="重量上限(g)" width="130">
              <template #default="{ row }">
                <el-input-number v-model="row.weightMaxG" :min="0" :controls="false" size="small" style="width: 100%" placeholder="0=不限" />
              </template>
            </el-table-column>
            <el-table-column label="分值系数" min-width="110">
              <template #default="{ row }">
                <el-input-number v-model="row.scoreFactor" :min="0" :precision="4" :controls="false" size="small" style="width: 100%" />
              </template>
            </el-table-column>
            <el-table-column label="" width="60">
              <template #default="{ $index }">
                <el-button link type="danger" :icon="Delete" @click="removeScore($index)" />
              </template>
            </el-table-column>
          </el-table>
          <div class="hint">商品信息中的分值系数优先；未填时按重量区间匹配。</div>
        </el-card>

        <el-card class="mt">
          <template #header>
            <div class="hdr">
              <span>订单商品数量系数</span>
              <el-button type="primary" link :icon="Plus" @click="addQty">添加</el-button>
            </div>
          </template>
          <el-table :data="qtyCoeffs" border size="small" empty-text="按订单商品总数量区间设置系数">
            <el-table-column label="数量下限" width="120">
              <template #default="{ row }">
                <el-input-number v-model="row.qtyMin" :min="0" :controls="false" size="small" style="width: 100%" />
              </template>
            </el-table-column>
            <el-table-column label="数量上限" width="120">
              <template #default="{ row }">
                <el-input-number v-model="row.qtyMax" :min="0" :controls="false" size="small" style="width: 100%" placeholder="0=不限" />
              </template>
            </el-table-column>
            <el-table-column label="数量系数" min-width="110">
              <template #default="{ row }">
                <el-input-number v-model="row.coeff" :min="0" :precision="4" :controls="false" size="small" style="width: 100%" />
              </template>
            </el-table-column>
            <el-table-column label="" width="60">
              <template #default="{ $index }">
                <el-button link type="danger" :icon="Delete" @click="removeQty($index)" />
              </template>
            </el-table-column>
          </el-table>
        </el-card>

        <el-card class="mt">
          <template #header><span>基础费用</span></template>
          <el-form label-width="140px">
            <el-form-item label="单位系数仓库费用">
              <el-input-number v-model="storeFee" :min="0" :precision="4" style="width: 180px" />
              <span class="unit">CNY</span>
            </el-form-item>
            <el-form-item label="仓库固定费用">
              <el-input-number v-model="fixedStoreFee" :min="0" :precision="4" style="width: 180px" />
              <span class="unit">CNY</span>
            </el-form-item>
            <el-form-item label="单位系数打包费用">
              <el-input-number v-model="packFee" :min="0" :precision="4" style="width: 180px" />
              <span class="unit">CNY</span>
            </el-form-item>
          </el-form>
          <el-button type="primary" :loading="saving" @click="save">保存</el-button>
        </el-card>
      </el-col>

      <el-col :span="12">
        <el-card>
          <template #header><span>计算说明</span></template>
          <ol class="notes">
            <li>可计算每一笔订单的打包费用和仓库费用。</li>
            <li>商品分值系数优先取商品信息中的分值系数；未填写则按重量区间规则。</li>
            <li>如需计算订单仓库费用，请在「仓库货位 → 仓库设置」开启「允许计算仓库费用」。</li>
          </ol>
          <div class="section">计算公式</div>
          <p>订单打包费用 = Σ(SKU分值系数 × 数量) × 订单商品总数量系数 × 单位系数打包费用</p>
          <p>订单仓库费用 = Σ(SKU分值系数 × 数量) × 订单商品总数量系数 × 单位系数仓库费用 + 仓库固定费用</p>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.mt { margin-top: 16px; }
.hint { margin-top: 8px; font-size: 12px; color: #909399; }
.unit { margin-left: 8px; color: #909399; }
.notes { margin: 0; padding-left: 18px; line-height: 1.8; color: #606266; }
.section { margin: 16px 0 8px; font-weight: 600; }
p { margin: 0 0 8px; color: #606266; line-height: 1.6; }
</style>
