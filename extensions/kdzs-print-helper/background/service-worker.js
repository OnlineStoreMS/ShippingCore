/** OSMS 快递助手·电脑版 — 发货中心 handoff（不含手机配对/远程任务） */
const STORAGE = {
  handoff: 'kdzsHandoff',
  apiBase: 'kdzsPrintApiBase',
}

const DEFAULT_API_BASE = 'https://osms.zfcycle.com/apps/shipping/api/v1'

async function getApiBase() {
  const data = await chrome.storage.local.get([STORAGE.apiBase])
  return String(data[STORAGE.apiBase] || DEFAULT_API_BASE).replace(/\/$/, '')
}

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (!msg || typeof msg !== 'object') return

  if (msg.type === 'KDZS_HELPER_PING') {
    sendResponse({ ok: true, version: chrome.runtime.getManifest().version, edition: 'desktop' })
    return true
  }

  if (msg.type === 'KDZS_PRINT_SET_API_BASE') {
    const base = String(msg.apiBase || '').trim().replace(/\/$/, '')
    chrome.storage.local.set({ [STORAGE.apiBase]: base || DEFAULT_API_BASE }, () => {
      sendResponse({ ok: true, apiBase: base || DEFAULT_API_BASE })
    })
    return true
  }

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
