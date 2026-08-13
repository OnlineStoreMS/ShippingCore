import { shippingApi } from '../api/shipping'
import { printWithSFPlugin } from './sfPrintPlugin'

/** 按物流账号「打印通道」打面单：pdf=浏览器打开官方 PDF；plugin=C-Lodop 插件 */
export async function printShipmentByChannel(opts: {
  shipmentId: number
  printChannel?: string | null
  /** 仅 plugin 通道需要 */
  printerIndex?: number | null
  /** 覆盖账号默认模板（plugin） */
  templateCode?: string
  customTemplateCode?: string
  /** 再次打印时可指定物流账号 */
  carrierAccountId?: number
}): Promise<'pdf' | 'plugin'> {
  const channel = (opts.printChannel || 'plugin').toLowerCase() === 'pdf' ? 'pdf' : 'plugin'
  const carrierAccountId = opts.carrierAccountId
  if (channel === 'pdf') {
    await shippingApi.printShipment(opts.shipmentId, carrierAccountId ? { carrierAccountId } : undefined)
    const file = await shippingApi.fetchShipmentLabelFile(
      opts.shipmentId,
      carrierAccountId ? { carrierAccountId } : undefined,
    )
    const blobUrl = URL.createObjectURL(file)
    const win = window.open(blobUrl, '_blank')
    if (!win) {
      URL.revokeObjectURL(blobUrl)
      throw new Error('浏览器拦截了弹窗，请允许后重试；也可在发货单详情查看存档 PDF')
    }
    // 延后释放，避免标签页尚未加载完 PDF 就被回收
    window.setTimeout(() => URL.revokeObjectURL(blobUrl), 120_000)
    return 'pdf'
  }
  if (opts.printerIndex == null) {
    throw new Error('PRINTER_NOT_SELECTED')
  }
  const params: {
    templateCode?: string
    customTemplateCode?: string
    carrierAccountId?: number
  } = {}
  if (opts.templateCode) params.templateCode = opts.templateCode
  if (opts.customTemplateCode) params.customTemplateCode = opts.customTemplateCode
  if (carrierAccountId) params.carrierAccountId = carrierAccountId
  const pluginData = await shippingApi.fetchShipmentPrintPluginData(
    opts.shipmentId,
    Object.keys(params).length ? params : undefined,
  )
  await printWithSFPlugin(pluginData, { printerIndex: opts.printerIndex })
  return 'plugin'
}

export function isPdfPrintChannel(printChannel?: string | null): boolean {
  return (printChannel || '').toLowerCase() === 'pdf'
}
