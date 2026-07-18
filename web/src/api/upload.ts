import client, { unwrap } from './client'
import axios from 'axios'

export async function uploadImage(file: File, subdir = 'products'): Promise<string> {
  const form = new FormData()
  form.append('file', file)
  form.append('subdir', subdir)
  const res = await client.post('/upload', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  const data = unwrap<{ url: string }>(res)
  return data.url
}

export interface PhotoUploadSession {
  token: string
  status: 'pending' | 'done'
  url?: string
  expireAt: string
}

export async function createPhotoUploadSession(subdir = 'products'): Promise<PhotoUploadSession> {
  const res = await client.post('/photo-upload-sessions', { subdir })
  return unwrap<PhotoUploadSession>(res)
}

export async function getPhotoUploadSession(token: string): Promise<PhotoUploadSession> {
  const res = await client.get(`/photo-upload-sessions/${token}`)
  return unwrap<PhotoUploadSession>(res)
}

/** 手机端免登录查询/上传 */
const mobileClient = axios.create({
  baseURL: '/api/v1/mobile',
  timeout: 60000,
})

function unwrapMobile<T>(res: { data: { code: number; message: string; data?: T } }): T {
  if (res.data.code !== 200) {
    throw new Error(res.data.message || '请求失败')
  }
  return res.data.data as T
}

export async function mobileGetPhotoSession(token: string): Promise<PhotoUploadSession> {
  const res = await mobileClient.get(`/photo-upload/${token}`)
  return unwrapMobile<PhotoUploadSession>(res)
}

export async function mobileUploadPhoto(token: string, file: File): Promise<{ url: string; status: string }> {
  const form = new FormData()
  form.append('file', file)
  const res = await mobileClient.post(`/photo-upload/${token}`, form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return unwrapMobile(res)
}
