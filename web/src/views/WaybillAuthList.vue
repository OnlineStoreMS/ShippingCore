<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { shippingApi, type WaybillAuth } from '../api/shipping'

/** 对齐快递助手 Ct 表 kddType → platformName */
const ELEC_TYPE_NAME: Record<number, string> = {
  3: '菜鸟',
  5: '京东',
  7: '拼多多',
  8: '抖店',
  9: '快手小店',
  14: '视频号',
  16: '小红书(新)',
}

interface AuthCard {
  id: number
  platformName: string
  accountName: string
  accountId: string
  authMobile: string
  serviceExpireTime: string
  expireTime: string
  nonShop: boolean
  expired: boolean
  sourceLabel: string
  syncedAt: string
}

function sourceLabelOf(row: WaybillAuth): string {
  return row.kdzsAccountName || row.kdzsAccountCode || (row.source === 'kdzs' ? '快递助手' : row.source || '-')
}

const loading = ref(false)
const syncing = ref(false)
const items = ref<WaybillAuth[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)

function parseRaw(raw?: string): Record<string, unknown> {
  if (!raw) return {}
  try {
    return JSON.parse(raw) as Record<string, unknown>
  } catch {
    return {}
  }
}

function str(v: unknown): string {
  if (v == null) return ''
  return String(v).trim()
}

function asNum(v: unknown): number | null {
  if (typeof v === 'number' && Number.isFinite(v)) return v
  if (typeof v === 'string' && v.trim() !== '' && !Number.isNaN(Number(v))) return Number(v)
  return null
}

function isExpired(expireTime: string): boolean {
  if (!expireTime) return false
  const t = Date.parse(expireTime.replace(/-/g, '/'))
  return Number.isFinite(t) && t < Date.now()
}

const cards = computed<AuthCard[]>(() =>
  items.value.map((row) => {
    const raw = parseRaw(row.rawJson)
    const elecType = asNum(raw.electronicType)
    const platformName =
      str(row.platform) ||
      str(raw.platformName) ||
      (elecType != null ? ELEC_TYPE_NAME[elecType] : '') ||
      '未知平台'
    const accountName = str(row.accountName) || str(raw.accountName) || '-'
    const expireTime = str(raw.expireTime) || (row.authStatus?.startsWith('expire:') ? row.authStatus.slice(7) : row.authStatus)
    const serviceExpireTime = str(raw.serviceExpireTime)
    const authMethod = asNum(raw.authorizationMethod) ?? 0
    return {
      id: row.id,
      platformName,
      accountName,
      accountId: str(raw.accountId),
      authMobile: str(raw.mobile) || str(raw.authMobile),
      serviceExpireTime,
      expireTime,
      nonShop: authMethod !== 0,
      expired: isExpired(expireTime),
      sourceLabel: sourceLabelOf(row),
      syncedAt: row.syncedAt,
    }
  }),
)

async function load() {
  loading.value = true
  try {
    const data = await shippingApi.listWaybillAuths({ page: page.value, pageSize: pageSize.value })
    items.value = data.list || []
    total.value = data.total || 0
  } catch (e) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function sync() {
  syncing.value = true
  try {
    const stats = await shippingApi.syncKdzsPrintAssets()
    const delTpl = stats.templatesDeleted || 0
    const delAuth = stats.authsDeleted || 0
    const extra = delTpl || delAuth ? `，清理模板 ${delTpl} / 授权 ${delAuth}` : ''
    ElMessage.success(`同步完成：授权 ${stats.auths} 条，模板 ${stats.templates} 条${extra}（已与快递助手对齐）`)
    page.value = 1
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message || '同步失败')
  } finally {
    syncing.value = false
  }
}

function openKdzs() {
  window.open('https://df.kdzs.com/?#/sygj/bind', '_blank')
}

function onPageChange(p: number) {
  page.value = p
  load()
}

onMounted(load)
</script>

<template>
  <div class="page">
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <div class="title-block">
            <span>面单授权</span>
            <span class="hint">全部电子面单授权，展示对齐快递助手「电子面单授权」</span>
          </div>
          <div class="actions">
            <el-button @click="openKdzs">打开快递助手</el-button>
            <el-button type="primary" :loading="syncing" @click="sync">从快递助手同步</el-button>
          </div>
        </div>
      </template>

      <div v-loading="loading" class="auth-grid">
        <div v-if="!loading && cards.length === 0" class="empty">暂无数据，请先切换账号后点击同步</div>
        <div v-for="card in cards" :key="card.id" class="auth-card" :class="{ expired: card.expired }">
          <div class="card-head">
            <div class="title">
              <span class="plat">{{ card.platformName }}</span>
              <span>{{ card.platformName }}电子面单</span>
              <el-tag v-if="card.nonShop" size="small" type="info" effect="plain">非店铺</el-tag>
            </div>
            <el-tag v-if="card.expired" type="danger" size="small">授权失效</el-tag>
            <el-tag v-else type="success" size="small" effect="plain">已授权</el-tag>
          </div>
          <div class="card-body">
            <p>
              <span class="label">来源账号</span>
              <el-tag size="small" type="success" effect="plain">{{ card.sourceLabel }}</el-tag>
            </p>
            <p><span class="label">电子面单账号</span>{{ card.accountName }}</p>
            <p v-if="card.authMobile"><span class="label">授权账号</span>{{ card.authMobile }}</p>
            <p v-if="card.serviceExpireTime"><span class="label">服务到期时间</span>{{ card.serviceExpireTime }}</p>
            <p v-if="card.expireTime"><span class="label">授权到期时间</span>{{ card.expireTime }}</p>
          </div>
        </div>
      </div>

      <div class="pager">
        <el-pagination
          v-model:current-page="page"
          :page-size="pageSize"
          :total="total"
          layout="total, prev, pager, next"
          @current-change="onPageChange"
        />
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 12px; }
.hdr { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.title-block { display: flex; flex-direction: column; gap: 4px; }
.hint { color: #909399; font-size: 12px; font-weight: 400; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; }
.auth-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
  min-height: 120px;
}
.empty {
  grid-column: 1 / -1;
  text-align: center;
  color: #909399;
  padding: 48px 0;
}
.auth-card {
  border: 1px solid #ebeef5;
  border-radius: 8px;
  padding: 16px;
  background: #fff;
  transition: border-color 0.15s;
}
.auth-card:hover { border-color: #c0c4cc; }
.auth-card.expired { background: #fafafa; }
.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}
.title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  color: #303133;
  flex-wrap: wrap;
}
.plat {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 40px;
  height: 24px;
  padding: 0 8px;
  border-radius: 4px;
  background: #ecf5ff;
  color: #409eff;
  font-size: 12px;
  font-weight: 600;
}
.card-body { display: flex; flex-direction: column; gap: 6px; color: #606266; font-size: 13px; }
.card-body p { margin: 0; line-height: 1.5; }
.label {
  display: inline-block;
  width: 96px;
  color: #909399;
}
.pager { margin-top: 16px; display: flex; justify-content: flex-end; }
</style>
