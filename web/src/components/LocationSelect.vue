<script setup lang="ts">
import { ref, watch } from 'vue'
import { api } from '../api/wms'

const props = defineProps<{
  modelValue?: number | null
  warehouseId?: number
  placeholder?: string
}>()
const emit = defineEmits<{ (e: 'update:modelValue', v: number | null): void }>()

const options = ref<any[]>([])
const loading = ref(false)

async function load() {
  if (!props.warehouseId) {
    options.value = []
    return
  }
  loading.value = true
  try {
    const res = await api.listLocations({
      warehouseId: props.warehouseId,
      page: 1,
      pageSize: 200,
    })
    options.value = res.list || []
  } finally {
    loading.value = false
  }
}

watch(
  () => props.warehouseId,
  () => {
    emit('update:modelValue', null)
    load()
  },
  { immediate: true },
)

function labelOf(item: any) {
  if (item.zone) return `${item.code} (${item.zone})`
  return item.code
}

function onChange(id: number | null | undefined) {
  emit('update:modelValue', id ?? null)
}
</script>

<template>
  <el-select
    :model-value="modelValue ?? undefined"
    filterable
    clearable
    :loading="loading"
    :disabled="!warehouseId"
    :placeholder="placeholder || '选择库位'"
    style="width: 100%"
    @update:model-value="onChange"
  >
    <el-option
      v-for="item in options"
      :key="item.id"
      :label="labelOf(item)"
      :value="item.id"
    />
  </el-select>
</template>
