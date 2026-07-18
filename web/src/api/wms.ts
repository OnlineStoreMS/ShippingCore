import client from './client'

export interface PageResult<T> {
  list: T[]
  total: number
  page: number
  pageSize: number
}

async function page<T>(url: string, params?: Record<string, unknown>) {
  const { data } = await client.get(url, { params })
  return data.data as PageResult<T>
}

export const api = {
  // categories
  listCategories: (params?: Record<string, unknown>) => page<any>('/categories', params),
  createCategory: (body: unknown) => client.post('/categories', body),
  updateCategory: (id: number, body: unknown) => client.put(`/categories/${id}`, body),
  deleteCategory: (id: number) => client.delete(`/categories/${id}`),

  // pack specs
  listPackSpecs: (params?: Record<string, unknown>) => page<any>('/pack-specs', params),
  createPackSpec: (body: unknown) => client.post('/pack-specs', body),
  updatePackSpec: (id: number, body: unknown) => client.put(`/pack-specs/${id}`, body),
  deletePackSpec: (id: number) => client.delete(`/pack-specs/${id}`),
  listPackSpecSkus: (id: number) => client.get(`/pack-specs/${id}/skus`).then((r) => r.data.data as any[]),
  bindPackSpecSku: (id: number, body: unknown) => client.post(`/pack-specs/${id}/skus`, body),
  updatePackSpecSku: (id: number, body: unknown) => client.put(`/pack-spec-skus/${id}`, body),
  unbindPackSpecSku: (id: number) => client.delete(`/pack-spec-skus/${id}`),

  // products
  listProducts: (params?: Record<string, unknown>) => page<any>('/products', params),
  getProduct: (id: number) => client.get(`/products/${id}`).then((r) => r.data.data),
  createProduct: (body: unknown) => client.post('/products', body),
  createProductWithSkus: (body: unknown) => client.post('/products/with-skus', body),
  updateProduct: (id: number, body: unknown) => client.put(`/products/${id}`, body),
  updateProductWithSkus: (id: number, body: unknown) => client.put(`/products/${id}/with-skus`, body),
  deleteProduct: (id: number) => client.delete(`/products/${id}`),

  // VMS suppliers (SupplyCore proxy)
  listSuppliers: (params?: Record<string, unknown>) => page<any>('/suppliers', params),

  // skus
  listSkus: (params?: Record<string, unknown>) => page<any>('/skus', params),
  getSku: (id: number) => client.get(`/skus/${id}`).then((r) => r.data.data),
  createSku: (body: unknown) => client.post('/skus', body),
  updateSku: (id: number, body: unknown) => client.put(`/skus/${id}`, body),
  deleteSku: (id: number) => client.delete(`/skus/${id}`),

  // boms
  listBoms: (params?: Record<string, unknown>) => page<any>('/boms', params),
  getBom: (id: number) => client.get(`/boms/${id}`).then((r) => r.data.data),
  saveBom: (body: unknown) => client.post('/boms', body),
  deleteBom: (id: number) => client.delete(`/boms/${id}`),

  // warehouses
  listWarehouses: (params?: Record<string, unknown>) => page<any>('/warehouses', params),
  createWarehouse: (body: unknown) => client.post('/warehouses', body),
  updateWarehouse: (id: number, body: unknown) => client.put(`/warehouses/${id}`, body),
  deleteWarehouse: (id: number) => client.delete(`/warehouses/${id}`),

  listLocations: (params?: Record<string, unknown>) => page<any>('/locations', params),
  createLocation: (body: unknown) => client.post('/locations', body),
  updateLocation: (id: number, body: unknown) => client.put(`/locations/${id}`, body),
  deleteLocation: (id: number) => client.delete(`/locations/${id}`),
  listLocationSkus: (id: number) => client.get(`/locations/${id}/skus`).then((r) => r.data.data as any[]),
  bindLocationSku: (id: number, body: unknown) => client.post(`/locations/${id}/skus`, body).then((r) => r.data.data),
  unbindLocationSku: (id: number) => client.delete(`/location-skus/${id}`),

  // stock
  stockBalances: (params?: Record<string, unknown>) => page<any>('/stock/balances', params),
  stockSummary: (params?: Record<string, unknown>) => page<any>('/stock/summary', params),
  stockMovements: (params?: Record<string, unknown>) => page<any>('/stock/movements', params),
  stockSlowMoving: (params?: Record<string, unknown>) => page<any>('/stock/slow-moving', params),

  // other in/out
  listOtherIn: (params?: Record<string, unknown>) => page<any>('/other-inbounds', params),
  getOtherIn: (id: number) => client.get(`/other-inbounds/${id}`).then((r) => r.data.data),
  createOtherIn: (body: unknown) => client.post('/other-inbounds', body),
  postOtherIn: (id: number) => client.post(`/other-inbounds/${id}/post`),
  cancelOtherIn: (id: number) => client.post(`/other-inbounds/${id}/cancel`),

  listOtherOut: (params?: Record<string, unknown>) => page<any>('/other-outbounds', params),
  getOtherOut: (id: number) => client.get(`/other-outbounds/${id}`).then((r) => r.data.data),
  createOtherOut: (body: unknown) => client.post('/other-outbounds', body),
  postOtherOut: (id: number) => client.post(`/other-outbounds/${id}/post`),
  cancelOtherOut: (id: number) => client.post(`/other-outbounds/${id}/cancel`),

  // stocktake
  listStocktakes: (params?: Record<string, unknown>) => page<any>('/stocktakes', params),
  getStocktake: (id: number) => client.get(`/stocktakes/${id}`).then((r) => r.data.data),
  createStocktake: (body: unknown) => client.post('/stocktakes', body),
  updateStocktake: (id: number, body: unknown) => client.put(`/stocktakes/${id}`, body),
  addStocktakeItems: (id: number, body: unknown) => client.post(`/stocktakes/${id}/items`, body),
  deleteStocktakeItem: (id: number, itemId: number) => client.delete(`/stocktakes/${id}/items/${itemId}`),
  startStocktake: (id: number) => client.post(`/stocktakes/${id}/start`),
  countStocktake: (id: number, body: unknown) => client.post(`/stocktakes/${id}/count`, body),
  saveStocktakeCounts: (id: number, body: unknown) => client.post(`/stocktakes/${id}/save-counts`, body),
  postStocktake: (id: number) => client.post(`/stocktakes/${id}/post`),
  cancelStocktake: (id: number) => client.post(`/stocktakes/${id}/cancel`),
  deleteStocktake: (id: number) => client.delete(`/stocktakes/${id}`),
  listStocktakeDetails: (params?: Record<string, unknown>) => page<any>('/stocktake-details', params),

  // transfers
  listTransfers: (params?: Record<string, unknown>) => page<any>('/transfers', params),
  getTransfer: (id: number) => client.get(`/transfers/${id}`).then((r) => r.data.data),
  createTransfer: (body: unknown) => client.post('/transfers', body),
  shipTransfer: (id: number) => client.post(`/transfers/${id}/ship`),
  receiveTransfer: (id: number) => client.post(`/transfers/${id}/receive`),
  cancelTransfer: (id: number) => client.post(`/transfers/${id}/cancel`),

  // integrations
  listPimMappings: (params?: Record<string, unknown>) => page<any>('/pim-mappings', params),
  upsertPimMapping: (body: unknown) => client.post('/pim-mappings', body),
  deletePimMapping: (id: number) => client.delete(`/pim-mappings/${id}`),

  // goods tools
  getGoodsFeeSettings: () => client.get('/goods-fee-settings').then((r) => r.data.data),
  saveGoodsFeeSettings: (body: unknown) => client.put('/goods-fee-settings', body).then((r) => r.data.data),
  getSkuByCode: (skuCode: string) => client.get('/skus/by-code', { params: { skuCode } }).then((r) => r.data.data),
  updateSkuWeight: (body: { skuCode: string; weightG: number }) => client.post('/skus/update-weight', body).then((r) => r.data.data),
  listProfitTrials: (params?: Record<string, unknown>) => page<any>('/profit-trials', params),
  createProfitTrial: (body: unknown) => client.post('/profit-trials', body),
  updateProfitTrial: (id: number, body: unknown) => client.put(`/profit-trials/${id}`, body),
  deleteProfitTrials: (ids: number[]) => client.post('/profit-trials/delete', { ids }),
  calcProfitTrials: (body: unknown) => client.post('/profit-trials/calc', body).then((r) => r.data.data),
}
