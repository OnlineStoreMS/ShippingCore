declare global {
  interface Window {
    __RUNTIME_CONFIG__?: {
      portalUrl?: string
    }
  }
}

/** 部署时可由 runtime-config.js + 环境变量 VITE_PORTAL_URL / PUBLIC_HOST 覆盖 */
export function getPortalUrl(): string {
  const configured =
    window.__RUNTIME_CONFIG__?.portalUrl?.trim() ||
    import.meta.env.VITE_PORTAL_URL?.trim()
  if (configured) return configured

  // 热更新覆盖 runtime-config.js 时兜底：用当前访问主机拼门户端口
  const { protocol, hostname } = window.location
  if (hostname && hostname !== 'localhost' && hostname !== '127.0.0.1') {
    return `${protocol}//${hostname}:5174`
  }
  return 'http://localhost:5174'
}
