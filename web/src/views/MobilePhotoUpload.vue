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
const doneUrl = ref('')
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
    if (s.status === 'done' && s.url) {
      status.value = 'done'
      doneUrl.value = s.url
      preview.value = s.url
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

async function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  // 微信相册偶发 type 为空，按扩展名兜底
  const okType = file.type.startsWith('image/') || /\.(jpe?g|png|gif|webp|bmp|heic)$/i.test(file.name)
  if (!okType) {
    ElMessage.error('请选择图片文件')
    return
  }
  uploading.value = true
  try {
    preview.value = URL.createObjectURL(file)
    const res = await mobileUploadPhoto(token.value, file)
    doneUrl.value = res.url
    status.value = 'done'
    ElMessage.success('上传成功，可返回电脑查看')
  } catch (err) {
    ElMessage.error((err as Error).message || '上传失败')
    status.value = 'expired'
  } finally {
    uploading.value = false
  }
}
</script>

<template>
  <div class="page" v-loading="loading">
    <header class="hdr">
      <h1>上传实拍图</h1>
      <p>拍照或从相册选择，上传后电脑端自动回填</p>
    </header>

    <div v-if="status === 'expired'" class="card err">
      二维码已过期或不存在，请在电脑端重新点击「扫码上传」。
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
          从相册选择
        </el-button>
      </div>
      <p v-if="status === 'done' && doneUrl" class="ok-tip">上传成功，请返回电脑端查看</p>
    </template>

    <!-- 微信：带 capture 只会调相机；相册必须用不带 capture 的 input -->
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
.hdr h1 {
  margin: 0;
  font-size: 22px;
  font-weight: 600;
}
.hdr p {
  margin: 8px 0 20px;
  color: #909399;
  font-size: 14px;
}
.card.err {
  padding: 20px;
  border-radius: 12px;
  background: #fef0f0;
  color: #f56c6c;
  line-height: 1.6;
}
.preview {
  width: 100%;
  aspect-ratio: 1;
  max-width: 420px;
  margin: 0 auto 20px;
  border-radius: 12px;
  overflow: hidden;
  background: #1a1a1a;
  display: flex;
  align-items: center;
  justify-content: center;
}
.preview img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.preview.empty {
  background: #eef2f6;
  color: #909399;
  flex-direction: column;
  gap: 10px;
  font-size: 14px;
}
.actions {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 420px;
  margin: 0 auto;
}
.actions .el-button {
  width: 100%;
  height: 48px;
  font-size: 16px;
  margin: 0;
}
.ok-tip {
  margin: 16px auto 0;
  text-align: center;
  color: #67c23a;
  font-size: 14px;
}
.hidden {
  display: none;
}
</style>
