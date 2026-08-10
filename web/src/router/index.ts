import { createRouter, createWebHistory } from 'vue-router'
import AdminLayout from '../layouts/AdminLayout.vue'
import { getToken, redirectToPortal, ensureSession, clearToken } from '../utils/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/auth/callback',
      name: 'AuthCallback',
      component: () => import('../views/AuthCallback.vue'),
      meta: { public: true },
    },
    {
      path: '/auth/logout',
      name: 'AuthLogout',
      component: () => import('../views/AuthLogout.vue'),
      meta: { public: true },
    },
    {
      path: '/',
      component: AdminLayout,
      redirect: '/pending',
      children: [
        {
          // 兼容门户/旧书签的 /dashboard
          path: 'dashboard',
          redirect: '/pending',
        },
        {
          path: 'pending',
          name: 'PendingOrders',
          component: () => import('../views/PendingOrders.vue'),
          meta: { title: '待发货' },
        },
        {
          path: 'shipments',
          name: 'Shipments',
          component: () => import('../views/ShipmentList.vue'),
          meta: { title: '发货单' },
        },
        {
          path: 'sf-order',
          name: 'SFOrderPrint',
          component: () => import('../views/SFOrderPrint.vue'),
          meta: { title: '顺丰标准寄件' },
        },
        {
          path: 'clodop',
          name: 'CLodopService',
          component: () => import('../views/CLodopService.vue'),
          meta: { title: 'C-Lodop 云打印' },
        },
        {
          path: 'carrier-accounts',
          name: 'CarrierAccounts',
          component: () => import('../views/CarrierAccountList.vue'),
          meta: { title: '物流账号' },
        },
        {
          path: 'kdzs-accounts',
          name: 'KdzsAccounts',
          component: () => import('../views/KdzsAccountList.vue'),
          meta: { title: '账号管理' },
        },
        {
          path: 'waybill-auths',
          name: 'WaybillAuths',
          component: () => import('../views/WaybillAuthList.vue'),
          meta: { title: '面单授权' },
        },
        {
          path: 'express-templates',
          name: 'ExpressTemplates',
          component: () => import('../views/ExpressTemplateList.vue'),
          meta: { title: '快递模板' },
        },
        {
          path: 'shipper-profiles',
          name: 'ShipperProfiles',
          component: () => import('../views/ShipperProfileList.vue'),
          meta: { title: '寄件人' },
        },
      ],
    },
    {
      path: '/:pathMatch(.*)*',
      redirect: '/pending',
    },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.public) return true
  if (!getToken()) {
    redirectToPortal()
    return false
  }
  const ok = await ensureSession()
  if (!ok) {
    clearToken()
    redirectToPortal()
    return false
  }
  return true
})

export default router
