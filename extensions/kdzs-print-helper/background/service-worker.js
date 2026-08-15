/** @typedef {import('../shared/types.js').KdzsHandoff} KdzsHandoff */

const STORAGE_KEY = 'kdzsHandoff'
/** 生产环境 ShippingCore 公开 API 前缀（经 Caddy /apps/shipping） */
const DEFAULT_API_BASE = 'https://osms.zfcycle.com/apps/shipping/api/v1'

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (!msg || typeof msg !== 'object') return

  if (msg.type === 'KDZS_HELPER_PING') {
    sendResponse({ ok: true, version: chrome.runtime.getManifest().version })
    return true
  }

  if (msg.type === 'KDZS_HELPER_HANDOFF' && msg.payload) {
    const payload = { ...msg.payload, savedAt: Date.now() }
    chrome.storage.local.set({ [STORAGE_KEY]: payload }, () => {
      sendResponse({ ok: !chrome.runtime.lastError })
    })
    return true
  }

  if (msg.type === 'KDZS_HELPER_HANDOFF_AND_OPEN' && msg.payload && msg.url) {
    const payload = { ...msg.payload, savedAt: Date.now() }
    chrome.storage.local.set({ [STORAGE_KEY]: payload }, () => {
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
    chrome.storage.local.get([STORAGE_KEY], (data) => {
      sendResponse({ ok: true, payload: data[STORAGE_KEY] || null })
    })
    return true
  }

  if (msg.type === 'KDZS_HELPER_CLEAR_HANDOFF') {
    chrome.storage.local.remove([STORAGE_KEY], () => sendResponse({ ok: true }))
    return true
  }

  /** 从 ShippingCore 云端拉取打单任务（无需发货中心页桥） */
  if (msg.type === 'KDZS_HELPER_FETCH_CLOUD' && msg.token) {
    const token = String(msg.token).trim()
    const base = String(msg.apiBase || DEFAULT_API_BASE).replace(/\/$/, '')
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
        chrome.storage.local.set({ [STORAGE_KEY]: payload }, () => {
          sendResponse({ ok: true, payload, expireAt: body.data.expireAt || '' })
        })
      })
      .catch((e) => {
        sendResponse({ ok: false, error: e?.message || String(e) })
      })
    return true
  }
})
