<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Search, Printer } from '@element-plus/icons-vue'
import { api } from '../../api/wms'

const loading = ref(false)
const list = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)
const keyword = ref('')
const selected = ref<any[]>([])
const preview = ref(false)

async function load() {
  loading.value = true
  try {
    const res = await api.listSkus({ page: page.value, pageSize: pageSize.value, keyword: keyword.value })
    list.value = res.list
    total.value = res.total
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

function openPreview() {
  if (!selected.value.length) {
    ElMessage.warning('请先勾选要打印的 SKU')
    return
  }
  preview.value = true
  nextTick(() => drawAll())
}

watch(preview, (v) => {
  if (v) nextTick(() => drawAll())
})

/** Minimal Code128-B style bar pattern from character codes (simplified). */
function drawBarcode(canvas: HTMLCanvasElement, text: string) {
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  const pattern: number[] = []
  // start B
  pattern.push(2, 1, 1, 2, 1, 4)
  for (let i = 0; i < text.length; i++) {
    const c = text.charCodeAt(i) % 10
    const widths = [
      [2, 1, 2, 2, 2, 2],
      [2, 2, 2, 1, 2, 2],
      [2, 2, 2, 2, 2, 1],
      [1, 2, 1, 2, 2, 3],
      [1, 2, 1, 3, 2, 2],
      [1, 3, 1, 2, 2, 2],
      [1, 2, 2, 2, 1, 3],
      [1, 2, 2, 3, 1, 2],
      [1, 3, 2, 2, 1, 2],
      [2, 2, 1, 2, 1, 3],
    ][c]
    pattern.push(...widths)
  }
  // stop
  pattern.push(2, 3, 3, 1, 1, 1, 2)

  const barH = 48
  const unit = 1.5
  const totalW = pattern.reduce((a, b) => a + b, 0) * unit
  canvas.width = Math.max(totalW + 20, 160)
  canvas.height = barH + 8
  ctx.fillStyle = '#fff'
  ctx.fillRect(0, 0, canvas.width, canvas.height)
  ctx.fillStyle = '#000'
  let x = 10
  let black = true
  for (const w of pattern) {
    if (black) ctx.fillRect(x, 4, w * unit, barH)
    x += w * unit
    black = !black
  }
}

function drawAll() {
  const canvases = document.querySelectorAll<HTMLCanvasElement>('.bc-canvas')
  canvases.forEach((c) => {
    const code = c.dataset.code || ''
    drawBarcode(c, code)
  })
}

function doPrint() {
  window.print()
}
</script>

<template>
  <div class="page">
    <el-card v-loading="loading" class="no-print">
      <template #header>
        <div class="hdr">
          <span>条码打印</span>
          <el-button type="primary" :icon="Printer" :disabled="!selected.length" @click="openPreview">打印预览</el-button>
        </div>
      </template>
      <div class="toolbar">
        <el-input v-model="keyword" placeholder="SKU编码 / 拣货名" clearable :prefix-icon="Search" style="width: 260px" @change="search" />
        <el-button type="primary" @click="search">查询</el-button>
      </div>
      <el-table :data="list" border stripe @selection-change="onSelectionChange">
        <el-table-column type="selection" width="48" />
        <el-table-column prop="skuCode" label="SKU编码" width="160" />
        <el-table-column prop="pickName" label="拣货名" min-width="160" />
        <el-table-column prop="style1" label="规格1" width="100" />
        <el-table-column prop="status" label="状态" width="90" />
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

    <el-dialog v-model="preview" title="打印预览" width="720px" class="no-print">
      <div id="print-area" class="print-area">
        <div v-for="sku in selected" :key="sku.id" class="label">
          <canvas class="bc-canvas" :data-code="sku.skuCode" />
          <div class="code">{{ sku.skuCode }}</div>
          <div class="name">{{ sku.pickName || '-' }}</div>
        </div>
      </div>
      <template #footer>
        <el-button @click="preview = false">关闭</el-button>
        <el-button type="primary" :icon="Printer" @click="doPrint">打印</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.toolbar { display: flex; gap: 8px; margin-bottom: 12px; }
.pager { margin-top: 16px; justify-content: flex-end; }
.print-area { display: flex; flex-wrap: wrap; gap: 16px; }
.label {
  width: 200px;
  border: 1px dashed #dcdfe6;
  padding: 12px;
  text-align: center;
  page-break-inside: avoid;
}
.code { font-family: monospace; font-size: 14px; font-weight: 600; margin-top: 4px; }
.name { font-size: 12px; color: #606266; margin-top: 2px; }
@media print {
  .no-print { display: none !important; }
  .print-area { display: flex !important; flex-wrap: wrap; gap: 12px; }
  .label { border: none; }
}
</style>
