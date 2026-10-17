import type { OMSOrder, OMSOrderAddress, OMSOrderItem } from '../api/shipping'

function pad(n: number) {
  return String(n).padStart(2, '0')
}

/** 与订单中心一致：2026 07/11 12:42 */
function formatOrderCopyDateTime(d: Date): string {
  return `${d.getFullYear()} ${pad(d.getMonth() + 1)}/${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function formatOMSAddress(addr?: OMSOrderAddress | null) {
  if (!addr) return '-'
  if (addr.fullText) return addr.fullText
  const parts = [addr.name, addr.phone, addr.province, addr.city, addr.district, addr.address].filter(Boolean)
  return parts.join(' ') || '-'
}

export function isMaskedReceiver(order: Pick<OMSOrder, 'buyerName' | 'buyerPhone' | 'address'>) {
  const text = [order.buyerName, order.buyerPhone, formatOMSAddress(order.address)].join(' ')
  return /[*＊]/.test(text)
}

export function canDecryptOMS(order: Pick<OMSOrder, 'sourceChannel' | 'platformSysTid'>) {
  return order.sourceChannel === 'kdzs' && !!order.platformSysTid
}

export function canDecryptRealPhone(order: Pick<OMSOrder, 'platform' | 'shopId' | 'platformOrderId'>) {
  const p = (order.platform || '').trim().toUpperCase()
  return (p === 'FXG' || p === 'DOUDIAN') && !!order.shopId?.trim() && !!order.platformOrderId?.trim()
}

function goodsLines(items?: OMSOrderItem[]) {
  return (items || [])
    .filter((it) => !it.splitKind && !(it.parentOrderItemId && it.parentOrderItemId > 0))
    .map((it) => {
      const spec = (it.skuSpecs || it.productName || '').trim()
      if (!spec) return ''
      const num = it.quantity && it.quantity > 0 ? it.quantity : 1
      return `${spec} x${num}`
    })
    .filter(Boolean)
    .join('\n')
}

/** 时间 + 空行 + 收件信息 + --- + 规格行 */
export function buildOMSCopyText(order: OMSOrder, now = new Date()) {
  const address = formatOMSAddress(order.address)
  const goodsBlock = goodsLines(order.items)
  const lines = [formatOrderCopyDateTime(now), '', address === '-' ? '' : address, '---']
  if (goodsBlock) lines.push(goodsBlock)
  return lines.join('\n')
}

export async function copyToClipboard(text: string): Promise<boolean> {
  if (!text) return false
  if (navigator.clipboard?.writeText && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      /* fallback */
    }
  }
  try {
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.setAttribute('readonly', 'true')
    textarea.style.position = 'fixed'
    textarea.style.left = '-9999px'
    document.body.appendChild(textarea)
    textarea.focus()
    textarea.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(textarea)
    return ok
  } catch {
    return false
  }
}
