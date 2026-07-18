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
  cancelShipment: (id: number) =>
    client.post(`/shipments/${id}/cancel`).then((r) => unwrap<Shipment>(r)),

  listPendingOrders: (params: PendingOrderQuery) =>
    client.get('/pending-orders', { params }).then((r) => unwrap<PendingOrderListResponse>(r)),
  decryptPendingOrders: (body: DecryptPendingOrdersInput) =>
    client.post('/pending-orders/decrypt', body).then((r) => unwrap<PendingOrderListResponse>(r)),
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
