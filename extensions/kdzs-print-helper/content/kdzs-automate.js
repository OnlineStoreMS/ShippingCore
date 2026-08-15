/**
 * 快递助手批打页自动化：
 * - 按单号搜索并勾选订单
 * - 尝试选择模板
 * - 尝试勾选指定商品（部分发货）
 * - 不自动点「打印」
 */
;(() => {
  const PANEL_ID = 'osms-kdzs-helper-panel'
  const HANDOFF_MAX_AGE_MS = 30 * 60 * 1000

  /** @type {any} */
  let handoff = null
  let running = false
  let lastLog = []

  function log(msg, level = 'info') {
    const line = `[${new Date().toLocaleTimeString()}] ${msg}`
    lastLog = [...lastLog.slice(-40), line]
    console[level === 'error' ? 'error' : 'log']('[OSMS-KDZS]', msg)
    renderPanel()
  }

  function sleep(ms) {
    return new Promise((r) => setTimeout(r, ms))
  }

  function visible(el) {
    if (!el || !(el instanceof HTMLElement)) return false
    const st = getComputedStyle(el)
    if (st.display === 'none' || st.visibility === 'hidden' || Number(st.opacity) === 0) return false
    const r = el.getBoundingClientRect()
    return r.width > 0 && r.height > 0
  }

  function textOf(el) {
    return (el?.innerText || el?.textContent || '').replace(/\s+/g, ' ').trim()
  }

  function clickEl(el) {
    if (!el) return false
    el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    el.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))
    el.click()
    return true
  }

  function findButtonsByText(needles, root = document) {
    const want = needles.map((n) => n.toLowerCase())
    const nodes = root.querySelectorAll('button, a, span, div, li, label')
    const hits = []
    for (const el of nodes) {
      if (!visible(el)) continue
      const t = textOf(el).toLowerCase()
      if (!t || t.length > 40) continue
      if (want.some((n) => t === n || t.includes(n))) hits.push(el)
    }
    return hits
  }

  function findInputByPlaceholder(needles) {
    const inputs = document.querySelectorAll('input, textarea')
    for (const el of inputs) {
      if (!visible(el)) continue
      const ph = `${el.getAttribute('placeholder') || ''} ${el.getAttribute('aria-label') || ''}`.toLowerCase()
      if (needles.some((n) => ph.includes(n))) return el
    }
    return null
  }

  function setInputValue(el, value) {
    if (!el) return false
    const proto = el.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype
    const desc = Object.getOwnPropertyDescriptor(proto, 'value')
    desc?.set?.call(el, value)
    el.dispatchEvent(new Event('input', { bubbles: true }))
    el.dispatchEvent(new Event('change', { bubbles: true }))
    el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    el.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', bubbles: true }))
    return true
  }

  function orderKeywords(order) {
    const keys = []
    for (const k of [order.orderNo, order.platformSysTid, order.platformOrderId, order.tid, order.sysTid]) {
      const v = String(k || '').trim()
      if (v && !keys.includes(v)) keys.push(v)
    }
    return keys
  }

  function goodsKeywords(order) {
    const out = []
    for (const g of order.goods || []) {
      for (const k of [g.skuName, g.title, g.outerId]) {
        const v = String(k || '').trim()
        if (v && v.length >= 2 && !out.includes(v)) out.push(v)
      }
    }
    return out
  }

  /**
   * 抖音批打页（dydf.kdzs.com）真实 DOM：
   * - 订单行：div[class*="packageItem"]，选中后带 pack_selected，背景变绿
   * - 订单勾选：左侧 input._merge_label_*（约 x=50）
   * - 商品勾选：商品列内 checkbox（约 x=200），默认常为勾选，不等于「选中整单」
   * - 模板：label.ant-radio-wrapper（文案可能带数量后缀，如「抖音中通一联单80」）
   * - 主按钮：「打印快递单」「发货」，无「选择发货」
   */
  function listPackageItems() {
    return [...document.querySelectorAll('[class*="packageItem"]')].filter(
      (el) => el instanceof HTMLElement && visible(el),
    )
  }

  function findRowContaining(text) {
    const needle = String(text || '').toLowerCase()
    if (!needle) return null
    for (const row of listPackageItems()) {
      if (textOf(row).toLowerCase().includes(needle)) return row
    }
    return null
  }

  function isInputChecked(el) {
    if (!el) return false
    if (el instanceof HTMLInputElement) return !!el.checked
    const aria = el.getAttribute?.('aria-checked')
    if (aria === 'true') return true
    if (aria === 'false') return false
    const cls = el.className || ''
    if (typeof cls === 'string' && /is-checked|checked/.test(cls)) return true
    const wrap = el.closest?.('.el-checkbox, .ant-checkbox-wrapper, label')
    if (wrap && /is-checked|checked/.test(wrap.className || '')) return true
    return false
  }

  function setBoxChecked(box, want) {
    if (!box) return false
    const input =
      box instanceof HTMLInputElement
        ? box
        : box.querySelector?.('input[type="checkbox"]') || box
    const currently = isInputChecked(input) || isInputChecked(box)
    if (currently === want) return true
    clickEl(input instanceof HTMLElement ? input : box)
    // React/Ant 受控：若 click 未生效再补一次原生
    if (input instanceof HTMLInputElement && input.checked !== want) {
      clickEl(input)
    }
    return isInputChecked(input) === want || true
  }

  /** 订单行左侧「选中整单」勾选（不要用商品列勾选） */
  function orderCheckboxInRow(row) {
    if (!row) return null
    const boxes = [...row.querySelectorAll('input[type="checkbox"]')].filter(
      (el) => el instanceof HTMLInputElement && visible(el),
    )
    const byClass = boxes.find((el) => /merge_label|merge-label/i.test(el.className || ''))
    if (byClass) return byClass
    // 源列（店铺名旁）优先
    const sourceCol = row.querySelector('[class*="tradeSourc"], [class*="tradeSource"]')
    if (sourceCol) {
      const b = sourceCol.querySelector('input[type="checkbox"]')
      if (b) return b
    }
    // 最左侧的勾选
    return boxes.sort((a, b) => a.getBoundingClientRect().x - b.getBoundingClientRect().x)[0] || null
  }

  function productCheckboxesInRow(row) {
    if (!row) return []
    const orderBox = orderCheckboxInRow(row)
    const goodsCol = row.querySelector('[class*="packageOrd"], [class*="packageOrder"]')
    const scope = goodsCol || row
    return [...scope.querySelectorAll('input[type="checkbox"]')].filter(
      (el) => el instanceof HTMLInputElement && visible(el) && el !== orderBox,
    )
  }

  function isPackageSelected(row) {
    if (!row) return false
    if (/pack_selected|selected/i.test(row.className || '')) return true
    const box = orderCheckboxInRow(row)
    return isInputChecked(box)
  }

  function uncheckOrderRow(row) {
    const box = orderCheckboxInRow(row)
    if (box && isInputChecked(box)) setBoxChecked(box, false)
  }

  function checkOrderRow(row) {
    const box = orderCheckboxInRow(row)
    if (!box) return false
    setBoxChecked(box, true)
    return isPackageSelected(row) || isInputChecked(box)
  }

  async function uncheckAllOrders() {
    const rows = listPackageItems()
    let n = 0
    for (const row of rows) {
      if (isPackageSelected(row)) {
        uncheckOrderRow(row)
        n += 1
      }
    }
    if (n) log(`已取消本页订单勾选 ${n} 行（商品列勾选默认常开，不代表选中整单）`)
    await sleep(200)
  }

  function preferSearchKeys(order) {
    const keys = []
    for (const k of [order.platformOrderId, order.tid, order.platformSysTid, order.sysTid, order.orderNo]) {
      const v = String(k || '').trim()
      if (v && !keys.includes(v)) keys.push(v)
    }
    return keys
  }

  async function queryByOrderNo(orderNo) {
    const platformInput = findInputByPlaceholder(['平台订单编号'])
    const sysInput = findInputByPlaceholder(['系统订单编号'])
    // 系统编号字段对「系统编号：27118…」更准；平台订单编号对「订单编号：692…」更准
    const looksSys = /^\d{16,}$/.test(String(orderNo)) && String(orderNo).startsWith('2')
    const input = looksSys
      ? sysInput || platformInput
      : platformInput || sysInput
    if (!input) return false
    setInputValue(input, orderNo)
    await sleep(200)
    const searchBtns = findButtonsByText(['查询', '搜索'])
    const btn = searchBtns.find((b) => /查\s*询|搜索/.test(textOf(b))) || searchBtns[0]
    if (btn) {
      clickEl(btn)
      await sleep(1400)
    } else {
      await sleep(600)
    }
    return true
  }

  function toYmd(v) {
    if (!v) return ''
    const d = new Date(v)
    if (!Number.isNaN(d.getTime())) {
      const p = (n) => String(n).padStart(2, '0')
      return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
    }
    const m = String(v).match(/(\d{4})-(\d{2})-(\d{2})/)
    return m ? `${m[1]}-${m[2]}-${m[3]}` : ''
  }

  function resolveOrderTimeRange() {
    if (handoff?.orderTimeFrom && handoff?.orderTimeTo) {
      return { fromYmd: toYmd(handoff.orderTimeFrom), toYmd: toYmd(handoff.orderTimeTo) }
    }
    const times = []
    for (const o of handoff?.orders || []) {
      const y = toYmd(o.payTime || o.orderedAt)
      if (y) times.push(y)
    }
    if (!times.length) return null
    times.sort()
    return { fromYmd: times[0], toYmd: times[times.length - 1] }
  }

  async function ensurePickerMonth(picker, year, month) {
    for (let i = 0; i < 24; i++) {
      const label = textOf(picker.querySelector('.picker-range-current-month'))
      const m = label.match(/(\d{4})\s*年\s*(\d{1,2})\s*月/)
      if (!m) return false
      const cy = Number(m[1])
      const cm = Number(m[2])
      if (cy === year && cm === month) return true
      const diff = (year - cy) * 12 + (month - cm)
      const heads = [...picker.querySelectorAll('span, a, div, button')].filter((el) => {
        const t = textOf(el)
        return t === '«' || t === '»'
      })
      const target = diff < 0 ? heads.find((el) => textOf(el) === '«') : heads.find((el) => textOf(el) === '»')
      if (!target) return false
      clickEl(target)
      await sleep(180)
    }
    return false
  }

  async function pickDayInPicker(picker, ymd) {
    if (!picker || !ymd) return false
    const [y, m] = ymd.split('-').map(Number)
    await ensurePickerMonth(picker, y, m)
    const cell = picker.querySelector(`.day-cell[title="${ymd}"]`)
    if (!cell || cell.classList.contains('disabled')) {
      log(`日期不可选：${ymd}`, 'error')
      return false
    }
    clickEl(cell)
    await sleep(200)
    return true
  }

  /** 把「下单时间」改成任务订单实际日期，避免默认近一月导致单量过大 */
  async function setOrderTimeRange() {
    const range = resolveOrderTimeRange()
    if (!range?.fromYmd || !range?.toYmd) {
      log('任务无付款/下单时间，跳过时间筛选')
      return false
    }

    const panel = document.querySelector('.range-picker-panel')
    if (!panel) {
      log('未找到下单时间控件', 'error')
      return false
    }

    const curBegin = (panel.getAttribute('data-begin-date') || '').slice(0, 10)
    const curEnd = (panel.getAttribute('data-end-date') || '').slice(0, 10)
    if (curBegin === range.fromYmd && curEnd === range.toYmd) {
      log(`下单时间已是 ${range.fromYmd} ~ ${range.toYmd}`)
      return true
    }

    log(`设置下单时间：${range.fromYmd} ~ ${range.toYmd}`)
    clickEl(panel)
    await sleep(450)

    let pop = document.querySelector('.range-picker-popover')
    if (!pop) {
      await sleep(400)
      pop = document.querySelector('.range-picker-popover')
    }
    if (!pop) {
      log('下单时间弹层未打开', 'error')
      return false
    }

    const pickers = [...pop.querySelectorAll('.kdzs-design-date-picker')]
    if (pickers.length < 2) {
      log('下单时间起止选择器不完整', 'error')
      return false
    }

    const ok1 = await pickDayInPicker(pickers[0], range.fromYmd)
    const ok2 = await pickDayInPicker(pickers[1], range.toYmd)
    if (!ok1 || !ok2) return false

    const okBtn = [...pop.querySelectorAll('.submit-btn, button, div, span')].find(
      (el) => textOf(el).replace(/\s/g, '') === '确定' && visible(el),
    )
    if (okBtn) clickEl(okBtn)
    await sleep(400)

    const begin = (document.querySelector('.range-picker-panel')?.getAttribute('data-begin-date') || '').slice(0, 10)
    const end = (document.querySelector('.range-picker-panel')?.getAttribute('data-end-date') || '').slice(0, 10)
    if (begin === range.fromYmd && end === range.toYmd) {
      log(`下单时间已设置为 ${begin} ~ ${end}`)
      return true
    }
    log(`下单时间设置后为 ${begin || '?'} ~ ${end || '?'}，请人工确认`, 'error')
    return begin === range.fromYmd || end === range.toYmd
  }

  async function searchAndSelectOrders() {
    const orders = handoff?.orders || []
    if (!orders.length) {
      log('无订单信息，跳过选单', 'error')
      return 0
    }

    await setOrderTimeRange()
    await uncheckAllOrders()

    let selected = 0
    for (const order of orders) {
      const keys = preferSearchKeys(order)
      if (!keys.length) continue
      const label = order.orderNo || keys[0]
      log(`查找订单 ${keys.join(' / ')}`)

      let hit = null
      for (const k of keys) {
        await queryByOrderNo(k)
        for (const kk of keys) {
          hit = findRowContaining(kk)
          if (hit) break
        }
        if (!hit) {
          const rows = listPackageItems()
          if (rows.length === 1) hit = rows[0]
        }
        if (hit) break
      }

      if (!hit) {
        for (const k of keys) {
          hit = findRowContaining(k)
          if (hit) break
        }
      }
      if (!hit) {
        log(`未在列表中找到：${label}`, 'error')
        continue
      }

      // 只保留目标行订单勾选
      for (const row of listPackageItems()) {
        if (row !== hit && isPackageSelected(row)) uncheckOrderRow(row)
      }

      if (checkOrderRow(hit)) {
        selected += 1
        log(`已勾选整单：${label}`)
        await selectGoodsInRow(hit, order)
      } else {
        log(`勾选失败：${label}`, 'error')
      }
      await sleep(300)
    }
    return selected
  }

  async function selectGoodsInRow(row, order) {
    const want = goodsKeywords(order)
    const boxes = productCheckboxesInRow(row)
    if (!boxes.length) {
      log('未找到行内商品勾选（可能单 SKU 已默认勾选）')
      return
    }
    if (!want.length) {
      for (const box of boxes) setBoxChecked(box, true)
      return
    }
    let matched = 0
    for (const box of boxes) {
      const cellText = textOf(box.closest('[class*="packageOrd"], [class*="container"]') || box.parentElement)
      const hit = want.some((k) => cellText.toLowerCase().includes(String(k).toLowerCase()))
      setBoxChecked(box, hit)
      if (hit) matched += 1
    }
    if (matched) log(`行内商品勾选 ${matched}/${boxes.length}`)
    else {
      for (const box of boxes) setBoxChecked(box, true)
      log('行内商品未能按明细匹配，已勾选该行全部商品', 'error')
    }
  }

  async function clickSelectShip() {
    const exact = findButtonsByText(['选择发货', '选中发货', '确认选择'])
    const btn = exact.find((el) => /选择发货|选中发货/.test(textOf(el))) || exact[0]
    if (!btn) {
      log('批打页无「选择发货」，请人工核对后点「打印快递单」')
      return false
    }
    log(`点击：${textOf(btn)}`)
    clickEl(btn)
    await sleep(1200)
    return true
  }

  async function selectTemplate() {
    const rawName = String(handoff?.templateName || '').trim()
    if (!rawName) {
      log('未指定模板名，跳过模板选择')
      return false
    }
    // 页面文案常带数量后缀：抖音中通一联单80
    const name = rawName.replace(/\s+/g, '')
    const nameLoose = name.replace(/\d+$/g, '')
    log(`尝试选择模板：${rawName}`)

    let radios = []
    for (let i = 0; i < 12; i++) {
      radios = [...document.querySelectorAll('label.ant-radio-wrapper, .ant-radio-wrapper')].filter(
        (el) => el instanceof HTMLElement,
      )
      if (radios.length) break
      await sleep(300)
    }
    if (!radios.length) {
      log('未找到快递模板单选，请手动选择', 'error')
      return false
    }

    let best = null
    let bestScore = 0
    for (const lab of radios) {
      const t = textOf(lab).replace(/\s+/g, '')
      if (!t || t.length > 48) continue
      let score = 0
      if (t === name) score = 100
      else if (t.startsWith(name) || name.startsWith(t.replace(/\d+$/, ''))) score = 92
      else if (t.includes(name) || name.includes(t)) score = 80
      else if (nameLoose && (t.includes(nameLoose) || t.replace(/\d+$/g, '') === nameLoose)) score = 75
      if (score > bestScore) {
        bestScore = score
        best = lab
      }
    }

    if (!best || bestScore < 50) {
      log(
        `未能匹配模板「${rawName}」。可选：${radios
          .slice(0, 6)
          .map((r) => textOf(r))
          .join(' / ')}`,
        'error',
      )
      return false
    }

    try {
      best.scrollIntoView({ block: 'center', inline: 'nearest' })
    } catch {
      /* ignore */
    }
    await sleep(150)

    const input =
      best.querySelector('input.ant-radio-input') ||
      best.querySelector('input[type="radio"]') ||
      best.querySelector('input')

    // Ant Design：点原生 radio input 才稳定选中
    if (input instanceof HTMLElement) clickEl(input)
    else clickEl(best)
    await sleep(350)

    let ok =
      best.classList.contains('ant-radio-wrapper-checked') ||
      !!best.querySelector('.ant-radio-checked') ||
      !!best.querySelector('input:checked')
    if (!ok) {
      clickEl(best)
      await sleep(300)
      ok =
        best.classList.contains('ant-radio-wrapper-checked') ||
        !!best.querySelector('.ant-radio-checked') ||
        !!best.querySelector('input:checked')
    }

    if (ok) {
      log(`已选择模板：${textOf(best)}`)
      return true
    }
    log(`已点击模板「${textOf(best)}」但未检测到选中态，请人工确认`, 'error')
    return false
  }

  async function selectGoods() {
    return 0
  }

  async function runAutomation() {
    if (running) return
    running = true
    renderPanel()
    try {
      if (!handoff) {
        log('没有待执行任务。请先在发货中心点「打开快递助手」。', 'error')
        return
      }
      const age = Date.now() - Number(handoff.createdAt || handoff.savedAt || 0)
      if (age > HANDOFF_MAX_AGE_MS) {
        log('任务已过期（>30 分钟），请回发货中心重新打开', 'error')
        return
      }

      log('开始自动化（不会自动打印）…')
      await sleep(800)

      for (let i = 0; i < 15; i++) {
        if (document.body && textOf(document.body).length > 50) break
        await sleep(400)
      }

      // 先选单（查询会刷新）；模板放最后，避免被冲掉
      const n = await searchAndSelectOrders()
      log(`订单勾选结果：${n}/${(handoff.orders || []).length}`)
      await selectTemplate()
      await clickSelectShip()

      log('自动化完成：请人工核对勾选与模板后点击「打印快递单」。打印后回发货中心「同步单号→确认发货」。')
    } catch (e) {
      log(`自动化异常：${e?.message || e}`, 'error')
    } finally {
      running = false
      renderPanel()
    }
  }

  function renderPanel() {
    let el = document.getElementById(PANEL_ID)
    if (!el) {
      el = document.createElement('div')
      el.id = PANEL_ID
      el.innerHTML = `
        <div class="hd">
          <strong>OSMS 打单助手</strong>
          <button type="button" data-act="min">—</button>
        </div>
        <div class="bd">
          <div class="status"></div>
          <div class="logs"></div>
          <div class="actions">
            <button type="button" data-act="run">执行选单</button>
            <button type="button" data-act="reload">读取任务</button>
            <button type="button" data-act="clear">清除</button>
          </div>
          <div class="tip">不会自动点击打印</div>
        </div>
      `
      const style = document.createElement('style')
      style.textContent = `
        #${PANEL_ID}{
          position:fixed; right:16px; bottom:16px; z-index:2147483646;
          width:320px; max-height:52vh; overflow:hidden;
          background:#0f172a; color:#e2e8f0; border-radius:12px;
          box-shadow:0 12px 40px rgba(0,0,0,.35); font:12px/1.45 system-ui,sans-serif;
        }
        #${PANEL_ID}.min .bd{display:none}
        #${PANEL_ID} .hd{
          display:flex; align-items:center; justify-content:space-between;
          padding:10px 12px; background:#1e293b; cursor:move;
        }
        #${PANEL_ID} .hd button,#${PANEL_ID} .actions button{
          border:0; border-radius:6px; padding:4px 8px; cursor:pointer;
          background:#334155; color:#f8fafc;
        }
        #${PANEL_ID} .bd{padding:10px 12px 12px}
        #${PANEL_ID} .status{margin-bottom:8px; color:#93c5fd}
        #${PANEL_ID} .logs{
          max-height:180px; overflow:auto; background:#020617; border-radius:8px;
          padding:8px; white-space:pre-wrap; color:#cbd5e1; margin-bottom:8px;
        }
        #${PANEL_ID} .actions{display:flex; gap:6px; flex-wrap:wrap}
        #${PANEL_ID} .actions button[data-act="run"]{background:#2563eb}
        #${PANEL_ID} .tip{margin-top:8px; color:#94a3b8}
      `
      document.documentElement.appendChild(style)
      document.documentElement.appendChild(el)
      el.addEventListener('click', (e) => {
        const t = e.target
        if (!(t instanceof HTMLElement)) return
        const act = t.getAttribute('data-act')
        if (act === 'min') el.classList.toggle('min')
        if (act === 'run') void runAutomation()
        if (act === 'reload') void loadHandoff(true)
        if (act === 'clear') {
          chrome.runtime.sendMessage({ type: 'KDZS_HELPER_CLEAR_HANDOFF' }, () => {
            handoff = null
            lastLog = []
            log('已清除任务')
          })
        }
      })
    }

    const status = el.querySelector('.status')
    const logs = el.querySelector('.logs')
    const runBtn = el.querySelector('[data-act="run"]')
    if (status) {
      if (!handoff) status.textContent = '无待办任务'
      else {
        const n = (handoff.orders || []).length
        const nos = (handoff.orders || [])
          .map((o) => o.orderNo || o.platformOrderId || o.platformSysTid || '')
          .filter(Boolean)
          .slice(0, 3)
          .join('、')
        const more = n > 3 ? ` 等${n}单` : ''
        status.textContent = `任务：${n} 单 · ${nos || '无单号'}${more} · 模板 ${handoff.templateName || '未指定'}${running ? ' · 执行中…' : ''}`
      }
    }
    if (logs) logs.textContent = lastLog.join('\n') || '等待任务…'
    if (runBtn instanceof HTMLButtonElement) runBtn.disabled = running
  }

  const OSMS_HANDOFF_QUERY = '_osms_ht'
  const WINDOW_TOKEN_PREFIX = 'OSMS_HT:'
  /** 首次从 URL/name 读出后缓存，避免重试时已被清掉 */
  let pendingCloudToken = ''

  function readTokenFromSearch(search) {
    try {
      return new URLSearchParams(search).get(OSMS_HANDOFF_QUERY) || ''
    } catch {
      return ''
    }
  }

  /** 从 URL query / hash / window.name 取云端 token（只消费一次，之后用缓存） */
  function peekCloudToken() {
    if (pendingCloudToken) return pendingCloudToken

    let token = ''
    try {
      token = readTokenFromSearch(location.search) || ''
      if (!token && location.hash) {
        const hash = location.hash.replace(/^#/, '')
        const qIdx = hash.indexOf('?')
        if (qIdx >= 0) token = readTokenFromSearch(hash.slice(qIdx)) || ''
        if (!token) {
          const m = hash.match(new RegExp(`[?&#]?${OSMS_HANDOFF_QUERY}=([^&]+)`))
          if (m) token = decodeURIComponent(m[1])
        }
      }
    } catch {
      /* ignore */
    }

    if (!token) {
      try {
        const n = String(window.name || '')
        if (n.startsWith(WINDOW_TOKEN_PREFIX)) {
          token = n.slice(WINDOW_TOKEN_PREFIX.length).trim()
          window.name = ''
        }
      } catch {
        /* ignore */
      }
    }

    token = String(token || '').trim()
    if (!token) return ''

    pendingCloudToken = token
    try {
      if (location.search.includes(OSMS_HANDOFF_QUERY)) {
        const u = new URL(location.href)
        u.searchParams.delete(OSMS_HANDOFF_QUERY)
        history.replaceState(null, '', u.toString())
      }
    } catch {
      /* ignore */
    }
    return pendingCloudToken
  }

  function fetchCloudHandoff(token) {
    return new Promise((resolve) => {
      chrome.runtime.sendMessage({ type: 'KDZS_HELPER_FETCH_CLOUD', token }, (res) => {
        if (chrome.runtime.lastError) {
          resolve({ ok: false, error: chrome.runtime.lastError.message })
          return
        }
        resolve(res || { ok: false, error: 'empty response' })
      })
    })
  }

  function applyHandoff(payload, source, manual) {
    handoff = { ...payload, savedAt: payload.savedAt || Date.now() }
    renderPanel()
    log(`已加载任务（${source}）：${(handoff.orders || []).length} 单`)
    if (!manual && !running) void runAutomation()
  }

  function loadHandoff(manual = false) {
    return (async () => {
      const cloudToken = peekCloudToken()
      if (cloudToken) {
        log(`正在从云端拉取任务…`)
        const res = await fetchCloudHandoff(cloudToken)
        if (res?.ok && res.payload) {
          pendingCloudToken = ''
          applyHandoff(res.payload, '云端', manual)
          return true
        }
        log(`云端拉取失败：${res?.error || '未知错误'}`, 'error')
      }

      const local = await new Promise((resolve) => {
        chrome.runtime.sendMessage({ type: 'KDZS_HELPER_GET_HANDOFF' }, (res) => {
          if (chrome.runtime.lastError) {
            resolve(null)
            return
          }
          resolve(res?.payload || null)
        })
      })
      if (local) {
        applyHandoff(local, '扩展存储', manual)
        return true
      }

      handoff = null
      renderPanel()
      if (manual) {
        log('无任务：请从发货中心点「打开快递助手」（会带云端 token）')
      }
      return false
    })()
  }

  // 仅顶层页执行，避免 iframe 抢先一次性消费 token
  if (window !== window.top) return

  // 仅在批打相关页显示面板；其它页也允许手动读取
  const href = location.href
  const likelyPrint =
    /printBatch|batchPrint|print/i.test(href) || /newIndex|df\.kdzs|kdzs\.com/i.test(href)

  if (likelyPrint) {
    renderPanel()
    void loadHandoff(false)
    setTimeout(() => {
      if (!handoff) void loadHandoff(false)
    }, 1500)
    setTimeout(() => {
      if (!handoff) void loadHandoff(false)
    }, 4000)

    chrome.storage.onChanged.addListener((changes, area) => {
      if (area !== 'local' || !changes.kdzsHandoff) return
      const next = changes.kdzsHandoff.newValue
      if (!next) return
      if (handoff && handoff.createdAt === next.createdAt) return
      applyHandoff(next, '存储更新', false)
    })
  }
})()
