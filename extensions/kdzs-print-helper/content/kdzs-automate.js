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

  function findRowContaining(text) {
    const needle = text.toLowerCase()
    const candidates = document.querySelectorAll('tr, .el-table__row, [class*="order"], [class*="row"], li, .ant-table-row')
    for (const row of candidates) {
      if (!visible(row)) continue
      const t = textOf(row).toLowerCase()
      if (t.includes(needle)) return row
    }
    // fallback: any element with the text, climb to a checkbox parent
    const walk = document.createTreeWalker(document.body, NodeFilter.SHOW_ELEMENT)
    let node
    while ((node = walk.nextNode())) {
      if (!(node instanceof HTMLElement) || !visible(node)) continue
      const own = (node.childNodes.length === 1 && node.childNodes[0].nodeType === 3)
        ? textOf(node).toLowerCase()
        : ''
      if (own && own.includes(needle) && own.length < 80) {
        return node.closest('tr, .el-table__row, li, [class*="row"]') || node.parentElement
      }
    }
    return null
  }

  function checkInRow(row) {
    if (!row) return false
    const box =
      row.querySelector('input[type="checkbox"]') ||
      row.querySelector('.el-checkbox__original') ||
      row.querySelector('.el-checkbox') ||
      row.querySelector('[role="checkbox"]')
    if (!box) {
      // click row itself as last resort
      return clickEl(row)
    }
    if (box instanceof HTMLInputElement) {
      if (!box.checked) {
        clickEl(box)
        if (!box.checked) box.checked = true
      }
      return !!box.checked || true
    }
    const aria = box.getAttribute('aria-checked')
    if (aria === 'true') return true
    clickEl(box)
    return true
  }

  async function searchAndSelectOrders() {
    const orders = handoff?.orders || []
    if (!orders.length) {
      log('无订单信息，跳过选单', 'error')
      return 0
    }

    const search =
      findInputByPlaceholder(['订单', '单号', '运单', '搜索', '关键字', '关键词']) ||
      document.querySelector('input[type="search"]')

    let selected = 0
    for (const order of orders) {
      const keys = orderKeywords(order)
      if (!keys.length) continue
      const primary = keys[0]
      log(`查找订单 ${keys.join(' / ')}`)

      if (search) {
        setInputValue(search, primary)
        await sleep(800)
        const searchBtns = findButtonsByText(['搜索', '查询'])
        if (searchBtns[0]) {
          clickEl(searchBtns[0])
          await sleep(1000)
        } else {
          await sleep(600)
        }
      }

      let hit = null
      for (const k of keys) {
        hit = findRowContaining(k)
        if (hit) break
      }
      if (!hit) {
        // 有时列表已含该单，无需搜索
        for (const k of keys) {
          hit = findRowContaining(k)
          if (hit) break
        }
      }
      if (!hit) {
        log(`未在列表中找到：${primary}`, 'error')
        continue
      }
      if (checkInRow(hit)) {
        selected += 1
        log(`已勾选：${primary}`)
      }
      await sleep(300)
    }
    return selected
  }

  async function clickSelectShip() {
    // 优先精确文案
    const exact = findButtonsByText(['选择发货', '选中发货', '确认选择'])
    const btn = exact.find((el) => /选择发货|选中发货/.test(textOf(el))) || exact[0]
    if (!btn) {
      log('未找到「选择发货」按钮（可能已进入明细，或需先勾选订单）', 'error')
      return false
    }
    log(`点击：${textOf(btn)}`)
    clickEl(btn)
    await sleep(1200)
    return true
  }

  async function selectTemplate() {
    const name = String(handoff?.templateName || '').trim()
    if (!name) {
      log('未指定模板名，跳过模板选择')
      return false
    }
    log(`尝试选择模板：${name}`)

    // 打开下拉
    const triggers = [
      ...findButtonsByText(['模板', '快递模板', '选择模板']),
      ...document.querySelectorAll('.el-select, .ant-select, [class*="template"], [class*="Template"]'),
    ]
    for (const t of triggers) {
      if (visible(t)) {
        clickEl(t)
        await sleep(400)
        break
      }
    }

    // 在下拉选项中匹配
    const options = document.querySelectorAll(
      '.el-select-dropdown__item, .ant-select-item, .el-option, li, [role="option"], label, span, div',
    )
    for (const opt of options) {
      if (!visible(opt)) continue
      const t = textOf(opt)
      if (!t || t.length > 80) continue
      if (t.includes(name) || name.includes(t)) {
        clickEl(opt)
        log(`已选择模板选项：${t}`)
        await sleep(400)
        return true
      }
    }

    // 页面上已展示的模板卡片/单选
    const cards = document.querySelectorAll('label, .el-radio, .el-radio-button, [class*="tpl"], [class*="template"]')
    for (const c of cards) {
      if (!visible(c)) continue
      const t = textOf(c)
      if (t.includes(name)) {
        clickEl(c)
        log(`已点击模板卡片：${t.slice(0, 40)}`)
        return true
      }
    }

    log(`未能自动选中模板「${name}」，请手动选择`, 'error')
    return false
  }

  async function selectGoods() {
    const orders = handoff?.orders || []
    const allKeys = []
    for (const o of orders) {
      for (const k of goodsKeywords(o)) {
        if (!allKeys.includes(k)) allKeys.push(k)
      }
    }
    if (!allKeys.length) {
      log('无指定商品明细（将保持页面默认勾选）')
      return 0
    }

    // 若是整单发货且仅 1 个 SKU，通常默认已勾选
    log(`尝试勾选商品：${allKeys.slice(0, 5).join('；')}${allKeys.length > 5 ? '…' : ''}`)

    // 先取消全选再按关键字勾选（仅当找到商品行复选框）
    let matched = 0
    for (const key of allKeys) {
      const row = findRowContaining(key)
      if (!row) continue
      if (checkInRow(row)) {
        matched += 1
        log(`已勾选商品行含：${key.slice(0, 24)}`)
      }
      await sleep(200)
    }
    if (!matched) log('未定位到商品行，请在页面上人工确认勾选', 'error')
    return matched
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

      // hash 到批打页时 SPA 可能晚渲染
      for (let i = 0; i < 15; i++) {
        if (document.body && textOf(document.body).length > 50) break
        await sleep(400)
      }

      const n = await searchAndSelectOrders()
      log(`订单勾选结果：${n}/${(handoff.orders || []).length}`)

      await clickSelectShip()
      await selectTemplate()
      await selectGoods()

      log('自动化完成：请人工确认模板/商品后点击打印。打印后回发货中心「同步单号→确认发货」。')
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
        status.textContent = `任务：${n} 单 · 模板 ${handoff.templateName || '未指定'}${running ? ' · 执行中…' : ''}`
      }
    }
    if (logs) logs.textContent = lastLog.join('\n') || '等待任务…'
    if (runBtn instanceof HTMLButtonElement) runBtn.disabled = running
  }

  function loadHandoff(manual = false) {
    return new Promise((resolve) => {
      chrome.runtime.sendMessage({ type: 'KDZS_HELPER_GET_HANDOFF' }, (res) => {
        if (chrome.runtime.lastError) {
          log(chrome.runtime.lastError.message, 'error')
          resolve(false)
          return
        }
        handoff = res?.payload || null
        renderPanel()
        if (handoff) {
          log(`已加载任务：${(handoff.orders || []).length} 单`)
          if (!manual && !running) {
            // 自动执行一次
            void runAutomation()
          }
        } else if (manual) {
          log('存储中无任务')
        }
        resolve(!!handoff)
      })
    })
  }

  // 仅在批打相关页显示面板；其它页也允许手动读取
  const href = location.href
  const likelyPrint =
    /printBatch|batchPrint|print/i.test(href) || /newIndex|df\.kdzs|kdzs\.com/i.test(href)

  if (likelyPrint) {
    renderPanel()
    // SPA 路由晚到，延迟加载并自动跑
    setTimeout(() => void loadHandoff(false), 1200)
    setTimeout(() => {
      if (!handoff) void loadHandoff(false)
    }, 3500)
  }
})()
