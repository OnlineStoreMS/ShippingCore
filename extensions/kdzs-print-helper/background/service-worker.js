/** @typedef {import('../shared/types.js').KdzsHandoff} KdzsHandoff */

const STORAGE_KEY = 'kdzsHandoff'

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (!msg || typeof msg !== 'object') return

  if (msg.type === 'KDZS_HELPER_PING') {
    sendResponse({ ok: true, version: chrome.runtime.getManifest().version })
    return true
  }

  if (msg.type === 'KDZS_HELPER_HANDOFF' && msg.payload) {
    const payload = { ...msg.payload, savedAt: Date.now() }
    chrome.storage.local.set({ [STORAGE_KEY]: payload }, () => {
      sendResponse({ ok: true })
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
})
