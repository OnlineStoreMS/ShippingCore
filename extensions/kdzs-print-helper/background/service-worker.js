/** OSMS 快递助手打单助手 — 绑定设备 + 心跳 + 领任务 */
const STORAGE = {
  handoff: 'kdzsHandoff',
  device: 'kdzsPrintDevice',
  apiBase: 'kdzsPrintApiBase',
}

const DEFAULT_API_BASE = 'https://osms.zfcycle.com/apps/shipping/api/v1'
const HEARTBEAT_ALARM = 'kdzs-print-heartbeat'
const CLAIM_ALARM = 'kdzs-print-claim'

async function getApiBase() {
  const data = await chrome.storage.local.get([STORAGE.apiBase])
  return String(data[STORAGE.apiBase] || DEFAULT_API_BASE).replace(/\/$/, '')
}

async function getDevice() {
  const data = await chrome.storage.local.get([STORAGE.device])
  return data[STORAGE.device] || null
}

async function setDevice(dev) {
  await chrome.storage.local.set({ [STORAGE.device]: dev })
}

async function clearDevice() {
  await chrome.storage.local.remove([STORAGE.device])
}

function deviceHeaders(dev) {
  return {
    'Content-Type': 'application/json',
    'X-KDZS-Device-Key': String(dev.deviceKey || ''),
    'X-KDZS-Device-Secret': String(dev.deviceSecret || ''),
  }
}

async function apiJSON(path, opts = {}) {
  const base = await getApiBase()
  const url = `${base}${path.startsWith('/') ? path : `/${path}`}`
  const res = await fetch(url, {
    method: opts.method || 'GET',
    headers: opts.headers || { 'Content-Type': 'application/json' },
    body: opts.body ? JSON.stringify(opts.body) : undefined,
    credentials: 'omit',
  })
  let body = null
  try {
    body = await res.json()
  } catch {
    body = null
  }
  if (!res.ok || !body || body.code !== 200) {
    const err = new Error((body && body.message) || `HTTP ${res.status}`)
    err.status = res.status
    err.body = body
    throw err
  }
  return body.data
}

async function heartbeat() {
  const dev = await getDevice()
  if (!dev?.deviceKey || !dev?.deviceSecret) return { ok: false, reason: 'unpaired' }
  try {
    const dto = await apiJSON('/mobile/kdzs-print/heartbeat', {
      method: 'POST',
      headers: deviceHeaders(dev),
      body: {},
    })
    return { ok: true, device: dto }
  } catch (e) {
    return { ok: false, reason: e.message || String(e) }
  }
}

async function claimAndDispatch() {
  const dev = await getDevice()
  if (!dev?.deviceKey || !dev?.deviceSecret) return { ok: false, reason: 'unpaired' }
  try {
    const data = await apiJSON('/mobile/kdzs-print/tasks/claim', {
      method: 'POST',
      headers: deviceHeaders(dev),
      body: {},
    })
    const task = data?.task
    if (!task) return { ok: true, task: null }

    let payload = task.payload
    if (typeof payload === 'string') {
      try {
        payload = JSON.parse(payload)
      } catch {
        payload = {}
      }
    }
    const handoff = {
      ...payload,
      savedAt: Date.now(),
      cloudTaskId: task.id,
      fromQueue: true,
    }
    await chrome.storage.local.set({ [STORAGE.handoff]: handoff })

    // 通知已打开的快递助手页执行
    const tabs = await chrome.tabs.query({ url: ['https://*.kdzs.com/*', 'http://*.kdzs.com/*'] })
    for (const tab of tabs) {
      if (!tab.id) continue
      try {
        await chrome.tabs.sendMessage(tab.id, { type: 'KDZS_HELPER_QUEUE_TASK', payload: handoff })
      } catch {
        /* tab 可能无 content script */
      }
    }
    return { ok: true, task }
  } catch (e) {
    return { ok: false, reason: e.message || String(e) }
  }
}

async function reportTask(taskId, status, errorMessage) {
  const dev = await getDevice()
  if (!dev?.deviceKey || !dev?.deviceSecret) throw new Error('未绑定')
  return apiJSON(`/mobile/kdzs-print/tasks/${taskId}/report`, {
    method: 'POST',
    headers: deviceHeaders(dev),
    body: { status, errorMessage: errorMessage || '' },
  })
}

async function completePair(pairCode, deviceName) {
  const data = await apiJSON('/mobile/kdzs-print/pair', {
    method: 'POST',
    body: {
      pairCode: String(pairCode || '').trim(),
      deviceName: String(deviceName || '').trim() || '打单电脑',
    },
  })
  const dev = {
    deviceId: data.deviceId,
    deviceKey: data.deviceKey,
    deviceSecret: data.deviceSecret,
    name: data.name,
    tenantId: data.tenantId,
    pairedAt: Date.now(),
  }
  await setDevice(dev)
  await ensureAlarms()
  await heartbeat()
  return dev
}

async function ensureAlarms() {
  // Chrome 正式环境闹钟最短约 1 分钟；开发者模式可更短
  await chrome.alarms.create(HEARTBEAT_ALARM, { periodInMinutes: 1 })
  await chrome.alarms.create(CLAIM_ALARM, { periodInMinutes: 1 })
}

chrome.runtime.onInstalled.addListener(() => {
  void ensureAlarms()
})

chrome.runtime.onStartup.addListener(() => {
  void ensureAlarms()
})

chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === HEARTBEAT_ALARM) {
    void heartbeat().then(() => claimAndDispatch())
  }
  if (alarm.name === CLAIM_ALARM) {
    void claimAndDispatch()
  }
})

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (!msg || typeof msg !== 'object') return

  if (msg.type === 'KDZS_HELPER_PING') {
    sendResponse({ ok: true, version: chrome.runtime.getManifest().version })
    return true
  }

  if (msg.type === 'KDZS_PRINT_GET_STATUS') {
    Promise.all([getDevice(), getApiBase(), heartbeat()])
      .then(([dev, apiBase, hb]) => {
        sendResponse({
          ok: true,
          version: chrome.runtime.getManifest().version,
          device: dev
            ? { deviceId: dev.deviceId, deviceKey: dev.deviceKey, name: dev.name, pairedAt: dev.pairedAt }
            : null,
          apiBase,
          online: !!hb.ok,
          heartbeatError: hb.ok ? '' : hb.reason || '',
        })
      })
      .catch((e) => sendResponse({ ok: false, error: e.message || String(e) }))
    return true
  }

  if (msg.type === 'KDZS_PRINT_SET_API_BASE') {
    const base = String(msg.apiBase || '').trim().replace(/\/$/, '')
    chrome.storage.local.set({ [STORAGE.apiBase]: base || DEFAULT_API_BASE }, () => {
      sendResponse({ ok: true, apiBase: base || DEFAULT_API_BASE })
    })
    return true
  }

  if (msg.type === 'KDZS_PRINT_PAIR' && msg.pairCode) {
    completePair(msg.pairCode, msg.deviceName)
      .then((dev) => sendResponse({ ok: true, device: { deviceId: dev.deviceId, name: dev.name } }))
      .catch((e) => sendResponse({ ok: false, error: e.message || String(e) }))
    return true
  }

  if (msg.type === 'KDZS_PRINT_UNPAIR') {
    clearDevice().then(() => sendResponse({ ok: true }))
    return true
  }

  if (msg.type === 'KDZS_PRINT_CLAIM_NOW') {
    claimAndDispatch().then((r) => sendResponse(r)).catch((e) => sendResponse({ ok: false, reason: e.message }))
    return true
  }

  if (msg.type === 'KDZS_PRINT_REPORT_TASK' && msg.taskId) {
    reportTask(msg.taskId, msg.status || 'done', msg.errorMessage || '')
      .then((dto) => sendResponse({ ok: true, task: dto }))
      .catch((e) => sendResponse({ ok: false, error: e.message || String(e) }))
    return true
  }

  // —— 兼容旧 handoff ——
  if (msg.type === 'KDZS_HELPER_HANDOFF' && msg.payload) {
    const payload = { ...msg.payload, savedAt: Date.now() }
    chrome.storage.local.set({ [STORAGE.handoff]: payload }, () => {
      sendResponse({ ok: !chrome.runtime.lastError })
    })
    return true
  }

  if (msg.type === 'KDZS_HELPER_HANDOFF_AND_OPEN' && msg.payload && msg.url) {
    const payload = { ...msg.payload, savedAt: Date.now() }
    chrome.storage.local.set({ [STORAGE.handoff]: payload }, () => {
      if (chrome.runtime.lastError) {
        sendResponse({ ok: false, opened: false, error: chrome.runtime.lastError.message })
        return
      }
      chrome.tabs.create({ url: String(msg.url), active: true }, (tab) => {
        sendResponse({ ok: true, opened: !!tab?.id, tabId: tab?.id })
      })
    })
    return true
  }

  if (msg.type === 'KDZS_HELPER_GET_HANDOFF') {
    chrome.storage.local.get([STORAGE.handoff], (data) => {
      sendResponse({ ok: true, payload: data[STORAGE.handoff] || null })
    })
    return true
  }

  if (msg.type === 'KDZS_HELPER_CLEAR_HANDOFF') {
    chrome.storage.local.remove([STORAGE.handoff], () => sendResponse({ ok: true }))
    return true
  }

  if (msg.type === 'KDZS_HELPER_FETCH_CLOUD' && msg.token) {
    const token = String(msg.token).trim()
    getApiBase().then((base) => {
      const url = `${base}/mobile/kdzs-helper-handoff/${encodeURIComponent(token)}`
      fetch(url, { method: 'GET', credentials: 'omit' })
        .then(async (res) => {
          let body = null
          try {
            body = await res.json()
          } catch {
            body = null
          }
          if (!res.ok || !body || body.code !== 200 || !body.data?.payload) {
            sendResponse({
              ok: false,
              error: (body && body.message) || `HTTP ${res.status}`,
              status: res.status,
            })
            return
          }
          const payload = { ...body.data.payload, savedAt: Date.now(), cloudToken: token }
          chrome.storage.local.set({ [STORAGE.handoff]: payload }, () => {
            sendResponse({ ok: true, payload, expireAt: body.data.expireAt || '' })
          })
        })
        .catch((e) => sendResponse({ ok: false, error: e?.message || String(e) }))
    })
    return true
  }
})

void ensureAlarms()
void heartbeat()
