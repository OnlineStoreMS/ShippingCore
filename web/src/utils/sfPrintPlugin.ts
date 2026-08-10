/** 顺丰云打印插件：OAuth2 accessToken + SCPPrint.js（官方 COM_RECE_CLOUD_PRINT_PARSEDDATA 方案） */

export interface SFPluginPrintPayload {
  partnerId: string
  env: string // sbox | pro
  templateCode: string
  mailNo: string
  requestId?: string
  accessToken?: string
  obj?: unknown
  files?: unknown
  customTemplateCode?: string
  /** 后端拼好的托寄物/商品备注，写入 documents.remark */
  labelRemark?: string
  sdkPrintData?: {
    requestID: string
    templateCode: string
    customTemplateCode?: string
    documents: Array<Record<string, string>>
    accessToken?: string
    extJson?: Record<string, unknown>
  }
}

export type LocalPrinter = { index: number; name: string }

const PRINTER_STORAGE_KEY = 'shippingcore.sf.printerIndex'
const PRINTER_NAME_STORAGE_KEY = 'shippingcore.sf.printerName'

type LodopInstance = {
  PRINT_INIT: (title: string) => void
  SET_PRINT_PAGESIZE: (intOrient: number, pageWidth: number | string, pageHeight: number | string, pageName: string) => void
  ADD_PRINT_PDF: (top: number | string, left: number | string, width: number | string, height: number | string, data: string) => void
  ADD_PRINT_TEXT?: (
    top: number | string,
    left: number | string,
    width: number | string,
    height: number | string,
    text: string,
  ) => void
  ADD_PRINT_URL?: (top: number | string, left: number | string, width: number | string, height: number | string, url: string) => void
  SET_PRINT_MODE?: (mode: string, value: string | number | boolean) => void
  SET_PRINT_STYLE?: (styleName: string, value: string | number) => void
  SET_PRINTER_INDEX?: (index: number | string) => void | boolean
  SET_PRINTER_INDEXA?: (indexOrName: number | string) => void | boolean
  GET_PRINTER_COUNT?: () => number
  GET_PRINTER_NAME?: (index: number) => string
  PRINT: () => void | boolean
  PREVIEW: () => void
}

type SCPPrintInstance = {
  getPrinters: (cb: (result: { code: number; printers?: Array<{ name: string; index: number }> }) => void) => void
  setPrinter: (index: number | string) => void
  print: (
    data: Record<string, unknown>,
    callback: (result: unknown) => void,
    options?: { lodopFn?: string },
  ) => void
}

declare global {
  interface Window {
    SCPPrint?: new (params: Record<string, unknown>) => SCPPrintInstance
    getCLodop?: () => LodopInstance
    LODOP?: LodopInstance
    CLODOP?: LodopInstance
  }
}

function wait(ms: number) {
  return new Promise((r) => setTimeout(r, ms))
}

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const existed = document.querySelector(`script[data-sf-print="${src}"]`) as HTMLScriptElement | null
    if (existed) {
      if ((existed as HTMLScriptElement & { dataset: { loaded?: string } }).dataset.loaded === '1') {
        resolve()
        return
      }
      existed.addEventListener('load', () => resolve(), { once: true })
      existed.addEventListener('error', () => reject(new Error(src)), { once: true })
      return
    }
    const s = document.createElement('script')
    s.src = src
    s.async = true
    s.dataset.sfPrint = src
    s.onload = () => {
      s.dataset.loaded = '1'
      resolve()
    }
    s.onerror = () => reject(new Error(`加载失败: ${src}`))
    document.head.appendChild(s)
  })
}

/** 安装 C-Lodop 后本机一般会起 Web 打印服务（8000 / 18000 / 8443） */
function getLodopIfReady(): LodopInstance | null {
  const getter = window.getCLodop
  if (typeof getter === 'function') {
    try {
      return getter()
    } catch {
      return null
    }
  }
  return window.LODOP || window.CLODOP || null
}

export async function ensureLocalPrintService(): Promise<LodopInstance> {
  const ready = getLodopIfReady()
  if (ready) return ready

  // 只探测浏览器本机 C-Lodop（与访问业务站的服务器无关）
  const candidates = [
    'http://localhost:8000/CLodopfuncs.js?priority=1',
    'http://127.0.0.1:8000/CLodopfuncs.js?priority=1',
    'http://localhost:18000/CLodopfuncs.js?priority=0',
    'https://localhost:8443/CLodopfuncs.js?priority=1',
  ]
  await Promise.allSettled(candidates.map((u) => loadScript(u)))
  await wait(400)

  const again = getLodopIfReady()
  if (again) return again

  throw new Error(
    '本机 C-Lodop 服务未连通。请在运行浏览器的电脑上启动 C-Lodop，打开 http://localhost:8000/ 自检后刷新。下载：http://www.lodop.net/download.html',
  )
}

/**
 * 加载官方 SCPPrint.js（需放在 web/public/sf/SCPPrint.js）。
 * 从丰桥文档页「另存为」下载：云打印面单打印插件接口 COM_RECE_CLOUD_PRINT_PARSEDDATA。
 */
export async function ensureSCPPrintSDK(): Promise<void> {
  if (window.SCPPrint) return
  const base = (import.meta.env.BASE_URL || '/').replace(/\/?$/, '/')
  // 本地优先；CDN 为丰桥文档公布的 lodop/2.7
  const candidates = [
    `${base}sf/SCPPrint.js`,
    '/sf/SCPPrint.js',
    'https://scp-tcdn.sf-express.com/prd/sdk/lodop/2.7/SCPPrint.js',
  ]
  for (const src of candidates) {
    try {
      await loadScript(src)
      await wait(50)
      if (window.SCPPrint) return
    } catch {
      /* try next */
    }
  }
  throw new Error('SCPPRINT_SDK_MISSING')
}

export function getSavedPrinterIndex(): number | null {
  const v = localStorage.getItem(PRINTER_STORAGE_KEY)
  if (v === null || v === '') return null
  const n = Number(v)
  return Number.isFinite(n) ? n : null
}

export function getSavedPrinterName(): string {
  return localStorage.getItem(PRINTER_NAME_STORAGE_KEY) || ''
}

export function savePrinterSelection(index: number, name?: string) {
  localStorage.setItem(PRINTER_STORAGE_KEY, String(index))
  if (name != null && name !== '') {
    localStorage.setItem(PRINTER_NAME_STORAGE_KEY, name)
  }
}

/** @deprecated 使用 savePrinterSelection */
export function savePrinterIndex(index: number) {
  savePrinterSelection(index)
}

function applyPrinterIndex(LODOP: LodopInstance, printerIndex: number) {
  try {
    if (typeof LODOP.SET_PRINTER_INDEXA === 'function') {
      LODOP.SET_PRINTER_INDEXA(printerIndex)
      return
    }
    if (typeof LODOP.SET_PRINTER_INDEX === 'function') {
      LODOP.SET_PRINTER_INDEX(printerIndex)
    }
  } catch {
    /* ignore */
  }
}

/** 列出本机 C-Lodop 打印机（浏览器所在电脑） */
export async function listLocalPrinters(): Promise<LocalPrinter[]> {
  const LODOP = await ensureLocalPrintService()
  const count = typeof LODOP.GET_PRINTER_COUNT === 'function' ? LODOP.GET_PRINTER_COUNT() : 0
  const list: LocalPrinter[] = []
  for (let i = 0; i < count; i++) {
    const name =
      (typeof LODOP.GET_PRINTER_NAME === 'function' && LODOP.GET_PRINTER_NAME(i)) || `打印机 ${i}`
    list.push({ index: i, name: String(name) })
  }
  if (list.length > 0) return list

  // Lodop 未暴露枚举时，尝试 SCPPrint
  try {
    await ensureSCPPrintSDK()
    if (!window.SCPPrint) return list
    const sdk = new window.SCPPrint({ env: 'sbox', partnerID: 'probe', notips: true, callback: () => {} })
    const viaSdk = await new Promise<LocalPrinter[]>((resolve) => {
      const timer = window.setTimeout(() => resolve([]), 4000)
      sdk.getPrinters((result) => {
        window.clearTimeout(timer)
        if (result.code === 1 && result.printers?.length) {
          resolve(result.printers.map((p) => ({ index: p.index, name: p.name })))
          return
        }
        resolve([])
      })
    })
    return viaSdk
  } catch {
    return list
  }
}

async function blobToBase64(blob: Blob): Promise<string> {
  const buf = await blob.arrayBuffer()
  let binary = ''
  const bytes = new Uint8Array(buf)
  const chunk = 0x8000
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode.apply(null, Array.from(bytes.subarray(i, i + chunk)))
  }
  return btoa(binary)
}

/** 用本机 C-Lodop 打印 PDF */
export async function printPDFWithLocalService(
  pdfBlob: Blob,
  opts?: { preview?: boolean; title?: string; printerIndex?: number | null },
) {
  const LODOP = await ensureLocalPrintService()
  const base64 = await blobToBase64(pdfBlob)
  LODOP.PRINT_INIT(opts?.title || '顺丰面单')
  const printerIndex = opts?.printerIndex ?? getSavedPrinterIndex()
  if (printerIndex != null) {
    applyPrinterIndex(LODOP, printerIndex)
  }
  try {
    LODOP.SET_PRINT_PAGESIZE(1, '76mm', '130mm', '')
  } catch {
    /* ignore */
  }
  LODOP.ADD_PRINT_PDF(0, 0, '100%', '100%', base64)
  if (opts?.preview) {
    LODOP.PREVIEW()
  } else {
    LODOP.PRINT()
  }
  await wait(300)
}

/** 官方插件打印：SCPPrint.print({ accessToken, templateCode, documents }) */
export async function printWithSFPlugin(
  payload: SFPluginPrintPayload,
  opts?: { preview?: boolean; printerIndex?: number | null },
) {
  try {
    await ensureLocalPrintService()
  } catch (e) {
    throw e
  }

  try {
    await ensureSCPPrintSDK()
  } catch {
    throw new Error('SCPPRINT_SDK_MISSING')
  }

  if (!window.SCPPrint) {
    throw new Error('SCPPRINT_SDK_MISSING')
  }

  const accessToken = payload.accessToken || payload.sdkPrintData?.accessToken
  if (!accessToken) {
    throw new Error('缺少丰桥 accessToken，请检查顾客编码/校验码/环境后重试')
  }

  const env = payload.env === 'pro' || payload.env === 'prod' ? 'pro' : 'sbox'
  const printSdk = new window.SCPPrint({
    env,
    partnerID: payload.partnerId,
    notips: false,
    callback: () => {},
  })

  const printerIndex = opts?.printerIndex ?? getSavedPrinterIndex()
  if (printerIndex == null) {
    throw new Error('PRINTER_NOT_SELECTED')
  }
  printSdk.setPrinter(printerIndex)

  const docs =
    payload.sdkPrintData?.documents ||
    ([
      {
        masterWaybillNo: payload.mailNo,
        ...(payload.labelRemark
          ? {
              remark: payload.labelRemark,
              cargoDesc: payload.labelRemark,
              goods: payload.labelRemark,
              product: payload.labelRemark,
            }
          : {}),
      },
    ] as Array<Record<string, string>>)
  const data: Record<string, unknown> = {
    requestID: payload.sdkPrintData?.requestID || payload.requestId || `SC-${Date.now()}`,
    accessToken,
    templateCode: payload.sdkPrintData?.templateCode || payload.templateCode,
    documents: docs,
  }
  const customTpl =
    payload.sdkPrintData?.customTemplateCode || payload.customTemplateCode
  if (customTpl) {
    data.customTemplateCode = customTpl
  }
  const extJson = (payload.sdkPrintData as { extJson?: Record<string, unknown> } | undefined)?.extJson
  if (extJson) {
    data.extJson = extJson
  } else if (payload.labelRemark) {
    data.extJson = {
      remark: payload.labelRemark,
      cargoDesc: payload.labelRemark,
      goods: payload.labelRemark,
      product: payload.labelRemark,
    }
  }

  await new Promise<void>((resolve, reject) => {
    const timer = window.setTimeout(() => reject(new Error('插件打印超时')), 20000)
    printSdk.print(
      data,
      (result) => {
        window.clearTimeout(timer)
        const code = (result as { code?: number })?.code
        if (code === 0 || code === 1 || code === undefined) {
          resolve()
          return
        }
        reject(new Error(`插件打印失败: ${JSON.stringify(result)}`))
      },
      { lodopFn: opts?.preview ? 'PREVIEW' : 'PRINT' },
    )
  })

  await wait(300)
}

/** 向指定本机打印机发送一页测试内容 */
export async function testPrintLocalPrinter(opts: { printerIndex: number; printerName?: string }) {
  const LODOP = await ensureLocalPrintService()
  const name = opts.printerName || `索引 ${opts.printerIndex}`
  LODOP.PRINT_INIT('ShippingCore 打印机测试')
  applyPrinterIndex(LODOP, opts.printerIndex)
  try {
    LODOP.SET_PRINT_PAGESIZE(1, '76mm', '130mm', '')
  } catch {
    /* ignore */
  }
  try {
    LODOP.SET_PRINT_STYLE?.('FontSize', 12)
    LODOP.SET_PRINT_STYLE?.('Bold', 1)
  } catch {
    /* ignore */
  }
  const text = [
    '发货中心 · 打印机测试',
    '',
    `打印机：${name}`,
    `索引：${opts.printerIndex}`,
    `时间：${new Date().toLocaleString()}`,
    '',
    '若本页从该打印机出纸，说明选择正确。',
  ].join('\n')
  if (typeof LODOP.ADD_PRINT_TEXT === 'function') {
    LODOP.ADD_PRINT_TEXT(8, 6, '68mm', '110mm', text)
  } else {
    throw new Error('本机打印组件不支持测试文本打印')
  }
  LODOP.PRINT()
  await wait(300)
}

/** 下载插件面单 JSON，便于排查 */
export function downloadPluginDataJSON(payload: SFPluginPrintPayload) {
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `sf-plugin-${payload.mailNo || 'label'}.json`
  a.click()
  URL.revokeObjectURL(url)
}
