<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Camera, Picture } from '@element-plus/icons-vue'
import { mobileGetPhotoSession, mobileUploadPhoto } from '../api/upload'

const route = useRoute()
const token = computed(() => String(route.query.token || ''))

const loading = ref(true)
const uploading = ref(false)
const status = ref<'ok' | 'expired' | 'done'>('ok')
const preview = ref('')
const doneCount = ref(0)
const progressText = ref('')
const cameraInput = ref<HTMLInputElement | null>(null)
const albumInput = ref<HTMLInputElement | null>(null)

onMounted(async () => {
  if (!token.value) {
    status.value = 'expired'
    loading.value = false
    return
  }
  try {
    const s = await mobileGetPhotoSession(token.value)
    const items = s.items?.length ? s.items : s.url ? [{ url: s.url }] : []
    if (s.status === 'done' && items.length) {
      status.value = 'done'
      doneCount.value = items.length
      preview.value = items[items.length - 1].url
    } else {
      status.value = 'ok'
    }
  } catch {
    status.value = 'expired'
  } finally {
    loading.value = false
  }
})

function openCamera() {
  cameraInput.value?.click()
}

function openAlbum() {
  albumInput.value?.click()
}

function isImage(file: File) {
  return file.type.startsWith('image/') || /\.(jpe?g|png|gif|webp|bmp|heic)$/i.test(file.name)
}

async function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  if (!files.length) return
  const allowed = files.filter(isImage)
  if (!allowed.length) {
    ElMessage.error('请选择图片文件')
    return
  }
  if (allowed.length < files.length) {
    ElMessage.warning(`已跳过 ${files.length - allowed.length} 个非图片文件`)
  }
  uploading.value = true
  progressText.value = ''
  let ok = 0
  try {
    for (let i = 0; i < allowed.length; i++) {
      const file = allowed[i]
      preview.value = URL.createObjectURL(file)
      progressText.value = `正在上传 ${i + 1}/${allowed.length}`
      await mobileUploadPhoto(token.value, file, { final: i === allowed.length - 1 })
      ok++
    }
    doneCount.value = ok
    status.value = 'done'
    progressText.value = ''
    ElMessage.success(ok > 1 ? `已上传 ${ok} 张，可返回电脑查看` : '上传成功，可返回电脑查看')
  } catch (err) {
    ElMessage.error((err as Error).message || '上传失败')
    if (ok === 0) status.value = 'expired'
    else {
      doneCount.value = ok
      status.value = 'done'
    }
  } finally {
    uploading.value = false
    progressText.value = ''
  }
}
</script>

<template>
  <div class="page" v-loading="loading">
    <header class="hdr">
      <h1>手机上传照片</h1>
      <p>相册支持一次多选；上传后电脑端自动回填</p>
    </header>

    <div v-if="status === 'expired'" class="card err">
      二维码已过期或不存在，请在电脑端重新点击「手机扫码上传」。
    </div>

    <template v-else>
      <div class="preview" v-if="preview">
        <img :src="preview" alt="预览" />
      </div>
      <div class="preview empty" v-else>
        <el-icon :size="48"><Camera /></el-icon>
        <span>尚未选择图片</span>
      </div>

      <div class="actions">
        <el-button type="primary" size="large" :icon="Camera" :loading="uploading" @click="openCamera">
          拍照
        </el-button>
        <el-button size="large" :icon="Picture" :loading="uploading" @click="openAlbum">
          从相册多选
        </el-button>
      </div>
      <p v-if="progressText" class="prog">{{ progressText }}</p>
      <p v-if="status === 'done' && doneCount" class="ok-tip">
        已上传 {{ doneCount }} 张，请返回电脑端查看
      </p>
    </template>

    <input
      ref="cameraInput"
      type="file"
      accept="image/*"
      capture="environment"
      class="hidden"
      @change="onFileChange"
    />
    <input
      ref="albumInput"
      type="file"
      accept="image/*"
      multiple
      class="hidden"
      @change="onFileChange"
    />
  </div>
</template>

<style scoped>
.page {
  min-height: 100vh;
  padding: 24px 20px 40px;
  box-sizing: border-box;
  background: linear-gradient(180deg, #f0f4f8 0%, #fff 40%);
  color: #303133;
  font-family: system-ui, -apple-system, 'PingFang SC', 'Microsoft YaHei', sans-serif;
}
.hdr h1 { margin: 0; font-size: 22px; font-weight: 600; }
.hdr p { margin: 8px 0 20px; color: #909399; font-size: 14px; }
.card.err {
  padding: 20px; border-radius: 12px; background: #fef0f0; color: #f56c6c; line-height: 1.6;
}
.preview {
  width: 100%; aspect-ratio: 1; max-width: 420px; margin: 0 auto 20px; border-radius: 12px;
  overflow: hidden; background: #1a1a1a; display: flex; align-items: center; justify-content: center;
}
.preview img { width: 100%; height: 100%; object-fit: contain; }
.preview.empty {
  background: #eef2f6; color: #909399; flex-direction: column; gap: 10px; font-size: 14px;
}
.actions { display: flex; flex-direction: column; gap: 12px; max-width: 420px; margin: 0 auto; }
.actions .el-button { width: 100%; height: 48px; font-size: 16px; margin: 0; }
.prog, .ok-tip { margin: 16px auto 0; text-align: center; font-size: 14px; }
.prog { color: #409eff; }
.ok-tip { color: #67c23a; }
.hidden { display: none; }
</style>
