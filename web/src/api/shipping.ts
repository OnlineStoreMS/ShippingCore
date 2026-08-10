import client, { unwrap, type PageData } from './client'

export interface CarrierAccount {
  id?: number
  carrierCode: string
  name: string
  partnerId: string
  checkword?: string
  useMonthly: boolean
  custId: string
  expressType: string
  /** 丰桥云打印模板编码，如 fm_76130_standard_XXXX（非客户编码） */
  templateCode: string
  env: string
  enabled: boolean
  remark: string
}

export interface ShipperProfile {
  id?: number
  name: string
  company: string
  mobile: string
  province: string
  city: string
  county: string
  address: string
  isDefault: boolean
  enabled: boolean
}

export interface OrderGoods {
  title: string
  skuName: string
  num: number
  outerId: string
  price: number
}

export interface OrderSnapshot {
  platform: string
  shopId: string
  sysTid: string
  sourceTid: string
  receiverName: string
  receiverMobile: string
  receiverProvince: string
  receiverCity: string
  receiverCounty: string
  receiverAddress: string
  goods: OrderGoods[]
}

export interface CreateShipmentFromOrderInput {
  carrierAccountId: number
  shipperProfileId: number
  useMonthly?: boolean
  expressType?: string
  payMethod?: number
  remark?: string
  totalWeight?: number
  orderId?: number
  sourceSystem?: 'ordercore' | 'storesyncagent'
  order: OrderSnapshot
}

export interface ConfirmKdzsShipInput {
  orderId: number
  expressNo: string
  expressCompany?: string
  order: OrderSnapshot
}

export interface ShipmentItem {
  id: number
  shipmentId: number
  goodsName: string
  quantity: number
  skuCode: string
  outerId: string
}

export interface Shipment {
  id: number
  sourceSystem: string
  sourceRef: string
  sourceTid: string
  platform: string
  shopId: string
  carrierAccountId: number
  shipperProfileId: number
  receiverName: string
  receiverMobile: string
  receiverProvince: string
  receiverCity: string
  receiverCounty: string
  receiverAddress: string
  shipperName: string
  shipperMobile: string
  shipperProvince: string
  shipperCity: string
  shipperCounty: string
  shipperAddress: string
  shipperCompany: string
  useMonthly: boolean
  payMethod: number
  custId: string
  expressType: string
  mailNo: string
  orderCoreOrderId?: number
  sfOrderId: string
  labelUrl: string
  labelData?: string
  status: string
  errorMessage?: string
  cargoName: string
  parcelQty: number
  createdAt: string
  updatedAt: string
  items?: ShipmentItem[]
}

export interface TradeGoods {
  title?: string
  skuName?: string
  picUrl?: string
  num?: number
  outerId?: string
  price?: number
}

export interface PendingOrder {
  platform: string
  platformName?: string
  sysTids?: string[]
  tids?: string[]
  shopName?: string
  shopId?: string
  receiverName?: string
  receiverMobile?: string
  receiverAddress?: string
  formattedReceiver?: string
  decrypted?: boolean
  tradeStatus?: string
  statusText?: string
  createTime?: string
  payTime?: string
  goods?: TradeGoods[]
}

export interface PendingOrderListResponse {
  total: number
  pageNo: number
  pageSize: number
  items: PendingOrder[]
  hint?: string
}

export interface PendingOrderQuery {
  platform?: string
  shopId?: string
  tradeStatus?: string
  pageNo?: number
  pageSize?: number
  timeType?: number
  startDateTime?: string
  endDateTime?: string
}

export interface DecryptPendingOrdersInput {
  platform: string
  tradeStatus: string
  sysTids: string[]
}

export interface KdzsAccountDetail {
  code: string
  name: string
  role: string
  roleLabel: string
  mobile: string
  enabled: boolean
  sortOrder?: number
  passwordSet?: boolean
  active?: boolean
  isDefault?: boolean
  source?: string
  sourceLabel?: string
}

export interface KdzsAccountInput {
  code: string
  name?: string
  role?: string
  mobile: string
  password: string
  enabled?: boolean
  sortOrder?: number
}

export interface KdzsAccountUpdateInput {
  name?: string
  role?: string
  mobile?: string
  password?: string
  enabled?: boolean
  sortOrder?: number
}

export interface ExpressTemplate {
  id: number
  source: string
  kdzsAccountCode?: string
  kdzsAccountName?: string
  platform: string
  templateId: string
  templateName: string
  carrierCode: string
  carrierName: string
  shopId: string
  shopName: string
  enabled: boolean
  syncedAt: string
}

export interface WaybillAuth {
  id: number
  source: string
  kdzsAccountCode?: string
  kdzsAccountName?: string
  platform: string
  accountName: string
  shopName: string
  authStatus: string
  detail: string
  rawJson?: string
  syncedAt: string
}

export interface OMSOrderItem {
  productName?: string
  skuSpecs?: string
  quantity?: number
  picUrl?: string
}

export interface OMSOrderAddress {
  name?: string
  phone?: string
  province?: string
  city?: string
  district?: string
  address?: string
  fullText?: string
}

export interface OMSOrder {
  id: number
  orderNo: string
  sourceChannel: string
  platform: string
  platformOrderId?: string
  platformSysTid?: string
  shopId?: string
  shopName?: string
  buyerName?: string
  buyerPhone?: string
  shipStatus?: string
  status?: string
  payTime?: string
  orderedAt?: string
  items?: OMSOrderItem[]
  address?: OMSOrderAddress
}

export interface OMSOrderListResponse {
  list: OMSOrder[]
  total: number
  page: number
  pageSize: number
}

export interface OMSOrderQuery {
  page?: number
  pageSize?: number
  shipStatus?: string
  allocType?: string
  platform?: string
  keyword?: string
  platformSysTid?: string
  sourceChannel?: string
}

async function page<T>(url: string, params?: Record<string, unknown>): Promise<PageData<T>> {
  const res = await client.get(url, { params })
  return unwrap(res) as PageData<T>
}

export const shippingApi = {
  listCarrierAccounts: (params?: Record<string, unknown>) =>
    page<CarrierAccount>('/carrier-accounts', params),
  getCarrierAccount: (id: number) =>
    client.get(`/carrier-accounts/${id}`).then((r) => unwrap<CarrierAccount>(r)),
  createCarrierAccount: (body: CarrierAccount) =>
    client.post('/carrier-accounts', body).then((r) => unwrap<CarrierAccount>(r)),
  updateCarrierAccount: (id: number, body: CarrierAccount) =>
    client.put(`/carrier-accounts/${id}`, body).then((r) => unwrap<CarrierAccount>(r)),
  deleteCarrierAccount: (id: number) => client.delete(`/carrier-accounts/${id}`),

  listShipperProfiles: (params?: Record<string, unknown>) =>
    page<ShipperProfile>('/shipper-profiles', params),
  getShipperProfile: (id: number) =>
    client.get(`/shipper-profiles/${id}`).then((r) => unwrap<ShipperProfile>(r)),
  createShipperProfile: (body: ShipperProfile) =>
    client.post('/shipper-profiles', body).then((r) => unwrap<ShipperProfile>(r)),
  updateShipperProfile: (id: number, body: ShipperProfile) =>
    client.put(`/shipper-profiles/${id}`, body).then((r) => unwrap<ShipperProfile>(r)),
  deleteShipperProfile: (id: number) => client.delete(`/shipper-profiles/${id}`),
  setDefaultShipperProfile: (id: number) =>
    client.post(`/shipper-profiles/${id}/set-default`).then((r) => unwrap<ShipperProfile>(r)),

  listShipments: (params?: Record<string, unknown>) => page<Shipment>('/shipments', params),
  getShipment: (id: number) => client.get(`/shipments/${id}`).then((r) => unwrap<Shipment>(r)),
  createShipmentFromOrder: (body: CreateShipmentFromOrderInput) =>
    client.post('/shipments/from-order', body).then((r) => unwrap<Shipment>(r)),
  createShipmentWaybill: (id: number) =>
    client.post(`/shipments/${id}/create-waybill`).then((r) => unwrap<Shipment>(r)),
  printShipment: (id: number) =>
    client.post(`/shipments/${id}/print`).then((r) => unwrap<Shipment>(r)),
  /** 拉取本地面单 PDF（带鉴权），返回 blob URL，调用方用完应 URL.revokeObjectURL */
  fetchShipmentLabelBlob: async (id: number): Promise<string> => {
    const res = await client.get(`/shipments/${id}/label`, { responseType: 'blob' })
    const blob = res.data as Blob
    if (!blob || blob.size === 0) {
      throw new Error('面单 PDF 为空')
    }
    const ctype = (blob.type || '').toLowerCase()
    if (ctype.includes('json') || ctype.includes('text')) {
      const text = await blob.text()
      try {
        const j = JSON.parse(text) as { message?: string }
        throw new Error(j.message || '获取面单失败')
      } catch (e) {
        if (e instanceof SyntaxError) throw new Error(text.slice(0, 200) || '获取面单失败')
        throw e
      }
    }
    return URL.createObjectURL(new Blob([blob], { type: 'application/pdf' }))
  },
  cancelShipment: (id: number) =>
    client.post(`/shipments/${id}/cancel`).then((r) => unwrap<Shipment>(r)),

  listPendingOrders: (params: PendingOrderQuery) =>
    client.get('/pending-orders', { params }).then((r) => unwrap<PendingOrderListResponse>(r)),
  decryptPendingOrders: (body: DecryptPendingOrdersInput) =>
    client.post('/pending-orders/decrypt', body).then((r) => unwrap<PendingOrderListResponse>(r)),

  listPendingOMSOrders: (params: OMSOrderQuery) =>
    client.get('/pending-oms-orders', { params }).then((r) => unwrap<OMSOrderListResponse>(r)),

  listKdzsAccountDetails: () =>
    client.get('/kdzs/account-details').then((r) => unwrap<{ items: KdzsAccountDetail[]; total: number }>(r)),
  syncKdzsAccounts: () =>
    client.post('/kdzs/accounts/sync').then((r) =>
      unwrap<{ synced: number; defaultAccountCode?: string; activeAccountCode?: string }>(r),
    ),
  createKdzsAccount: (body: KdzsAccountInput) =>
    client.post('/kdzs/accounts', body).then((r) => unwrap<KdzsAccountDetail>(r)),
  updateKdzsAccount: (id: string, body: KdzsAccountUpdateInput) =>
    client.put(`/kdzs/accounts/${id}`, body).then((r) => unwrap<KdzsAccountDetail>(r)),
  deleteKdzsAccount: (id: string) => client.delete(`/kdzs/accounts/${id}`),
  setDefaultKdzsAccount: (accountId: string) =>
    client.post('/kdzs/accounts/default', { accountId }),
  switchKdzsAccount: (accountId: string) =>
    client.post('/kdzs/accounts/switch', { accountId }),

  syncKdzsPrintAssets: () =>
    client.post('/sync/kdzs-print-assets').then((r) =>
      unwrap<{ auths: number; templates: number; authsDeleted?: number; templatesDeleted?: number }>(r),
    ),
  listExpressTemplates: (params?: Record<string, unknown>) =>
    page<ExpressTemplate>('/express-templates', params),
  listWaybillAuths: (params?: Record<string, unknown>) =>
    page<WaybillAuth>('/waybill-auths', params),
  getBatchPrintURL: (platform: string) =>
    client
      .get('/kdzs/batch-print-url', { params: { platform } })
      .then((r) => unwrap<{ url: string; platform: string }>(r)),
  queryPrintWaybills: (body: {
    platform: string
    items: { sysTid?: string; tid?: string }[]
  }) =>
    client.post('/kdzs/print-waybills', body).then((r) =>
      unwrap<{
        items: {
          sysTid?: string
          tid?: string
          found: boolean
          expressNo?: string
          expressCompany?: string
          expressCode?: string
          message?: string
        }[]
        total: number
      }>(r),
    ),

  confirmKdzsShip: (body: ConfirmKdzsShipInput) =>
    client.post('/shipments/confirm-kdzs-ship', body).then((r) => unwrap<Shipment>(r)),
}

export function maskCheckword(value?: string): string {
  if (!value) return '-'
  if (value.length <= 4) return '****'
  return '****' + value.slice(-4)
}

export const shipmentStatusMap: Record<string, { label: string; type: '' | 'success' | 'warning' | 'info' | 'danger' }> = {
  draft: { label: '草稿', type: 'info' },
  created: { label: '已建单', type: 'success' },
  printed: { label: '已打印', type: 'success' },
  cancelled: { label: '已取消', type: 'info' },
  failed: { label: '失败', type: 'danger' },
}
