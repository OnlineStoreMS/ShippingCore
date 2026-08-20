const statusEl = document.getElementById('status')
const taskEl = document.getElementById('task')
const apiBaseEl = document.getElementById('apiBase')

function setStatus(text, cls) {
  statusEl.className = cls || 'warn'
  statusEl.textContent = text
}

function refresh() {
  chrome.runtime.sendMessage({ type: 'KDZS_HELPER_PING' }, (st) => {
    if (chrome.runtime.lastError) {
      setStatus(chrome.runtime.lastError.message, 'bad')
      return
    }
    setStatus(`已就绪 · 电脑版 v${st?.version || '?'}`, 'ok')
  })

  chrome.storage.local.get(['kdzsPrintApiBase'], (data) => {
    apiBaseEl.value = data.kdzsPrintApiBase || 'https://osms.zfcycle.com/apps/shipping/api/v1'
  })

  chrome.runtime.sendMessage({ type: 'KDZS_HELPER_GET_HANDOFF' }, (res) => {
    const p = res?.payload
    if (!p) {
      taskEl.textContent = '无任务'
      return
    }
    taskEl.textContent = JSON.stringify(
      {
        templateName: p.templateName,
        platform: p.platform,
        orders: (p.orders || []).map((o) => ({
          orderNo: o.orderNo,
          platformSysTid: o.platformSysTid,
          platformOrderId: o.platformOrderId,
          goods: (o.goods || []).length,
        })),
        createdAt: p.createdAt ? new Date(p.createdAt).toLocaleString() : '',
      },
      null,
      2,
    )
  })
}

document.getElementById('clear').addEventListener('click', () => {
  chrome.runtime.sendMessage({ type: 'KDZS_HELPER_CLEAR_HANDOFF' }, () => refresh())
})

document.getElementById('saveApi').addEventListener('click', () => {
  chrome.runtime.sendMessage({ type: 'KDZS_PRINT_SET_API_BASE', apiBase: apiBaseEl.value }, () => {
    setStatus('API 已保存', 'ok')
    refresh()
  })
})

refresh()
