/**
 * 发货中心页面桥：标记扩展已安装，接收页面 postMessage 写入 handoff。
 * 需 all_frames=true：子应用若在 iframe 内也能注入。
 */
;(() => {
  const MARK = 'data-kdzs-helper'
  const SOURCE = 'shippingcore-kdzs-helper'

  function mark() {
    try {
      document.documentElement.setAttribute(MARK, '1')
      document.documentElement.setAttribute('data-kdzs-helper-version', chrome.runtime.getManifest().version)
    } catch {
      /* ignore */
    }
  }

  mark()
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', mark, { once: true })
  }

  window.addEventListener('message', (ev) => {
    const data = ev.data
    if (!data || data.source !== 'shippingcore') return

    if (data.type === 'KDZS_HELPER_PING') {
      chrome.runtime.sendMessage({ type: 'KDZS_HELPER_PING' }, (res) => {
        window.postMessage(
          {
            source: SOURCE,
            type: 'KDZS_HELPER_PONG',
            ok: !chrome.runtime.lastError && !!res?.ok,
            version: res?.version || chrome.runtime.getManifest().version,
          },
          '*',
        )
      })
      return
    }

    if (data.type === 'KDZS_HELPER_HANDOFF') {
      if (!data.payload) return
      chrome.runtime.sendMessage({ type: 'KDZS_HELPER_HANDOFF', payload: data.payload }, (res) => {
        const err = chrome.runtime.lastError
        window.postMessage(
          {
            source: SOURCE,
            type: 'KDZS_HELPER_HANDOFF_ACK',
            ok: !err && !!res?.ok,
            error: err?.message || '',
          },
          '*',
        )
      })
      return
    }

    if (data.type === 'KDZS_HELPER_HANDOFF_AND_OPEN') {
      if (!data.payload || !data.url) return
      chrome.runtime.sendMessage(
        { type: 'KDZS_HELPER_HANDOFF_AND_OPEN', payload: data.payload, url: data.url },
        (res) => {
          const err = chrome.runtime.lastError
          window.postMessage(
            {
              source: SOURCE,
              type: 'KDZS_HELPER_HANDOFF_ACK',
              ok: !err && !!res?.ok,
              error: err?.message || '',
              opened: !!res?.opened,
            },
            '*',
          )
        },
      )
    }
  })
})()
