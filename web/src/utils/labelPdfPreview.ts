import * as pdfjs from 'pdfjs-dist'
import pdfWorkerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url'

let workerReady: Promise<void> | null = null

/** 用 blob URL 挂 worker，避免 nginx 把 .mjs 标成 octet-stream 导致动态 import 失败 */
function ensurePdfWorker() {
  if (!workerReady) {
    workerReady = (async () => {
      try {
        const res = await fetch(pdfWorkerUrl)
        if (!res.ok) throw new Error(`worker HTTP ${res.status}`)
        const buf = await res.arrayBuffer()
        const blob = new Blob([buf], { type: 'application/javascript' })
        pdfjs.GlobalWorkerOptions.workerSrc = URL.createObjectURL(blob)
      } catch {
        pdfjs.GlobalWorkerOptions.workerSrc = pdfWorkerUrl
      }
    })()
  }
  return workerReady
}

/** 将面单 PDF（URL）渲染为 PNG data URL（首页） */
export async function renderLabelPdfToPng(pdfUrl: string, scale = 2): Promise<string> {
  const url = (pdfUrl || '').trim()
  if (!url) throw new Error('无面单存档')

  await ensurePdfWorker()

  const res = await fetch(url, { mode: 'cors' })
  if (!res.ok) throw new Error(`拉取面单失败 HTTP ${res.status}`)
  const data = new Uint8Array(await res.arrayBuffer())

  const doc = await pdfjs.getDocument({ data }).promise
  try {
    const page = await doc.getPage(1)
    const viewport = page.getViewport({ scale })
    const canvas = document.createElement('canvas')
    canvas.width = Math.ceil(viewport.width)
    canvas.height = Math.ceil(viewport.height)
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('无法创建画布')
    // pdfjs 4.4：canvasContext + viewport
    await page.render({ canvasContext: ctx, viewport } as Parameters<typeof page.render>[0]).promise
    return canvas.toDataURL('image/png')
  } finally {
    await doc.destroy()
  }
}

export function downloadDataUrl(dataUrl: string, filename: string) {
  const a = document.createElement('a')
  a.href = dataUrl
  a.download = filename
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
}

export async function copyText(text: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text)
    return
  }
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'
  ta.style.left = '-9999px'
  document.body.appendChild(ta)
  ta.select()
  document.execCommand('copy')
  ta.remove()
}

/** 尝试复制 PNG 到剪贴板；不支持时抛错由调用方回退 */
export async function copyPngDataUrl(dataUrl: string) {
  if (!navigator.clipboard?.write || typeof ClipboardItem === 'undefined') {
    throw new Error('当前环境不支持复制图片')
  }
  const blob = await (await fetch(dataUrl)).blob()
  await navigator.clipboard.write([new ClipboardItem({ [blob.type || 'image/png']: blob })])
}
