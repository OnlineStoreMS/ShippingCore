import type { OMSOrder, OrderGoods, OrderSnapshot } from '../api/shipping'

export const SF_ORDER_HANDOFF_KEY = 'shippingcore.sfOrder.handoff'

export interface SFOrderHandoff {
  orderId?: number
  sourceSystem?: 'ordercore' | 'storesyncagent'
  order: OrderSnapshot
}

export function omsOrderToSnapshot(order: OMSOrder): OrderSnapshot {
  const addr = order.address
  return {
    platform: order.platform || '',
    shopId: order.shopId || '',
    sysTid: order.platformSysTid || '',
    sourceTid: order.platformOrderId || order.orderNo,
    receiverName: addr?.name || order.buyerName || '',
    receiverMobile: addr?.phone || order.buyerPhone || '',
    receiverProvince: addr?.province || '',
    receiverCity: addr?.city || '',
    receiverCounty: addr?.district || '',
    receiverAddress: addr?.fullText || addr?.address || '',
    goods: (order.items || []).map((g) => {
      const spec = (g.skuSpecs || '').trim()
      const product = (g.productName || '').trim()
      return {
        title: product,
        // 发货/下顺丰单用规格名称；无规格时才落商品名到 skuName 兜底
        skuName: spec || product,
        num: g.quantity && g.quantity > 0 ? g.quantity : 1,
        outerId: '',
        price: 0,
      }
    }),
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
