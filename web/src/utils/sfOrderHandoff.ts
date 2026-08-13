import type { OMSOrder, OrderGoods, OrderSnapshot } from '../api/shipping'

export const SF_ORDER_HANDOFF_KEY = 'shippingcore.sfOrder.handoff'

export interface SFOrderHandoff {
  orderId?: number
  sourceSystem?: 'ordercore' | 'storesyncagent'
  order: OrderSnapshot
}

export interface OMSOrderShipmentItem {
  orderItemId?: number
  qty?: number
}

export interface OMSOrderShipment {
  id?: number
  items?: OMSOrderShipmentItem[]
}

/** 订单行已发数量（有明细运单时按明细；无明细且整单已发完时视为全部已发） */
export function shippedQtyByItem(order: OMSOrder & { shipments?: OMSOrderShipment[] }): Record<number, number> {
  const map: Record<number, number> = {}
  let hasItemRows = false
  for (const sh of order.shipments || []) {
    if (sh.items?.length) {
      hasItemRows = true
      for (const it of sh.items) {
        if (!it.orderItemId || !(it.qty && it.qty > 0)) continue
        map[it.orderItemId] = (map[it.orderItemId] || 0) + it.qty
      }
    }
  }
  if (!hasItemRows && order.shipStatus === 'shipped') {
    for (const it of order.items || []) {
      if (it.id) map[it.id] = it.quantity || 0
    }
  }
  return map
}

export function remainingQtyByItem(order: OMSOrder & { shipments?: OMSOrderShipment[] }): Record<number, number> {
  const shipped = shippedQtyByItem(order)
  const out: Record<number, number> = {}
  for (const it of order.items || []) {
    if (!it.id) continue
    out[it.id] = Math.max(0, (it.quantity || 0) - (shipped[it.id] || 0))
  }
  return out
}

/** itemIndexes：按订单明细下标勾选发货；不传则全部可发商品 */
export function omsOrderToSnapshot(
  order: OMSOrder & { shipments?: OMSOrderShipment[] },
  opts?: { itemIndexes?: number[]; qtyByItemId?: Record<number, number> },
): OrderSnapshot {
  const addr = order.address
  const manualSource = (order.manualSourceName || '').trim()
  const shopName = (order.shopName || '').trim() || manualSource
  const all = order.items || []
  const remaining = opts?.qtyByItemId || remainingQtyByItem(order)
  const indexes = opts?.itemIndexes
  const picked =
    indexes && indexes.length
      ? indexes
          .filter((i) => i >= 0 && i < all.length)
          .map((i) => all[i])
          .filter(Boolean)
      : all.filter((g) => {
          if (!g.id) return true
          const left = remaining[g.id]
          return left == null || left > 0
        })
  return {
    platform: order.platform || '',
    shopId: order.shopId || '',
    shopName,
    sourceChannel: order.sourceChannel || '',
    manualSourceName: manualSource,
    orderNo: order.orderNo || '',
    sysTid: order.platformSysTid || '',
    sourceTid: order.platformOrderId || order.orderNo,
    receiverName: addr?.name || order.buyerName || '',
    receiverMobile: addr?.phone || order.buyerPhone || '',
    receiverProvince: addr?.province || '',
    receiverCity: addr?.city || '',
    receiverCounty: addr?.district || '',
    receiverAddress: addr?.fullText || addr?.address || '',
    goods: picked
      .map((g) => {
        const spec = (g.skuSpecs || '').trim()
        const product = (g.productName || '').trim()
        const id = g.id || 0
        const left = id ? remaining[id] : undefined
        if (id && left != null && left <= 0) return null
        const num =
          left != null && left > 0
            ? left
            : g.quantity && g.quantity > 0
              ? g.quantity
              : 1
        return {
          orderItemId: id,
          title: product,
          // 发货/下顺丰单用规格名称；无规格时才落商品名到 skuName 兜底
          skuName: spec || product,
          num,
          outerId: '',
          price: 0,
        }
      })
      .filter((g): g is NonNullable<typeof g> => !!g),
  }
}

export function saveSFOrderHandoff(payload: SFOrderHandoff) {
  sessionStorage.setItem(SF_ORDER_HANDOFF_KEY, JSON.stringify(payload))
}

export function consumeSFOrderHandoff(): SFOrderHandoff | null {
  const raw = sessionStorage.getItem(SF_ORDER_HANDOFF_KEY)
  if (!raw) return null
  sessionStorage.removeItem(SF_ORDER_HANDOFF_KEY)
  try {
    return JSON.parse(raw) as SFOrderHandoff
  } catch {
    return null
  }
}

/** 发货内容：优先规格名称（skuName），无规格时才用商品名称 */
export function goodsShipName(g: OrderGoods): string {
  return (g.skuName || g.title || '商品').trim() || '商品'
}

export function goodsCargoName(goods: OrderGoods[]): string {
  const first = goods.find((g) => (g.skuName || g.title || '').trim())
  return first ? goodsShipName(first) : '商品'
}

export function goodsParcelQty(goods: OrderGoods[]): number {
  const sum = goods.reduce((acc, g) => acc + (g.num > 0 ? g.num : 0), 0)
  return sum > 0 ? sum : 1
}

/** 简单粘贴识别：姓名 手机 地址 */
export function parsePastedContact(text: string): {
  name?: string
  mobile?: string
  address?: string
} {
  const raw = text.replace(/\s+/g, ' ').trim()
  if (!raw) return {}
  const mobileMatch = raw.match(/(1[3-9]\d{9})/)
  const mobile = mobileMatch?.[1]
  let rest = raw
  if (mobile) rest = rest.replace(mobile, ' ').replace(/[,，]/g, ' ').replace(/\s+/g, ' ').trim()
  const parts = rest.split(/[,，]/).map((s) => s.trim()).filter(Boolean)
  if (parts.length >= 2) {
    return { name: parts[0], mobile, address: parts.slice(1).join('') }
  }
  const tokens = rest.split(' ').filter(Boolean)
  if (tokens.length >= 2) {
    return { name: tokens[0], mobile, address: tokens.slice(1).join('') }
  }
  return { name: rest || undefined, mobile }
}
