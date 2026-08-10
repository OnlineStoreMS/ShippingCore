<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { CircleCheck, CircleClose, Link, Printer, Refresh } from '@element-plus/icons-vue'
import {
  ensureLocalPrintService,
  getSavedPrinterIndex,
  getSavedPrinterName,
  listLocalPrinters,
  savePrinterSelection,
  testPrintLocalPrinter,
  type LocalPrinter,
} from '../utils/sfPrintPlugin'

const CLODOP_DOWNLOAD = 'http://www.lodop.net/download.html'
const CLODOP_PORTS = [
  { label: 'HTTP 主端口', url: 'http://localhost:8000/', host: 'localhost:8000' },
  { label: 'HTTP 备用端口', url: 'http://localhost:18000/', host: 'localhost:18000' },
  { label: 'HTTPS', url: 'https://localhost:8443/', host: 'localhost:8443' },
] as const
const CLODOP_SCRIPT_SNIPPET =
  '<script src="http://localhost:8000/CLodopfuncs.js"><' +
  '/script>\n' +
  '<script src="http://localhost:18000/CLodopfuncs.js"><' +
  '/script>'

const loading = ref(false)
const testing = ref(false)
const serviceOk = ref(false)
const serviceError = ref('')
const serviceEndpoint = ref('')
const printers = ref<LocalPrinter[]>([])
const selected = ref<number | null>(getSavedPrinterIndex())
const savedName = ref(getSavedPrinterName())

const selectedPrinter = computed(() => printers.value.find((p) => p.index === selected.value) || null)

async function probeService() {
  serviceError.value = ''
  serviceEndpoint.value = ''
  try {
    await ensureLocalPrintService()
    serviceOk.value = true
    // 探测成功即说明本机 CLodopfuncs.js 已加载
    serviceEndpoint.value = 'localhost（8000 / 18000 / 8443）'
  } catch (e) {
    serviceOk.value = false
    serviceError.value = (e as Error).message || '本机 C-Lodop 服务未连通'
  }
}

async function refresh() {
  loading.value = true
  try {
    await probeService()
    if (!serviceOk.value) {
      printers.value = []
      return
    }
    printers.value = await listLocalPrinters()
    if (selected.value != null && !printers.value.some((p) => p.index === selected.value)) {
      const byName = printers.value.find((p) => p.name === savedName.value)
      selected.value = byName?.index ?? (printers.value[0]?.index ?? null)
    }
    if (selected.value == null && printers.value.length === 1) {
      selected.value = printers.value[0].index
    }
  } catch (e) {
    printers.value = []
    ElMessage.error((e as Error).message || '读取打印机失败')
  } finally {
    loading.value = false
  }
}

function choose(p: LocalPrinter) {
  selected.value = p.index
  savePrinterSelection(p.index, p.name)
  savedName.value = p.name
  ElMessage.success(`已选择：${p.name}`)
}

function saveCurrent() {
  if (selected.value == null || !selectedPrinter.value) {
    ElMessage.warning('请先选择一台打印机')
    return
  }
  savePrinterSelection(selected.value, selectedPrinter.value.name)
  savedName.value = selectedPrinter.value.name
  ElMessage.success('已保存为默认打印机')
}

async function runTest() {
  if (selected.value == null) {
    ElMessage.warning('请先选择一台打印机')
    return
  }
  testing.value = true
  try {
    const name = selectedPrinter.value?.name
    savePrinterSelection(selected.value, name)
    await testPrintLocalPrinter({ printerIndex: selected.value, printerName: name })
    ElMessage.success('测试页已发送，请查看打印机是否出纸')
  } catch (e) {
    ElMessage.error((e as Error).message || '测试打印失败')
  } finally {
    testing.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <div class="page" v-loading="loading">
    <div class="page-hd">
      <div>
        <h1>C-Lodop 云打印服务</h1>
        <p class="sub">
          本机 Web 打印中间件。业务页通过浏览器访问本机服务完成打印，与快递品牌无关。
        </p>
      </div>
      <el-button :icon="Refresh" @click="refresh">刷新状态</el-button>
    </div>

    <el-row :gutter="16">
      <el-col :xs="24" :md="14">
        <el-card shadow="never" class="card" :class="serviceOk ? 'card-ok' : 'card-bad'">
          <div class="status-row">
            <el-icon :size="32" :color="serviceOk ? '#52c41a' : '#ff4d4f'">
              <CircleCheck v-if="serviceOk" />
              <CircleClose v-else />
            </el-icon>
            <div>
              <div class="status-title">
                {{ serviceOk ? '本机服务已连通' : '本机服务未连通' }}
              </div>
              <div class="status-desc">
                <template v-if="serviceOk">
                  已加载 C-Lodop（{{ serviceEndpoint }}）。发货、寄件等页面将通过该服务输出到本机打印机。
                </template>
                <template v-else>
                  {{ serviceError || '请在本机安装并启动 C-Lodop 后刷新。' }}
                </template>
              </div>
            </div>
          </div>

          <div class="port-list">
            <div v-for="p in CLODOP_PORTS" :key="p.host" class="port-item">
              <span class="port-label">{{ p.label }}</span>
              <code>{{ p.host }}</code>
              <el-link type="primary" :href="p.url" target="_blank" :underline="false">
                <el-icon><Link /></el-icon>打开
              </el-link>
            </div>
          </div>

          <div class="status-actions">
            <el-link type="primary" href="http://localhost:8000/" target="_blank" :icon="Link">
              打开本机欢迎页
            </el-link>
            <el-link type="primary" :href="CLODOP_DOWNLOAD" target="_blank" :icon="Link">
              官网下载 C-Lodop
            </el-link>
          </div>
        </el-card>

        <el-card shadow="never" class="card">
          <template #header>
            <div class="card-hd">
              <span>默认打印机</span>
              <span v-if="savedName" class="muted">当前：{{ savedName }}</span>
            </div>
          </template>

          <div class="printer-list">
            <button
              v-for="p in printers"
              :key="p.index"
              type="button"
              class="printer-item"
              :class="{ active: selected === p.index }"
              @click="choose(p)"
            >
              <el-icon class="printer-ico"><Printer /></el-icon>
              <span class="meta">
                <span class="name">{{ p.name }}</span>
                <span class="idx">索引 {{ p.index }}</span>
              </span>
              <el-tag v-if="selected === p.index" size="small" type="success" effect="plain">已选</el-tag>
            </button>

            <div v-if="!loading && serviceOk && !printers.length" class="empty">
              未检测到打印机，请确认本机已安装驱动后刷新
            </div>
            <div v-if="!loading && !serviceOk" class="empty">服务未连通，无法列出打印机</div>
          </div>

          <div class="ft">
            <el-button type="primary" :disabled="selected == null" @click="saveCurrent">
              设为默认打印机
            </el-button>
            <el-button :loading="testing" :disabled="selected == null" @click="runTest">
              测试打印
            </el-button>
          </div>
        </el-card>
      </el-col>

      <el-col :xs="24" :md="10">
        <el-card shadow="never" class="card">
          <template #header>服务说明</template>
          <ol class="guide">
            <li>
              在运行浏览器的电脑安装
              <a :href="CLODOP_DOWNLOAD" target="_blank" rel="noreferrer">C-Lodop 云打印服务</a>
              （推荐 Win32 版，兼容面更广）。
            </li>
            <li>安装后保持托盘图标运行；默认监听 <code>8000</code> / <code>18000</code>（HTTPS <code>8443</code>）。</li>
            <li>本页检测的是浏览器本机服务，与业务服务器 IP 无关。</li>
            <li>选择默认打印机后，发货中心各打印入口将复用该设备（保存在本浏览器）。</li>
          </ol>
        </el-card>

        <el-card shadow="never" class="card">
          <template #header>页面引用（供对接参考）</template>
          <pre class="code">{{ CLODOP_SCRIPT_SNIPPET }}</pre>
          <p class="muted tip">
            双端口引用可在其中一个端口占用失败时自动兜底。页面加载后通过全局
            <code>getCLodop()</code> / <code>LODOP</code> 发起打印。
          </p>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<style scoped>
.page {
  max-width: 1100px;
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
  margin-bottom: 16px;
  border-radius: 8px;
}
.card-ok {
  border-color: #b7eb8f;
  background: #f6ffed;
}
.card-bad {
  border-color: #ffccc7;
  background: #fff2f0;
}
.card-hd {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
}
.status-row {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.status-title {
  font-size: 16px;
  font-weight: 600;
}
.status-desc {
  margin-top: 4px;
  font-size: 13px;
  color: #595959;
  line-height: 1.55;
}
.port-list {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.port-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  background: #fff;
  border: 1px solid #e4e7ec;
  border-radius: 6px;
  font-size: 13px;
}
.port-label {
  width: 96px;
  color: #667085;
  flex-shrink: 0;
}
.port-item code {
  flex: 1;
  font-size: 12px;
  color: #1f2a37;
}
.status-actions {
  margin-top: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
}
.printer-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-height: 100px;
}
.printer-item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  text-align: left;
  border: 1px solid #e4e7ec;
  background: #fcfcfd;
  border-radius: 8px;
  padding: 10px 12px;
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
}
.printer-item:hover {
  border-color: #91caff;
  background: #f0f7ff;
}
.printer-item.active {
  border-color: #1677ff;
  background: #e6f4ff;
}
.printer-ico {
  color: #667085;
}
.meta {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.name {
  font-size: 14px;
  font-weight: 600;
  word-break: break-all;
}
.idx {
  font-size: 12px;
  color: #98a2b3;
}
.empty {
  padding: 28px 8px;
  text-align: center;
  color: #98a2b3;
  font-size: 13px;
}
.ft {
  margin-top: 14px;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.guide {
  margin: 0;
  padding-left: 18px;
  color: #344054;
  font-size: 13px;
  line-height: 1.7;
}
.guide a {
  color: #1677ff;
}
.guide code {
  padding: 0 4px;
  background: #f2f4f7;
  border-radius: 3px;
  font-size: 12px;
}
.code {
  margin: 0;
  padding: 12px;
  background: #0f172a;
  color: #e2e8f0;
  border-radius: 6px;
  font-size: 12px;
  line-height: 1.55;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
.muted {
  color: #98a2b3;
  font-size: 12px;
}
.tip {
  margin: 10px 0 0;
  line-height: 1.55;
}
.tip code {
  padding: 0 4px;
  background: #f2f4f7;
  border-radius: 3px;
}
</style>
