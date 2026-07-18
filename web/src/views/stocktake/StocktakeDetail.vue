<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../../api/wms'
import StocktakeFormDialog from '../../components/StocktakeFormDialog.vue'

const route = useRoute()
const router = useRouter()
const warehouses = ref<any[]>([])
const formVisible = ref(true)
const id = computed(() => Number(route.params.id) || null)

async function loadWarehouses() {
  const res = await api.listWarehouses({ page: 1, pageSize: 200 })
  warehouses.value = (res.list || []).filter((w: any) => w.status !== 0)
}

onMounted(loadWarehouses)

watch(formVisible, (v) => {
  if (!v) router.push('/stocktakes')
})
</script>

<template>
  <div class="page">
    <StocktakeFormDialog
      v-model="formVisible"
      :stocktake-id="id"
      :warehouses="warehouses"
      @saved="() => {}"
    />
  </div>
</template>
