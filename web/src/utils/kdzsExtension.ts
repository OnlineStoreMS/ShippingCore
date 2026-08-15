/** 与 Chrome 扩展 `kdzs-print-helper` 通信 */

export interface KdzsHandoffGoods {
  title?: string
  skuName?: string
  outerId?: string
  num?: number
}

export interface KdzsHandoffOrder {
  orderNo?: string
  platformSysTid?: string
  platformOrderId?: string
  sysTid?: string
  tid?: string
  goods?: KdzsHandoffGoods[]
}

export interface KdzsHandoffPayload {
  v: 1
  createdAt: number
  platform: string
  templateName: string
  templateId?: string
  orders: KdzsHandoffOrder[]
  /** 固定 false：打印由人工 */
  autoPrint: false
}

export function isKdzsHelperInstalled(): boolean {
  return document.documentElement.getAttribute('data-kdzs-helper') === '1'
}

export function getKdzsHelperVersion(): string {
  return document.documentElement.getAttribute('data-kdzs-helper-version') || ''
}

/** 把任务交给扩展；无扩展时静默失败 */
export function sendKdzsHelperHandoff(payload: KdzsHandoffPayload): Promise<boolean> {
  return new Promise((resolve) => {
    if (!isKdzsHelperInstalled()) {
      resolve(false)
      return
    }
    let settled = false
    const timer = window.setTimeout(() => {
      if (settled) return
      settled = true
      window.removeEventListener('message', onAck)
      resolve(false)
    }, 1500)

    function onAck(ev: MessageEvent) {
      const data = ev.data
      if (!data || data.source !== 'shippingcore-kdzs-helper' || data.type !== 'KDZS_HELPER_HANDOFF_ACK') {
        return
      }
      if (settled) return
      settled = true
      window.clearTimeout(timer)
      window.removeEventListener('message', onAck)
      resolve(!!data.ok)
    }

    window.addEventListener('message', onAck)
    window.postMessage({ source: 'shippingcore', type: 'KDZS_HELPER_HANDOFF', payload }, '*')
  })
}
