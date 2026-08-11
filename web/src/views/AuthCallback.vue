<script setup lang="ts">
import { onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { fetchSession } from '../api/session'
import {
  exchangeSsoCode,
  redirectToPortal,
  saveAuthTokens,
  startTokenKeepAlive,
  trustFreshToken,
} from '../utils/auth'

const route = useRoute()
const router = useRouter()

/** 仅允许站内相对路径，防开放重定向 */
function safeRedirect(raw?: string | string[]): string {
  const v = Array.isArray(raw) ? raw[0] : raw
  if (!v || typeof v !== 'string') return '/pending'
  const path = v.trim()
  if (!path.startsWith('/') || path.startsWith('//')) return '/pending'
  return path
}

onMounted(async () => {
  const dest = safeRedirect(route.query.redirect as string | undefined)
  const code = route.query.code as string | undefined
  if (code) {
    try {
      const tokens = await exchangeSsoCode(code)
      saveAuthTokens(tokens.accessToken, tokens.refreshToken, tokens.expiresAt)
      trustFreshToken()
      startTokenKeepAlive()
      await fetchSession()
      router.replace(dest)
      return
    } catch {
      redirectToPortal()
      return
    }
  }
  const info = await fetchSession()
  if (!info) {
    redirectToPortal()
    return
  }
  trustFreshToken()
  startTokenKeepAlive()
  router.replace(dest)
})
</script>

<template>
  <div class="auth-callback">正在登录…</div>
</template>

<style scoped>
.auth-callback {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #909399;
}
</style>
