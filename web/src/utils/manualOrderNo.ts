/** 与订单中心同结构的日期+4 位日序号（本机按日自增），不含 OC 前缀 */
function nextDateSeqSuffix(): string {
  const now = new Date()
  const y = now.getFullYear()
  const m = String(now.getMonth() + 1).padStart(2, '0')
  const d = String(now.getDate()).padStart(2, '0')
  const day = `${y}${m}${d}`
  const key = `shippingcore.manualOCSeq.${day}`
  let seq = Number(localStorage.getItem(key) || '0')
  if (!Number.isFinite(seq) || seq < 0) seq = 0
  seq += 1
  if (seq > 9999) seq = 1
  localStorage.setItem(key, String(seq))
  return `${day}${String(seq).padStart(4, '0')}`
}

/** 手工打单占位单号：SC-MANUAL-YYYYMMDD#### */
export function nextSCManualOrderNo(): string {
  return `SC-MANUAL-${nextDateSeqSuffix()}`
}
