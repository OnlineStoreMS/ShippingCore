import type { Shipment } from '../api/shipping'

type ShipFlagRow = Pick<Shipment, 'carrierAccountId' | 'sfOrderId' | 'mailNo' | 'shipVia'> | null | undefined

/** 快递助手推送/打单确认：本系统取消运单、打印、面单、物流账号均无效 */
export function isKdzsShipment(row: ShipFlagRow): boolean {
  if (!row) return false
  const via = String(row.shipVia || '').trim().toLowerCase()
  if (via === 'kdzs') return true
  if (via === 'sf') return false
  // 有运单号但从未丰桥取号（无 sfOrderId）→ 快递助手/手工填单；勿因误绑 carrierAccountId 判成顺丰
  return !!String(row.mailNo || '').trim() && !String(row.sfOrderId || '').trim()
}

/** 顺丰取号或已关联丰桥账号（预计派送 / 云打印 / 面单存档） */
export function isSFManagedShipment(row: ShipFlagRow): boolean {
  if (!row || isKdzsShipment(row)) return false
  const via = String(row.shipVia || '').trim().toLowerCase()
  if (via === 'sf') return true
  if (String(row.sfOrderId || '').trim()) return true
  return Number(row.carrierAccountId || 0) > 0
}
