/** 与 Chrome 扩展 `kdzs-print-helper` 通信；任务主通道为云端 token */

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

/** URL 查询参数：扩展凭此从 ShippingCore 拉取任务 */
export const OSMS_HANDOFF_QUERY = '_osms_ht'

export function isKdzsHelperInstalled(): boolean {
  return document.documentElement.getAttribute('data-kdzs-helper') === '1'
}

export function getKdzsHelperVersion(): string {
  return document.documentElement.getAttribute('data-kdzs-helper-version') || ''
}

/** 把云端 handoff token 挂到快递助手打开地址上（保留原有 hash/query） */
export function appendOsmsHandoffToken(url: string, token: string): string {
  const t = String(token || '').trim()
  if (!t) return url
  try {
    const u = new URL(url)
    u.searchParams.set(OSMS_HANDOFF_QUERY, t)
    return u.toString()
  } catch {
    const join = url.includes('?') ? '&' : '?'
    return `${url}${join}${OSMS_HANDOFF_QUERY}=${encodeURIComponent(t)}`
  }
}

const WINDOW_TOKEN_PREFIX = 'OSMS_HT:'

/**
 * 打开快递助手：URL 带 `_osms_ht`，并用 window.name 备份 token（防重定向丢 query）
 */
export function openKdzsWithCloudToken(url: string, token: string): Window | null {
  const t = String(token || '').trim()
  const finalUrl = appendOsmsHandoffToken(url, t)
  const win = window.open('about:blank', '_blank')
  if (!win) return null
  if (t) {
    try {
      win.name = WINDOW_TOKEN_PREFIX + t
    } catch {
      /* ignore */
    }
  }
  win.location.href = finalUrl
  return win
}
