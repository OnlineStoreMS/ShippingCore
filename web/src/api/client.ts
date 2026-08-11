import axios from 'axios'
import type { AxiosInstance } from 'axios'
import { getToken, redirectToPortal, clearToken, tryRefreshAccessToken } from '../utils/auth'

export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data?: T
}

export interface PageData<T> {
  list: T[]
  total: number
  page: number
  pageSize: number
}

const client: AxiosInstance = axios.create({
  baseURL: '/api/v1/admin',
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' },
  withCredentials: true,
})

client.interceptors.request.use((config) => {
  const token = getToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

client.interceptors.response.use(
  (res) => {
    // blob/arraybuffer（如面单 PDF）不是统一 JSON 包，不能按 code 校验
    const rt = res.config.responseType
    if (rt === 'blob' || rt === 'arraybuffer') {
      return res
    }
    if (typeof Blob !== 'undefined' && res.data instanceof Blob) {
      return res
    }
    const body = res.data as ApiResponse
    if (body && typeof body === 'object' && 'code' in body && body.code !== 200) {
      return Promise.reject(new Error(body.message || '请求失败'))
    }
    return res
  },
  async (err) => {
    const cfg = err.config as {
      _retry?: boolean
      headers?: Record<string, string>
      responseType?: string
    } | undefined
    // blob 错误体常是 JSON，尽量还原 message
    if (cfg?.responseType === 'blob' && err.response?.data instanceof Blob) {
      try {
        const text = await (err.response.data as Blob).text()
        const j = JSON.parse(text) as ApiResponse
        if (j?.message) {
          return Promise.reject(new Error(j.message))
        }
      } catch {
        /* ignore */
      }
    }
    if (err.response?.status === 401 && cfg && !cfg._retry) {
      const ok = await tryRefreshAccessToken()
      if (ok) {
        cfg._retry = true
        const token = getToken()
        if (token && cfg.headers) cfg.headers.Authorization = `Bearer ${token}`
        return client.request(cfg as any)
      }
    } else if (err.response?.status === 401) {
      clearToken()
      redirectToPortal()
    }
    return Promise.reject(err)
  },
)

export function unwrap<T>(res: { data: ApiResponse<T> }): T {
  return res.data.data as T
}

export default client
