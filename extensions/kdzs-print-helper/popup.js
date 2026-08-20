const statusEl = document.getElementById('status')
const deviceEl = document.getElementById('device')
const taskEl = document.getElementById('task')
const pairBlock = document.getElementById('pairBlock')
const boundBlock = document.getElementById('boundBlock')
const pairShow = document.getElementById('pairShow')
const pairCodeShow = document.getElementById('pairCodeShow')
const pairExpireShow = document.getElementById('pairExpireShow')
const apiBaseEl = document.getElementById('apiBase')

let pollTimer = null

function setStatus(text, cls) {
  statusEl.className = cls || 'warn'
  statusEl.textContent = text
}

function formatExpire(v) {
  if (!v) return ''
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return `有效至 ${v}`
  return `有效至 ${d.toLocaleTimeString()}`
}

function showPendingPair(pending) {
  if (!pending?.pairCode) {
    pairShow.hidden = true
    return
  }
  pairShow.hidden = false
  pairCodeShow.textContent = pending.pairCode
  pairExpireShow.textContent = formatExpire(pending.expireAt)
}

function refresh() {
  chrome.runtime.sendMessage({ type: 'KDZS_PRINT_GET_STATUS' }, (st) => {
    if (chrome.runtime.lastError) {
      setStatus(chrome.runtime.lastError.message, 'bad')
      return
    }
    if (!st?.ok) {
      setStatus(st?.error || '状态读取失败', 'bad')
      return
    }
    apiBaseEl.value = st.apiBase || ''

    if (!st.device) {
      pairBlock.hidden = false
      boundBlock.hidden = true
      showPendingPair(null)
      setStatus(`未绑定 · v${st.version}`, 'warn')
      deviceEl.textContent = '点「生成配对码」，再在手机输入绑定'
      stopPoll()
      return
    }

    if (st.waitingClaim || !st.device.claimed) {
      pairBlock.hidden = false
      boundBlock.hidden = true
      showPendingPair(st.pendingPair)
      setStatus(`等待手机绑定 · v${st.version}`, 'warn')
      deviceEl.textContent = `设备已就绪 · ${st.device.name || '打单电脑'} · 请在手机输入配对码`
      startPoll()
      return
    }

    stopPoll()
    pairBlock.hidden = true
    boundBlock.hidden = false
    showPendingPair(null)
    if (st.online) {
      setStatus(`在线 · ${st.device.name || '打单电脑'} · v${st.version}`, 'ok')
    } else {
      setStatus(`离线 · ${st.device.name || '打单电脑'} · ${st.heartbeatError || '心跳失败'}`, 'bad')
    }
    deviceEl.textContent = `设备 Key：${st.device.deviceKey || '-'} · ID ${st.device.deviceId || '-'}`
  })

  chrome.runtime.sendMessage({ type: 'KDZS_HELPER_GET_HANDOFF' }, (res) => {
    const p = res?.payload
    if (!p) {
      taskEl.textContent = '无任务'
      return
    }
    taskEl.textContent = JSON.stringify(
      {
        cloudTaskId: p.cloudTaskId,
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

function startPoll() {
  if (pollTimer) return
  pollTimer = window.setInterval(() => refresh(), 3000)
}

function stopPoll() {
  if (!pollTimer) return
  window.clearInterval(pollTimer)
  pollTimer = null
}

document.getElementById('createPairBtn').addEventListener('click', () => {
  setStatus('生成配对码中…', 'warn')
  chrome.runtime.sendMessage({ type: 'KDZS_PRINT_CREATE_PAIR', deviceName: '打单电脑' }, (res) => {
    if (!res?.ok) {
      setStatus(res?.error || '生成失败', 'bad')
      return
    }
    showPendingPair({ pairCode: res.pairCode, expireAt: res.expireAt })
    setStatus('请在手机输入配对码', 'warn')
    refresh()
  })
})

document.getElementById('unpairBtn').addEventListener('click', () => {
  chrome.runtime.sendMessage({ type: 'KDZS_PRINT_UNPAIR' }, () => refresh())
})

document.getElementById('refreshBtn').addEventListener('click', () => refresh())

document.getElementById('claimBtn').addEventListener('click', () => {
  setStatus('领取中…', 'warn')
  chrome.runtime.sendMessage({ type: 'KDZS_PRINT_CLAIM_NOW' }, (res) => {
    if (!res?.ok) {
      setStatus(res?.reason || '领取失败', 'bad')
      return
    }
    if (res.waitingClaim) setStatus('仍在等待手机绑定', 'warn')
    else if (!res.task) setStatus('暂无新任务（仍在线）', 'ok')
    else setStatus(`已领取任务 #${res.task.id}`, 'ok')
    refresh()
  })
})

document.getElementById('clear').addEventListener('click', () => {
  chrome.runtime.sendMessage({ type: 'KDZS_HELPER_CLEAR_HANDOFF' }, () => refresh())
})

document.getElementById('saveApi').addEventListener('click', () => {
  chrome.runtime.sendMessage({ type: 'KDZS_PRINT_SET_API_BASE', apiBase: apiBaseEl.value }, () => {
    setStatus('API 已保存', 'ok')
    refresh()
  })
})

window.addEventListener('unload', stopPoll)
refresh()
