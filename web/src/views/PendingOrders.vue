<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import {
  shippingApi,
  type CarrierAccount,
  type OrderSnapshot,
  type PendingOrder,
  type ShipperProfile,
  type TradeGoods,
} from '../api/shipping'
import { dateShortcuts, defaultDateRange } from '../utils/date'

const loading = reactive({ orders: false, decrypt: false, ship: false, decryptRow: {} as Record<string, boolean> })
const orders = ref<PendingOrder[]>([])
const orderTotal = ref(0)
const orderHint = ref('')

const carrierAccounts = ref<CarrierAccount[]>([])
const shipperProfiles = ref<ShipperProfile[]>([])

const shipDialogVisible = ref(false)
const shipTarget = ref<PendingOrder | null>(null)
const shipForm = reactive({
  carrierAccountId: undefined as number | undefined,
  shipperProfileId: undefined as number | undefined,
  useMonthly: false,
})
const shipResult = ref<{ mailNo: string; labelUrl?: string; shipmentId?: number } | null>(null)

const [defaultStart, defaultEnd] = defaultDateRange()

const filters = reactive({
  platform: 'FXG',
  shopId: '',
  tradeStatus: 'wait_send',
  timeType: 0,
  dateRange: [defaultStart, defaultEnd] as [string, string],
  pageNo: 1,
  pageSize: 20,
})

const platformOptions = [
  { label: '抖店', value: 'FXG' },
  { label: '淘宝', value: 'TB' },
  { label: '小红书', value: 'XHS' },
]

const timeTypeOptions = [
  { label: '下单时间', value: 0 },
  { label: '发货时间', value: 1 },
]

function orderSysTid(order: PendingOrder) {
  return order.sysTids?.[0] || ''
}

function formatGoodsLines(goods?: TradeGoods[]): string {
  return (goods || [])
    .map((g) => {
      const spec = g.skuName?.trim() || g.title?.trim() || g.outerId?.trim() || ''
      if (!spec) return ''
      const num = g.num && g.num > 0 ? g.num : 1
      return `${spec} x${num}`
    })
    .filter(Boolean)
    .join('；')
}

async function loadOrders() {
  loading.orders = true
  try {
    const [startDateTime, endDateTime] = filters.dateRange
    const data = await shippingApi.listPendingOrders({
      platform: filters.platform || undefined,
      shopId: filters.shopId || undefined,
      tradeStatus: filters.tradeStatus,
      timeType: filters.timeType,
      startDateTime,
      endDateTime,
      pageNo: filters.pageNo,
      pageSize: filters.pageSize,
    })
    orders.value = data.items || []
    orderTotal.value = data.total || 0
    orderHint.value = data.hint || ''
  } catch (e) {
    ElMessage.error((e as Error).message || '加载订单失败')
  } finally {
    loading.orders = false
  }
}

async function loadOptions() {
  try {
    const [carriers, shippers] = await Promise.all([
      shippingApi.listCarrierAccounts({ page: 1, pageSize: 200 }),
      shippingApi.listShipperProfiles({ page: 1, pageSize: 200 }),
    ])
    carrierAccounts.value = (carriers.list || []).filter((c) => c.enabled)
    shipperProfiles.value = (shippers.list || []).filter((s) => s.enabled)
  } catch {
    /* optional preload */
  }
}

function onFilterChange() {
  filters.pageNo = 1
  loadOrders()
}

function onPageChange(page: number) {
  filters.pageNo = page
  loadOrders()
}

function applyDecryptedItems(items: PendingOrder[]) {
  const bySysTid = new Map(items.map((item) => [orderSysTid(item), item]))
  orders.value = orders.value.map((order) => {
    const sysTid = orderSysTid(order)
    const decrypted = sysTid ? bySysTid.get(sysTid) : undefined
    if (!decrypted) return order
    return {
      ...order,
      receiverName: decrypted.receiverName,
      receiverMobile: decrypted.receiverMobile,
      receiverAddress: decrypted.receiverAddress,
      formattedReceiver: decrypted.formattedReceiver,
      decrypted: true,
    }
  })
}

async function decryptOne(order: PendingOrder) {
  const sysTid = orderSysTid(order)
  if (!sysTid) {
    ElMessage.warning('缺少系统订单号')
    return
  }
  loading.decryptRow[sysTid] = true
  try {
    const data = await shippingApi.decryptPendingOrders({
      platform: order.platform || filters.platform,
      tradeStatus: filters.tradeStatus,
      sysTids: [sysTid],
    })
    applyDecryptedItems(data.items || [])
    ElMessage.success('解密成功')
  } catch (e) {
    ElMessage.error((e as Error).message || '解密失败')
  } finally {
    loading.decryptRow[sysTid] = false
  }
}

function orderToSnapshot(order: PendingOrder): OrderSnapshot {
  const sysTid = orderSysTid(order)
  const sourceTid = order.tids?.[0] || ''
  return {
    platform: order.platform || filters.platform,
    shopId: order.shopId || '',
    sysTid,
    sourceTid,
    receiverName: order.receiverName || '',
    receiverMobile: order.receiverMobile || '',
    receiverProvince: '',
    receiverCity: '',
    receiverCounty: '',
    receiverAddress: order.receiverAddress || order.formattedReceiver || '',
    goods: (order.goods || []).map((g) => ({
      title: g.title || '',
      skuName: g.skuName || '',
      num: g.num && g.num > 0 ? g.num : 1,
      outerId: g.outerId || '',
      price: g.price || 0,
    })),
  }
}

function openShipDialog(order: PendingOrder) {
  if (!order.decrypted && !order.formattedReceiver) {
    ElMessage.warning('请先解密收件信息')
    return
  }
  shipTarget.value = order
  shipResult.value = null
  const defaultCarrier = carrierAccounts.value[0]
  const defaultShipper = shipperProfiles.value.find((s) => s.isDefault) || shipperProfiles.value[0]
  shipForm.carrierAccountId = defaultCarrier?.id
  shipForm.shipperProfileId = defaultShipper?.id
  shipForm.useMonthly = defaultCarrier?.useMonthly ?? false
  shipDialogVisible.value = true
}

function onCarrierChange(id: number | undefined) {
  const carrier = carrierAccounts.value.find((c) => c.id === id)
  if (carrier) shipForm.useMonthly = carrier.useMonthly
}

async function submitShip() {
  if (!shipTarget.value) return
  if (!shipForm.carrierAccountId || !shipForm.shipperProfileId) {
    ElMessage.warning('请选择物流账号和寄件人')
    return
  }
  loading.ship = true
  try {
    const shipment = await shippingApi.createShipmentFromOrder({
      carrierAccountId: shipForm.carrierAccountId,
      shipperProfileId: shipForm.shipperProfileId,
      useMonthly: shipForm.useMonthly,
      order: orderToSnapshot(shipTarget.value),
    })
    const waybill = await shippingApi.createShipmentWaybill(shipment.id)
    shipResult.value = {
      mailNo: waybill.mailNo,
      labelUrl: waybill.labelUrl,
      shipmentId: waybill.id,
    }
    ElMessage.success(`打单成功，运单号：${waybill.mailNo}`)
  } catch (e) {
    ElMessage.error((e as Error).message || '打单失败')
  } finally {
    loading.ship = false
  }
}

async function printResult() {
  if (!shipResult.value?.shipmentId) return
  try {
    const updated = await shippingApi.printShipment(shipResult.value.shipmentId)
    if (updated.labelUrl) {
      window.open(updated.labelUrl, '_blank')
    } else {
      ElMessage.info('暂无面单链接')
    }
  } catch (e) {
    ElMessage.error((e as Error).message || '打印失败')
  }
}

function closeShipDialog() {
  shipDialogVisible.value = false
  shipTarget.value = null
  shipResult.value = null
}

onMounted(async () => {
  await loadOptions()
  await loadOrders()
})
</script>

<template>
  <div class="page">
    <el-alert v-if="orderHint" type="warning" :title="orderHint" show-icon :closable="false" class="hint" />

    <el-card v-loading="loading.orders">
      <template #header>
        <div class="hdr">
          <span>待发货 <span class="count">({{ orderTotal }})</span></span>
        </div>
      </template>

      <div class="filter-panel">
        <div class="filter-row">
          <span class="filter-label">时间类型</span>
          <el-radio-group v-model="filters.timeType" @change="onFilterChange">
            <el-radio-button v-for="opt in timeTypeOptions" :key="opt.value" :label="opt.value">
              {{ opt.label }}
            </el-radio-button>
          </el-radio-group>
        </div>
        <div class="filter-row">
          <span class="filter-label">时间范围</span>
          <el-date-picker
            v-model="filters.dateRange"
            type="datetimerange"
            range-separator="至"
            start-placeholder="开始时间"
            end-placeholder="结束时间"
            value-format="YYYY-MM-DD HH:mm:ss"
            :shortcuts="dateShortcuts"
            style="width: 420px"
            @change="onFilterChange"
          />
        </div>
        <div class="filter-row">
          <span class="filter-label">筛选</span>
          <div class="filters">
            <el-select v-model="filters.platform" placeholder="平台" clearable style="width: 120px" @change="onFilterChange">
              <el-option v-for="opt in platformOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
            </el-select>
            <el-input v-model="filters.shopId" clearable placeholder="店铺 ID" style="width: 160px" @change="onFilterChange" />
            <el-button type="primary" :icon="Search" :loading="loading.orders" @click="loadOrders">查询</el-button>
          </div>
        </div>
      </div>

      <el-table :data="orders" border stripe empty-text="暂无待发货订单">
        <el-table-column prop="platformName" label="平台" width="90" />
        <el-table-column label="订单号" min-width="200">
          <template #default="{ row }">
            <div v-if="row.tids?.length">平台：{{ row.tids.join(', ') }}</div>
            <div v-if="row.sysTids?.length" class="muted">系统：{{ row.sysTids.join(', ') }}</div>
          </template>
        </el-table-column>
        <el-table-column prop="shopName" label="店铺" min-width="140" show-overflow-tooltip />
        <el-table-column label="商品" min-width="240" show-overflow-tooltip>
          <template #default="{ row }">{{ formatGoodsLines(row.goods) || '-' }}</template>
        </el-table-column>
        <el-table-column label="收件信息" min-width="260">
          <template #default="{ row }">
            <template v-if="row.decrypted && row.formattedReceiver">
              <div class="decrypted-line">{{ row.formattedReceiver }}</div>
            </template>
            <template v-else>
              <div>{{ row.receiverName || '-' }}</div>
              <div class="muted">{{ row.receiverMobile }}</div>
              <div class="muted">{{ row.receiverAddress }}</div>
            </template>
          </template>
        </el-table-column>
        <el-table-column prop="payTime" label="付款时间" width="170" />
        <el-table-column prop="statusText" label="状态" width="100">
          <template #default="{ row }">{{ row.statusText || row.tradeStatus || '-' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="!row.decrypted"
              link
              type="primary"
              size="small"
              :loading="loading.decryptRow[orderSysTid(row)]"
              @click="decryptOne(row)"
            >
              解密
            </el-button>
            <el-button link type="primary" size="small" @click="openShipDialog(row)">打单</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pager">
        <el-pagination
          v-model:current-page="filters.pageNo"
          :page-size="filters.pageSize"
          :total="orderTotal"
          layout="total, prev, pager, next"
          @current-change="onPageChange"
        />
      </div>
    </el-card>

    <el-dialog v-model="shipDialogVisible" title="打单" width="520px" @close="closeShipDialog">
      <template v-if="shipTarget">
        <div class="ship-order-info muted">
          订单：{{ orderSysTid(shipTarget) }}
          <span v-if="shipTarget.tids?.length"> / {{ shipTarget.tids.join(', ') }}</span>
        </div>
        <el-form label-width="100px" class="ship-form">
          <el-form-item label="物流账号" required>
            <el-select
              v-model="shipForm.carrierAccountId"
              placeholder="选择顺丰账号"
              style="width: 100%"
              @change="onCarrierChange"
            >
              <el-option v-for="c in carrierAccounts" :key="c.id" :label="c.name" :value="c.id!" />
            </el-select>
          </el-form-item>
          <el-form-item label="寄件人" required>
            <el-select v-model="shipForm.shipperProfileId" placeholder="选择寄件人" style="width: 100%">
              <el-option
                v-for="s in shipperProfiles"
                :key="s.id"
                :label="s.isDefault ? `${s.name}（默认）` : s.name"
                :value="s.id!"
              />
            </el-select>
          </el-form-item>
          <el-form-item label="月结">
            <el-switch v-model="shipForm.useMonthly" />
          </el-form-item>
        </el-form>

        <el-result v-if="shipResult" icon="success" title="打单成功">
          <template #sub-title>
            运单号：<strong>{{ shipResult.mailNo }}</strong>
          </template>
          <template #extra>
            <el-button type="primary" @click="printResult">打开面单</el-button>
          </template>
        </el-result>
      </template>

      <template #footer>
        <el-button @click="closeShipDialog">{{ shipResult ? '关闭' : '取消' }}</el-button>
        <el-button v-if="!shipResult" type="primary" :loading="loading.ship" @click="submitShip">确认打单</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 12px; }
.hint { margin-bottom: 0; }
.hdr { display: flex; align-items: center; justify-content: space-between; }
.count { color: #909399; font-size: 14px; font-weight: normal; }
.filter-panel { display: flex; flex-direction: column; gap: 12px; margin-bottom: 16px; }
.filter-row { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.filter-label { width: 72px; color: #606266; flex-shrink: 0; }
.filters { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
.muted { color: #909399; font-size: 13px; }
.decrypted-line { white-space: pre-wrap; line-height: 1.5; }
.pager { margin-top: 16px; display: flex; justify-content: flex-end; }
.ship-order-info { margin-bottom: 16px; }
.ship-form { margin-top: 8px; }
</style>
