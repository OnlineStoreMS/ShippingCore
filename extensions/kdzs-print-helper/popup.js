const statusEl = document.getElementById('status')
const deviceEl = document.getElementById('device')
const taskEl = document.getElementById('task')
const pairBlock = document.getElementById('pairBlock')
const boundBlock = document.getElementById('boundBlock')
const apiBaseEl = document.getElementById('apiBase')

function setStatus(text, cls) {
  statusEl.className = cls || 'warn'
  statusEl.textContent = text
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
      setStatus(`未绑定 · v${st.version}`, 'warn')
      deviceEl.textContent = '请先在手机 OpsMobile 生成配对码并在此绑定'
    } else {
      pairBlock.hidden = true
      boundBlock.hidden = false
      if (st.online) {
        setStatus(`在线 · ${st.device.name || '打单电脑'} · v${st.version}`, 'ok')
      } else {
        setStatus(`离线 · ${st.device.name || '打单电脑'} · ${st.heartbeatError || '心跳失败'}`, 'bad')
      }
      deviceEl.textContent = `设备 Key：${st.device.deviceKey || '-'} · ID ${st.device.deviceId || '-'}`
    }
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

document.getElementById('pairBtn').addEventListener('click', () => {
  const pairCode = document.getElementById('pairCode').value.trim()
  if (!pairCode) {
    setStatus('请输入配对码', 'warn')
    return
  }
  setStatus('绑定中…', 'warn')
  chrome.runtime.sendMessage({ type: 'KDZS_PRINT_PAIR', pairCode, deviceName: '打单电脑' }, (res) => {
    if (!res?.ok) {
      setStatus(res?.error || '绑定失败', 'bad')
      return
    }
    setStatus('绑定成功', 'ok')
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
    if (!res.task) setStatus('暂无新任务（仍在线）', 'ok')
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

refresh()
