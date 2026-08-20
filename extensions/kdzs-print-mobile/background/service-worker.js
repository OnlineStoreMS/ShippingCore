/** OSMS 快递助手·手机版 — 配对 + 心跳 + 领任务（不含发货中心电脑 handoff） */
const STORAGE = {
  handoff: 'kdzsHandoff',
  device: 'kdzsPrintDevice',
  apiBase: 'kdzsPrintApiBase',
  pendingPair: 'kdzsPendingPair',
}

const DEFAULT_API_BASE = 'https://osms.zfcycle.com/apps/shipping/api/v1'
const HEARTBEAT_ALARM = 'kdzs-print-heartbeat'
const CLAIM_ALARM = 'kdzs-print-claim'

/** 门户 hash platform= 参数 */
const PLATFORM_QUERY = {
  FXG: 'fxg',
  DY: 'fxg',
  TB: 'tb',
  TAOBAO: 'tb',
  XHS: 'xhs',
  PDD: 'pdd',
  KSXD: 'ks',
  DFHAND: 'dfhand',
  HAND: 'dfhand',
  MANUAL: 'dfhand',
}

function normalizePlatform(code) {
  const p = String(code || '').trim().toUpperCase()
  if (p === 'DY') return 'FXG'
  if (p === 'HAND' || p === 'MANUAL') return 'DFHAND'
  return p || 'FXG'
}

function platformQuery(code) {
  const p = normalizePlatform(code)
  return PLATFORM_QUERY[p] || 'fxg'
}

/** 走快递助手门户：打单发货 + 顶栏切平台（批打在 iframe） */
function dfBatchPrintUrl(platform) {
  return `https://df.kdzs.com/#/batchPrint?platform=${platformQuery(platform)}`
}

function tabIsDfPortal(tab) {
  try {
    return new URL(tab.url || '').hostname === 'df.kdzs.com'
  } catch {
    return false
  }
}

/**
 * 打开/聚焦 df.kdzs.com 打单发货页；由页面脚本切平台并在 iframe 勾选。
 */
async function ensurePlatformPrintTab(platform, handoff) {
  const targetUrl = dfBatchPrintUrl(platform)
  const all = await chrome.tabs.query({ url: ['https://*.kdzs.com/*', 'http://*.kdzs.com/*'] })
  let tab = all.find((t) => tabIsDfPortal(t))

  if (tab?.id) {
    await chrome.tabs.update(tab.id, { url: targetUrl, active: true })
    await waitTabComplete(tab.id, 15000)
  } else {
    tab = await chrome.tabs.create({ url: targetUrl, active: true })
    if (tab?.id) await waitTabComplete(tab.id, 20000)
  }

  if (tab?.id) {
    for (let i = 0; i < 10; i++) {
      try {
        await chrome.tabs.sendMessage(tab.id, { type: 'KDZS_HELPER_QUEUE_TASK', payload: handoff })
        return { ok: true, tabId: tab.id, url: targetUrl }
      } catch {
        await sleep(500)
      }
    }
  }
  return { ok: true, tabId: tab?.id, url: targetUrl, warned: 'content-script-pending' }
}

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms))
}

function waitTabComplete(tabId, timeoutMs) {
  return new Promise((resolve) => {
    let done = false
    const finish = () => {
      if (done) return
      done = true
      chrome.tabs.onUpdated.removeListener(onUpdated)
      resolve()
    }
    const timer = setTimeout(finish, timeoutMs)
    function onUpdated(id, info) {
      if (id === tabId && info.status === 'complete') {
        clearTimeout(timer)
        // SPA hash 路由还需一点时间挂载
        setTimeout(finish, 800)
      }
    }
    chrome.tabs.onUpdated.addListener(onUpdated)
    chrome.tabs.get(tabId, (t) => {
      if (chrome.runtime.lastError) {
        clearTimeout(timer)
        finish()
        return
      }
      if (t?.status === 'complete') {
        clearTimeout(timer)
        setTimeout(finish, 800)
      }
    })
  })
}

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
    const claimed = !!dto?.claimed
    if (claimed && !dev.claimed) {
      await setDevice({ ...dev, claimed: true, name: dto.name || dev.name, tenantId: dto.tenantId })
      await chrome.storage.local.remove([STORAGE.pendingPair])
    }
    return { ok: true, device: dto, claimed }
  } catch (e) {
    return { ok: false, reason: e.message || String(e) }
  }
}

async function claimAndDispatch() {
  const hb = await heartbeat()
  const dev = await getDevice()
  if (!dev?.deviceKey || !dev?.deviceSecret) return { ok: false, reason: 'unpaired' }
  if (!(dev.claimed || hb.claimed)) {
    return { ok: true, task: null, waitingClaim: true }
  }
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

    // 打开快递助手门户打单发货页（顶栏切平台，iframe 内勾选）
    const nav = await ensurePlatformPrintTab(handoff.platform, handoff)
    return { ok: true, task, nav }
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

/** 电脑生成配对码；手机认领后 heartbeat.claimed=true */
async function createPairOffer(deviceName) {
  const data = await apiJSON('/mobile/kdzs-print/pair-sessions', {
    method: 'POST',
    body: {
      deviceName: String(deviceName || '').trim() || '打单电脑',
    },
  })
  const dev = {
    deviceId: data.deviceId,
    deviceKey: data.deviceKey,
    deviceSecret: data.deviceSecret,
    name: data.name,
    claimed: false,
    pairedAt: Date.now(),
  }
  await setDevice(dev)
  await chrome.storage.local.set({
    [STORAGE.pendingPair]: {
      pairCode: data.pairCode,
      expireAt: data.expireAt,
      createdAt: Date.now(),
    },
  })
  await ensureAlarms()
  await heartbeat()
  return { device: dev, pairCode: data.pairCode, expireAt: data.expireAt }
}

async function getPendingPair() {
  const data = await chrome.storage.local.get([STORAGE.pendingPair])
  return data[STORAGE.pendingPair] || null
}

async function ensureAlarms() {
  // Chrome 闹钟最短约 1 分钟；另外用内存定时器加快领任务
  await chrome.alarms.create(HEARTBEAT_ALARM, { periodInMinutes: 1 })
  await chrome.alarms.create(CLAIM_ALARM, { periodInMinutes: 1 })
}

let claimTimer = null
function ensureFastClaimLoop() {
  if (claimTimer) return
  // SW 存活期间约 12s 领一次；被挂起后靠 alarm 唤醒
  claimTimer = setInterval(() => {
    void claimAndDispatch()
  }, 12000)
}

chrome.runtime.onInstalled.addListener(() => {
  void ensureAlarms()
  ensureFastClaimLoop()
})

chrome.runtime.onStartup.addListener(() => {
  void ensureAlarms()
  ensureFastClaimLoop()
})

chrome.alarms.onAlarm.addListener((alarm) => {
  ensureFastClaimLoop()
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
    Promise.all([getDevice(), getApiBase(), getPendingPair(), heartbeat()])
      .then(([dev, apiBase, pending, hb]) => {
        const claimed = !!(dev?.claimed || hb.claimed || hb.device?.claimed)
        sendResponse({
          ok: true,
          version: chrome.runtime.getManifest().version,
          device: dev
            ? {
                deviceId: dev.deviceId,
                deviceKey: dev.deviceKey,
                name: (hb.device && hb.device.name) || dev.name,
                pairedAt: dev.pairedAt,
                claimed,
              }
            : null,
          pendingPair: claimed ? null : pending,
          apiBase,
          online: !!hb.ok && claimed,
          waitingClaim: !!dev && !claimed,
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

  if (msg.type === 'KDZS_PRINT_CREATE_PAIR') {
    createPairOffer(msg.deviceName)
      .then((r) =>
        sendResponse({
          ok: true,
          pairCode: r.pairCode,
          expireAt: r.expireAt,
          device: { deviceId: r.device.deviceId, name: r.device.name },
        }),
      )
      .catch((e) => sendResponse({ ok: false, error: e.message || String(e) }))
    return true
  }

  if (msg.type === 'KDZS_PRINT_UNPAIR') {
    clearDevice()
      .then(() => chrome.storage.local.remove([STORAGE.pendingPair]))
      .then(() => sendResponse({ ok: true }))
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

  // 队列任务读写（由 claimAndDispatch 写入，页面脚本读取）
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
})

void ensureAlarms()
ensureFastClaimLoop()
void heartbeat().then(() => claimAndDispatch())
