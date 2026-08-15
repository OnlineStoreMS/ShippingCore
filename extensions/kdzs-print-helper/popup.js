const statusEl = document.getElementById('status')
const taskEl = document.getElementById('task')

function refresh() {
  chrome.runtime.sendMessage({ type: 'KDZS_HELPER_GET_HANDOFF' }, (res) => {
    if (chrome.runtime.lastError) {
      statusEl.className = 'warn'
      statusEl.textContent = chrome.runtime.lastError.message
      return
    }
    const p = res?.payload
    if (!p) {
      statusEl.className = 'warn'
      statusEl.textContent = '当前无任务'
      taskEl.textContent = '无任务'
      return
    }
    statusEl.className = 'ok'
    statusEl.textContent = `已就绪 · v${chrome.runtime.getManifest().version}`
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

refresh()
