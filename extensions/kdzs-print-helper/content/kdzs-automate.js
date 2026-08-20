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

  /** 快递助手分销门户：df.kdzs.com 顶栏切平台，批打 DOM 在平台 iframe 内 */
  const PLATFORM_HOST = {
    FXG: 'dydf.kdzs.com',
    DY: 'dydf.kdzs.com',
    TB: 'tbdf.kdzs.com',
    TAOBAO: 'tbdf.kdzs.com',
    XHS: 'xhsdf.kdzs.com',
    PDD: 'pdddf.kdzs.com',
    KSXD: 'ksdf.kdzs.com',
    DFHAND: 'hand.kdzs.com',
    HAND: 'hand.kdzs.com',
    MANUAL: 'hand.kdzs.com',
  }

  /** hash ?platform= 与顶栏文案 */
  const PLATFORM_UI = {
    FXG: { query: 'fxg', label: '抖音' },
    TB: { query: 'tb', label: '淘宝' },
    TAOBAO: { query: 'tb', label: '淘宝' },
    XHS: { query: 'xhs', label: '小红书' },
    PDD: { query: 'pdd', label: '拼多多' },
    KSXD: { query: 'ks', label: '快手' },
    DFHAND: { query: 'dfhand', label: '手工订单' },
  }

  const IS_TOP = window === window.top
  const HOST = location.hostname.toLowerCase()
  const IS_DF_SHELL = HOST === 'df.kdzs.com'
  const IS_PLATFORM_FRAME = /^(dydf|tbdf|xhsdf|pdddf|ksdf|hand)\.kdzs\.com$/i.test(HOST)

  function normalizePlatform(code) {
    const p = String(code || '').trim().toUpperCase()
    if (p === 'DY') return 'FXG'
    if (p === 'HAND' || p === 'MANUAL') return 'DFHAND'
    return p || 'FXG'
  }

  function hostForPlatform(code) {
    return PLATFORM_HOST[normalizePlatform(code)] || PLATFORM_HOST.FXG
  }

  function platformUi(code) {
    const p = normalizePlatform(code)
    return PLATFORM_UI[p] || PLATFORM_UI.FXG
  }

  function dfBatchPrintUrl(platform) {
    const q = platformUi(platform).query
    return `https://df.kdzs.com/#/batchPrint?platform=${q}`
  }

  function currentDfPlatformQuery() {
    const m = String(location.hash || '').match(/[?&]platform=([^&]+)/i)
    return (m?.[1] || '').toLowerCase()
  }

  function isDfBatchPrintRoute() {
    return /batchPrint/i.test(location.hash || '')
  }

  /** 顶栏电商平台：抖音 / 淘宝 / 手工订单 … */
  async function clickPlatformTab(label) {
    const want = String(label || '').trim()
    if (!want) return false
    const nodes = [...document.querySelectorAll('div, span')]
    const candidates = nodes.filter((n) => {
      if (!visible(n)) return false
      return textOf(n) === want
    })
    if (!candidates.length) {
      log(`未找到平台入口「${want}」`, 'error')
      return false
    }
    // 优先顶栏平台条（实测 class 含 rp3kVd8Oxl0zNW7I4eCm）
    const el =
      candidates.find((n) => /rp3kVd8Oxl0zNW7I4eCm/.test(String(n.className || ''))) ||
      candidates.find((n) => n.children.length === 0) ||
      candidates[0]
    const cls = String(el.className || '')
    if (/pcjqLrA4Lw3fHkcLPgCU/.test(cls)) {
      log(`平台已是「${want}」`)
      return true
    }
    log(`切换电商平台 → ${want}`)
    clickEl(el)
    await sleep(1200)
    return true
  }

  /**
   * 门户外壳：进入「打单发货」并切到目标电商平台。
   * @returns {'shell-ready'|'navigating'|'not-shell'}
   */
  async function ensureDfShellBatchPrint(payload) {
    if (!IS_DF_SHELL) return 'not-shell'
    const ui = platformUi(payload?.platform)
    const needQ = ui.query
    const onBatch = isDfBatchPrintRoute()
    const curQ = currentDfPlatformQuery()
    if (!onBatch || curQ !== needQ) {
      log(`进入打单发货 · 平台 ${ui.label}（${needQ}）…`)
      location.hash = `#/batchPrint?platform=${needQ}`
      await sleep(800)
    }
    await clickPlatformTab(ui.label)
    // 等平台 iframe 挂载批打页
    for (let i = 0; i < 20; i++) {
      const frame = [...document.querySelectorAll('iframe')].find((f) => {
        try {
          const h = new URL(f.src || '').hostname
          return h === hostForPlatform(payload?.platform) && f.offsetHeight > 80
        } catch {
          return false
        }
      })
      if (frame) {
        log(`已就绪：${ui.label} 批打 iframe`)
        return 'shell-ready'
      }
      await sleep(400)
    }
    log('平台 iframe 加载较慢，将继续等待批打页脚本…', 'error')
    return 'shell-ready'
  }

  function hostMatchesTask(platform) {
    return HOST === hostForPlatform(platform)
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
    const btn = findMainQueryButton()
    if (btn) {
      clickEl(btn.closest('button') || btn)
      await sleep(1400)
    } else {
      const searchBtns = findButtonsByText(['查询', '搜索'])
      const fallback = searchBtns.find((b) => /查\s*询|搜索/.test(textOf(b))) || searchBtns[0]
      if (fallback) {
        clickEl(fallback.closest('button') || fallback)
        await sleep(1400)
      } else {
        await sleep(600)
      }
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
      const fromDay = toYmd(handoff.orderTimeFrom)
      const toDay = toYmd(handoff.orderTimeTo)
      if (fromDay && toDay) return { fromYmd: fromDay, toYmd: toDay, source: '云端任务' }
    }
    const times = []
    for (const o of handoff?.orders || []) {
      const y = toYmd(o.payTime || o.orderedAt)
      if (y) times.push(y)
    }
    if (!times.length) return null
    times.sort()
    return { fromYmd: times[0], toYmd: times[times.length - 1], source: '任务订单字段' }
  }

  function extractCreateTimeYmd(row) {
    if (!row) return ''
    const el =
      row.querySelector('[class*="col-resize-createTime"]') ||
      row.querySelector('[class*="createTime"]')
    const t = textOf(el) || ''
    const m = t.match(/(\d{4}-\d{2}-\d{2})/)
    return m ? m[1] : ''
  }

  async function waitForPrintBatchReady() {
    log('等待批打页加载完成…')
    for (let i = 0; i < 40; i++) {
      const panel = document.querySelector('.range-picker-panel')
      const begin = panel?.getAttribute('data-begin-date')
      const wrap = document.querySelector('.kdzs-design-range-picker-wrapper')
      const typeSelect = wrap?.parentElement?.querySelector('.ant-select')
      if (panel && begin && wrap && typeSelect) {
        // 默认历史时间范围渲染后再动手，避免点空
        await sleep(1000)
        log(`页面已就绪，当前时间范围 ${begin.slice(0, 10)} ~ ${(panel.getAttribute('data-end-date') || '').slice(0, 10)}`)
        return true
      }
      await sleep(400)
    }
    log('批打页加载超时，继续尝试', 'error')
    return false
  }

  /** 时间类型下拉：下单时间 / 打印时间 / 发货时间 —— 必须先选「下单时间」 */
  async function ensureOrderTimeType() {
    const wrap = document.querySelector('.kdzs-design-range-picker-wrapper')?.parentElement
    const typeSelect = wrap?.querySelector('.ant-select')
    if (!typeSelect) {
      log('未找到时间类型下拉', 'error')
      return false
    }
    const cur = textOf(typeSelect.querySelector('.ant-select-selection-item') || typeSelect)
    if (cur.includes('下单时间')) {
      log('时间类型已是「下单时间」')
      return true
    }
    log(`当前时间类型「${cur || '?'}」，切换为「下单时间」`)
    const selector = typeSelect.querySelector('.ant-select-selector') || typeSelect
    clickEl(selector)
    await sleep(450)
    let opt = [...document.querySelectorAll('.ant-select-item-option')].find(
      (el) => textOf(el).replace(/\s/g, '') === '下单时间',
    )
    if (!opt) {
      // 再点一次
      clickEl(selector)
      await sleep(450)
      opt = [...document.querySelectorAll('.ant-select-item-option')].find(
        (el) => textOf(el).replace(/\s/g, '') === '下单时间',
      )
    }
    if (!opt) {
      log('下拉中未找到「下单时间」选项', 'error')
      return false
    }
    clickEl(opt)
    await sleep(350)
    const after = textOf(typeSelect.querySelector('.ant-select-selection-item') || typeSelect)
    if (!after.includes('下单时间')) {
      log(`切换后仍为「${after}」`, 'error')
      return false
    }
    log('已选择时间类型：下单时间')
    return true
  }

  async function ensurePickerMonth(picker, year, month) {
    for (let i = 0; i < 36; i++) {
      const label = textOf(picker.querySelector('.picker-range-current-month'))
      const m = label.match(/(\d{4})\s*年\s*(\d{1,2})\s*月/)
      if (!m) return false
      const cy = Number(m[1])
      const cm = Number(m[2])
      if (cy === year && cm === month) return true
      const diff = (year - cy) * 12 + (month - cm)
      // 必须点这两个节点；泛搜「«」「»」容易点歪导致翻月失败
      const btn =
        diff < 0
          ? picker.querySelector('.picker-range-last-month')
          : picker.querySelector('.picker-range-next-month')
      if (!btn) return false
      btn.click()
      await sleep(280)
    }
    return false
  }

  async function pickDayInPicker(picker, ymd) {
    if (!picker || !ymd) return false
    const [y, m] = ymd.split('-').map(Number)
    const okMonth = await ensurePickerMonth(picker, y, m)
    if (!okMonth) {
      log(`未能切到 ${y}年${m}月`, 'error')
      return false
    }
    const cell = picker.querySelector(`.day-cell[title="${ymd}"]`)
    if (!cell || cell.classList.contains('disabled')) {
      log(`日期不可选：${ymd}`, 'error')
      return false
    }
    cell.click()
    await sleep(250)
    return true
  }

  /** 设置下单时间（优先云端任务时间；无则先用单号探一次列表读「下单时间」列） */
  async function setOrderTimeRange() {
    let range = resolveOrderTimeRange()
    if (!range) {
      log('任务未带付款/下单时间，尝试从列表探测…')
      const orders = handoff?.orders || []
      const probe = orders[0]
      if (probe) {
        const keys = preferSearchKeys(probe)
        for (const k of keys) {
          await queryByOrderNo(k)
          let hit = findRowContaining(k)
          if (!hit && listPackageItems().length === 1) hit = listPackageItems()[0]
          if (hit) {
            const y = extractCreateTimeYmd(hit)
            if (y) {
              range = { fromYmd: y, toYmd: y, source: '列表下单时间列' }
              log(`探测到下单时间 ${y}`)
              break
            }
          }
        }
      }
    }
    if (!range?.fromYmd || !range?.toYmd) {
      log('无法确定下单时间，将按页面当前时间范围继续', 'error')
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
      log(`下单时间已是 ${range.fromYmd} ~ ${range.toYmd}，仍将点查询刷新列表`)
      return true
    }

    log(`设置下单时间：${range.fromYmd} ~ ${range.toYmd}${range.source ? `（${range.source}）` : ''}`)
    await ensureOrderTimeType()

    const panelEl = document.querySelector('.range-picker-panel')
    try {
      panelEl?.scrollIntoView({ block: 'center' })
    } catch {
      /* ignore */
    }
    clickEl(panelEl)
    await sleep(700)

    let pop = document.querySelector('.range-picker-popover')
    if (!pop) {
      await sleep(500)
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

    // 起、止月历都要翻到目标月再点同一天（否则只改到结束日）
    if (!(await pickDayInPicker(pickers[0], range.fromYmd))) return false
    if (!(await pickDayInPicker(pickers[1], range.toYmd))) return false

    const okBtn =
      pop.querySelector('.submit-btn') ||
      [...pop.querySelectorAll('button, div, span')].find((el) => textOf(el).replace(/\s/g, '') === '确定')
    if (!okBtn) {
      log('未找到时间筛选「确定」', 'error')
      return false
    }
    okBtn.click()
    await sleep(800)

    // 弹层若未关，再点一次确定
    pop = document.querySelector('.range-picker-popover')
    if (pop && visible(pop)) {
      const again =
        pop.querySelector('.submit-btn') ||
        [...pop.querySelectorAll('button')].find((el) => textOf(el).replace(/\s/g, '') === '确定')
      if (again) {
        again.click()
        await sleep(500)
      }
    }

    const begin = (document.querySelector('.range-picker-panel')?.getAttribute('data-begin-date') || '').slice(0, 10)
    const end = (document.querySelector('.range-picker-panel')?.getAttribute('data-end-date') || '').slice(0, 10)
    if (begin === range.fromYmd && end === range.toYmd) {
      log(`下单时间已设置为 ${begin} ~ ${end}`)
      return true
    }
    log(`下单时间设置后为 ${begin || '?'} ~ ${end || '?'}，请人工确认`, 'error')
    return false
  }

  /** 筛选栏里真正的「查询」按钮（避开弹层/其它文案） */
  function findMainQueryButton() {
    const exact = (el) => {
      const t = textOf(el).replace(/\s/g, '')
      return t === '查询' || t === '搜索'
    }
    const nearPicker = []
    const panel = document.querySelector('.range-picker-panel')
    const toolbar =
      panel?.closest('.ant-form, form, [class*="filter"], [class*="Filter"], [class*="search"], [class*="Search"]') ||
      document.querySelector('.kdzs-design-range-picker-wrapper')?.parentElement?.parentElement

    const pool = toolbar
      ? [...toolbar.querySelectorAll('button, .ant-btn, a')]
      : [...document.querySelectorAll('button.ant-btn, button, .ant-btn')]

    for (const el of pool) {
      if (!visible(el) || !exact(el)) continue
      // 排除日期弹层内按钮
      if (el.closest('.range-picker-popover, .ant-picker-dropdown')) continue
      nearPicker.push(el)
    }
    if (nearPicker.length) {
      // 优先主色/实心按钮
      return (
        nearPicker.find((el) => el.classList.contains('ant-btn-primary') || el.className.includes('primary')) ||
        nearPicker[0]
      )
    }

    // 全局兜底：精确文案的 button
    return (
      [...document.querySelectorAll('button, .ant-btn')].find(
        (el) => visible(el) && exact(el) && !el.closest('.range-picker-popover, .ant-picker-dropdown'),
      ) || null
    )
  }

  /** 点击「查询」使时间范围生效 */
  async function clickQuery(reason = '应用时间范围') {
    // 等日期弹层完全收起，避免点到遮罩/错误节点
    for (let i = 0; i < 8; i++) {
      const pop = document.querySelector('.range-picker-popover')
      if (!pop || !visible(pop)) break
      await sleep(200)
    }

    let btn = findMainQueryButton()
    if (!btn) {
      const searchBtns = findButtonsByText(['查询', '搜索']).filter(
        (el) => !el.closest('.range-picker-popover, .ant-picker-dropdown'),
      )
      btn =
        searchBtns.find((b) => {
          const t = textOf(b).replace(/\s/g, '')
          return t === '查询' || t === '搜索'
        }) || searchBtns.find((b) => b.tagName === 'BUTTON') || searchBtns[0]
    }
    if (!btn) {
      log('未找到「查询」按钮', 'error')
      return false
    }
    log(`点击查询（${reason}）`)
    try {
      btn.scrollIntoView({ block: 'center', inline: 'nearest' })
    } catch {
      /* ignore */
    }
    // 优先点到真正的 button
    const realBtn = btn.closest('button') || btn
    clickEl(realBtn)
    await sleep(1600)
    return true
  }

  async function searchAndSelectOrders(opts = {}) {
    const preferListFirst = !!opts.preferListFirst
    const orders = handoff?.orders || []
    if (!orders.length) {
      log('无订单信息，跳过选单', 'error')
      return 0
    }

    await uncheckAllOrders()

    let selected = 0
    for (const order of orders) {
      const keys = preferSearchKeys(order)
      if (!keys.length) continue
      const label = order.orderNo || keys[0]
      log(`查找订单 ${keys.join(' / ')}`)

      let hit = null
      // 时间筛选查询后列表里通常已有目标单：先直接找，避免再点查询刷新
      if (preferListFirst) {
        for (const k of keys) {
          hit = findRowContaining(k)
          if (hit) break
        }
        if (!hit && listPackageItems().length === 1) {
          hit = listPackageItems()[0]
        }
        if (hit) {
          log(`列表中已找到：${label}（跳过单号查询）`)
        }
      }

      if (!hit) {
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
      log('批打页无「选择发货」，继续打印快递单')
      return false
    }
    log(`点击：${textOf(btn)}`)
    clickEl(btn)
    await sleep(1200)
    return true
  }

  function findVisibleExact(texts) {
    const want = texts.map((t) => String(t).replace(/\s/g, ''))
    return [...document.querySelectorAll('button, a, span, div, label')].find((el) => {
      if (!visible(el)) return false
      // 不要误点页脚绿色大按钮
      if (el.matches?.('button.bg-green') || /btn-big-bold.*bg-green|bg-green.*btn-big-bold/.test(String(el.className))) {
        return false
      }
      const t = textOf(el).replace(/\s/g, '')
      return want.includes(t)
    })
  }

  /** 按配置名选中打印机（完整名优先，其次包含匹配） */
  function selectConfiguredPrinter() {
    const want = String(handoff?.printerName || '').trim()
    if (!want) {
      log('未配置打印机名称，使用弹窗当前默认打印机')
      return false
    }
    const wantNorm = want.replace(/\s+/g, ' ').trim().toLowerCase()
    const nodes = [...document.querySelectorAll('label, .ant-radio-wrapper, li, div, span')]
    const candidates = nodes.filter((el) => {
      if (!visible(el)) return false
      const t = textOf(el).replace(/\s+/g, ' ').trim()
      if (!t || t.length > 120) return false
      // 排除整块弹窗容器
      if (t.includes('选择打印机') && t.includes('打印快递单')) return false
      return true
    })

    let best = null
    let bestScore = 0
    for (const el of candidates) {
      const t = textOf(el).replace(/\s+/g, ' ').trim()
      const low = t.toLowerCase()
      let score = 0
      if (low === wantNorm) score = 100
      else if (low.includes(wantNorm)) score = 80 + Math.min(15, wantNorm.length)
      else if (wantNorm.includes(low) && low.length >= 4) score = 50 + low.length
      if (score > bestScore) {
        bestScore = score
        best = el
      }
    }
    if (!best || bestScore < 50) {
      log(`未找到打印机「${want}」，请核对配置名称是否与弹窗一致`, 'error')
      return false
    }
    log(`选择打印机：${textOf(best)}`)
    clickEl(best)
    // 再点一次 radio input 更稳
    const input = best.querySelector?.('input[type="radio"]') || best.closest('label')?.querySelector('input')
    if (input instanceof HTMLElement) clickEl(input)
    return true
  }

  /** 打印机弹窗内的确认：点「打印快递单」（勿再点页脚绿色大按钮） */
  function findPrinterDialogPrintButton() {
    // 弹窗专用 a.print-btn
    const link = document.querySelector('a.print-btn')
    if (link && visible(link)) return link
    // 文案精确匹配，且排除页脚 bg-green
    const nodes = [...document.querySelectorAll('button, a, span')]
    return (
      nodes.find((el) => {
        if (!visible(el)) return false
        if (/bg-green|btn-big-bold/i.test(String(el.className || ''))) return false
        return textOf(el).replace(/\s/g, '') === '打印快递单'
      }) || null
    )
  }

  /** 当前任务订单行 */
  function findTargetOrderRow() {
    const orders = handoff?.orders || []
    for (const order of orders) {
      for (const k of preferSearchKeys(order)) {
        const row = findRowContaining(k)
        if (row) return row
      }
    }
    return null
  }

  /** 行上是否已出现快递单号（打印成功标志） */
  function rowHasExpressNo(row) {
    if (!row) return false
    const t = textOf(row)
    // 常见：快递单号 / 运单号 + 一串数字
    if (/快递单号|运单号/.test(t) && /\d{10,}/.test(t)) return true
    // 部分列表用独立字段展示长单号
    const m = t.match(/(?:快递单号|运单号)[:：\s]*([A-Za-z0-9-]{10,})/)
    return !!m
  }

  /** 处理「已打印过 / 选单号 / 选打印机」等打印弹层 */
  async function resolvePrintDialogs() {
    for (let i = 0; i < 12; i++) {
      const body = textOf(document.body)

      if (body.includes('确认重新打印')) {
        const orig = findVisibleExact(['原单号打印'])
        if (orig) {
          log('检测到已打印订单，选择「原单号打印」')
          clickEl(orig)
          await sleep(1200)
          continue
        }
      }

      if (body.includes('请选择要打印的单号')) {
        const print = findVisibleExact(['打印'])
        if (print) {
          log('确认打印单号 → 打印')
          clickEl(print)
          await sleep(1200)
          continue
        }
      }

      if (body.includes('选择打印机') || document.querySelector('a.print-btn')) {
        selectConfiguredPrinter()
        await sleep(400)
        const confirm = findPrinterDialogPrintButton()
        if (confirm) {
          log('打印机弹窗 → 打印快递单')
          clickEl(confirm)
          await sleep(2000)
          continue
        }
        log('打印机弹窗未找到「打印快递单」', 'error')
      }

      // 弹层已收起
      if (
        !body.includes('确认重新打印') &&
        !body.includes('请选择要打印的单号') &&
        !body.includes('选择打印机') &&
        !document.querySelector('a.print-btn')
      ) {
        return true
      }
      await sleep(500)
    }
    return (
      !textOf(document.body).includes('选择打印机') && !document.querySelector('a.print-btn')
    )
  }

  async function clickPrintExpress() {
    // 页脚只点这一次「打印快递单」
    const btn = [...document.querySelectorAll('button')].find(
      (b) =>
        visible(b) &&
        /bg-green/i.test(String(b.className)) &&
        textOf(b).replace(/\s/g, '') === '打印快递单',
    )
    if (!btn) {
      log('未找到页脚「打印快递单」按钮', 'error')
      return false
    }
    log('点击页脚「打印快递单」')
    clickEl(btn)
    await sleep(1000)
    const dialogOk = await resolvePrintDialogs()
    if (!dialogOk) {
      log('打印弹层未消失，请人工确认', 'error')
      return false
    }
    log('打印弹窗已关闭，等待订单显示快递单号…')

    // 等行上出现快递单号
    for (let i = 0; i < 20; i++) {
      const row = findTargetOrderRow()
      if (rowHasExpressNo(row)) {
        const t = textOf(row)
        const m = t.match(/(?:快递单号|运单号)[:：\s]*([A-Za-z0-9-]{10,})/) || t.match(/\b(\d{12,})\b/)
        log(`订单已显示快递单号${m ? `：${m[1]}` : ''}`)
        return true
      }
      await sleep(500)
    }
    // 弹窗已关但单号字段文案可能不同：若行仍选中则继续发货，仅警告
    const row = findTargetOrderRow()
    if (row && /pack_selected/.test(row.className || '')) {
      log('未识别到快递单号文案，弹窗已关且订单仍选中，继续发货', 'error')
      return true
    }
    log('打印后未看到快递单号，中止自动发货', 'error')
    return false
  }

  async function clickShip() {
    const btn = [...document.querySelectorAll('button')].find(
      (b) => visible(b) && textOf(b).replace(/\s/g, '') === '发货',
    )
    if (!btn) {
      log('未找到「发货」按钮', 'error')
      return false
    }
    log('点击「发货」')
    clickEl(btn)
    await sleep(1200)

    for (let i = 0; i < 6; i++) {
      const body = textOf(document.body)
      if (body.includes('请设置发货方式') || body.includes('发货方式')) {
        const normal = findVisibleExact(['普通发货'])
        if (normal) {
          log('发货方式：普通发货')
          clickEl(normal)
          await sleep(300)
        }
        // 对话框右下「确定」
        const oks = [...document.querySelectorAll('button')].filter(
          (b) => visible(b) && textOf(b).replace(/\s/g, '') === '确定',
        )
        const okBtn =
          oks.find((b) => b.closest('.ant-modal, .ant-modal-root, [class*=modal], [class*=Modal]')) ||
          oks[oks.length - 1]
        if (okBtn) {
          log('确认发货')
          clickEl(okBtn)
          await sleep(2500)
        }
      }
      if (!textOf(document.body).includes('请设置发货方式')) break
      await sleep(500)
    }

    const still = textOf(document.body).includes('请设置发货方式')
    if (still) {
      log('发货确认弹层仍在，请人工确认', 'error')
      return false
    }
    log('发货流程已提交')
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

  function clearFinishedHandoff(reason) {
    handoff = null
    chrome.runtime.sendMessage({ type: 'KDZS_HELPER_CLEAR_HANDOFF' }, () => {
      log(reason || '已清除已完成任务')
      renderPanel()
    })
  }

  async function runAutomation() {
    if (running) return
    running = true
    renderPanel()
    try {
      if (!handoff) {
        log('没有待执行任务。请先绑定插件并由手机发送打单任务，或从发货中心打开快递助手。', 'error')
        return
      }
      const age = Date.now() - Number(handoff.createdAt || handoff.savedAt || 0)
      if (age > HANDOFF_MAX_AGE_MS) {
        log('任务已过期（>30 分钟），请回发货中心重新打开', 'error')
        return
      }

      log('开始自动化（不会自动打印）…')

      // 门户外壳只负责进「打单发货」+ 切平台；勾选在平台 iframe 内执行
      if (IS_DF_SHELL) {
        await ensureDfShellBatchPrint(handoff)
        log('已切换打单发货与电商平台，批打页将自动勾选订单…')
        return
      }

      // 错误平台的旧 iframe 直接忽略，等正确 iframe 加载
      if (IS_PLATFORM_FRAME && !hostMatchesTask(handoff.platform)) {
        log(`当前 iframe 为 ${HOST}，任务需要 ${hostForPlatform(handoff.platform)}，跳过`)
        return
      }

      // 若误开到子站但非批打路由，尽量回到门户走标准路径
      if (IS_TOP && IS_PLATFORM_FRAME && !/printBatch|batchPrint|newIndex/i.test(location.href)) {
        log('不在批打页，改走快递助手门户打单发货…')
        location.href = dfBatchPrintUrl(handoff.platform)
        return
      }

      await sleep(500)
      await waitForPrintBatchReady()

      // 下单时间 → 点一次查询 → 在结果里勾选（列表已有则不再按单号查，避免二次刷新）
      const n = await (async () => {
        await setOrderTimeRange()
        await clickQuery('应用时间范围')
        return searchAndSelectOrders({ preferListFirst: true })
      })()
      log(`订单勾选结果：${n}/${(handoff.orders || []).length}`)
      await selectTemplate()
      await clickSelectShip()

      if (n <= 0) {
        log('未勾选到订单，任务记为失败。请确认平台已切换正确且订单在时间筛选内。', 'error')
        if (handoff?.cloudTaskId) {
          chrome.runtime.sendMessage(
            {
              type: 'KDZS_PRINT_REPORT_TASK',
              taskId: handoff.cloudTaskId,
              status: 'failed',
              errorMessage: '未在批打列表中找到订单（请检查平台切换与时间筛选）',
            },
            () => {
              /* ignore */
            },
          )
        }
        return
      }

      const doPrint = handoff.autoPrint !== false
      if (doPrint) {
        const printed = await clickPrintExpress()
        if (!printed) {
          if (handoff?.cloudTaskId) {
            chrome.runtime.sendMessage(
              {
                type: 'KDZS_PRINT_REPORT_TASK',
                taskId: handoff.cloudTaskId,
                status: 'failed',
                errorMessage: '打印快递单未完成',
              },
              () => {
                /* ignore */
              },
            )
          }
          return
        }
        const shipped = await clickShip()
        if (!shipped) {
          if (handoff?.cloudTaskId) {
            chrome.runtime.sendMessage(
              {
                type: 'KDZS_PRINT_REPORT_TASK',
                taskId: handoff.cloudTaskId,
                status: 'failed',
                errorMessage: '发货未完成（面单可能已打印）',
              },
              () => {
                /* ignore */
              },
            )
          }
          return
        }
        log('自动化完成：已打印快递单并提交发货。请回发货中心核对同步结果。')
      } else {
        log('自动化完成：已勾选订单（autoPrint=false，未自动打印/发货）。')
      }

      const doneTaskId = handoff?.cloudTaskId
      if (doneTaskId) {
        chrome.runtime.sendMessage(
          {
            type: 'KDZS_PRINT_REPORT_TASK',
            taskId: doneTaskId,
            status: 'done',
          },
          () => {
            /* ignore */
          },
        )
      }
      // 清除本地任务，避免刷新/重进批打页再次执行
      clearFinishedHandoff('已清除已完成任务')
    } catch (e) {
      log(`自动化异常：${e?.message || e}`, 'error')
      if (handoff?.cloudTaskId) {
        chrome.runtime.sendMessage(
          {
            type: 'KDZS_PRINT_REPORT_TASK',
            taskId: handoff.cloudTaskId,
            status: 'failed',
            errorMessage: String(e?.message || e),
          },
          () => {
            /* ignore */
          },
        )
      }
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
    log(`已加载任务（${source}）：${(handoff.orders || []).length} 单 · 平台 ${platformUi(handoff.platform).label}`)
    if (!manual && !running) void runAutomation()
  }

  function loadHandoff(manual = false) {
    return (async () => {
      // 云端 token 仅顶层消费，避免 iframe 抢先
      if (IS_TOP) {
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
        // 错误平台的旧 iframe 不抢跑
        if (IS_PLATFORM_FRAME && !hostMatchesTask(local.platform)) {
          return false
        }
        applyHandoff(local, '扩展存储', manual)
        return true
      }

      handoff = null
      renderPanel()
      if (manual) {
        log('无任务：请保持插件绑定在线，由手机发送打单；或从发货中心点「打开快递助手」')
      }
      return false
    })()
  }

  // 门户顶层 + 平台批打 iframe 都注入；其它无关 frame 退出
  if (!IS_TOP && !IS_PLATFORM_FRAME) return

  const href = location.href
  const likelyPrint =
    IS_DF_SHELL ||
    IS_PLATFORM_FRAME ||
    /printBatch|batchPrint|print|newIndex/i.test(href)

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
      if (!next) {
        handoff = null
        renderPanel()
        return
      }
      if (handoff && handoff.createdAt === next.createdAt && handoff.cloudTaskId === next.cloudTaskId) return
      if (IS_PLATFORM_FRAME && !hostMatchesTask(next.platform)) return
      applyHandoff(next, '队列任务', false)
    })

    chrome.runtime.onMessage.addListener((msg) => {
      if (msg?.type === 'KDZS_HELPER_QUEUE_TASK' && msg.payload) {
        if (IS_PLATFORM_FRAME && !hostMatchesTask(msg.payload.platform)) return
        applyHandoff(msg.payload, '队列推送', false)
      }
    })
  }
})()
