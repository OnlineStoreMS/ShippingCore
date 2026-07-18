<script setup lang="ts">
import { useRouter } from 'vue-router'
import {
  Goods,
  Box,
  OfficeBuilding,
  DataBoard,
  Document,
  Switch,
  Download,
  Upload,
  Search,
  List,
  Warning,
  Printer,
  Collection,
  Location,
} from '@element-plus/icons-vue'

const router = useRouter()

type QuickLink = {
  title: string
  desc: string
  path: string
  icon: typeof Search
  color: string
}

type Section = {
  title: string
  items: QuickLink[]
}

const sections: Section[] = [
  {
    title: '常用查询',
    items: [
      { title: '库存查询', desc: '结存数量、金额与库位', path: '/stock/balances', icon: Search, color: '#409eff' },
      { title: '库存汇总账', desc: '按仓库/SKU 汇总出入', path: '/stock/summary', icon: DataBoard, color: '#409eff' },
      { title: '库存明细表', desc: '出入库流水明细', path: '/stock/movements', icon: List, color: '#409eff' },
      { title: '滞销查询', desc: '久未动销库存分析', path: '/stock/slow-moving', icon: Warning, color: '#e6a23c' },
      { title: '盘点明细表', desc: '跨单盘点商品行查询', path: '/stocktake-details', icon: Document, color: '#e6a23c' },
    ],
  },
  {
    title: '业务单据',
    items: [
      { title: '仓库盘点单', desc: '新建/审核盘点单', path: '/stocktakes', icon: Document, color: '#67c23a' },
      { title: '仓库调拨单', desc: '仓间调拨过账', path: '/transfers', icon: Switch, color: '#67c23a' },
      { title: '其它入库单', desc: '非采购入库过账', path: '/other-inbounds', icon: Download, color: '#67c23a' },
      { title: '其它出库单', desc: '其它出库过账', path: '/other-outbounds', icon: Upload, color: '#f56c6c' },
    ],
  },
  {
    title: '商品与主档',
    items: [
      { title: '商品信息', desc: '父SKU / 库存SKU 档案', path: '/products', icon: Goods, color: '#909399' },
      { title: '商品类别', desc: '仓配分类维护', path: '/categories', icon: Collection, color: '#909399' },
      { title: '条码打印', desc: 'SKU 条码打印', path: '/barcode', icon: Printer, color: '#909399' },
      { title: '仓库设置', desc: '仓库档案与默认仓', path: '/warehouses', icon: OfficeBuilding, color: '#909399' },
      { title: '库位管理', desc: '库位编码与拣货位置', path: '/locations', icon: Location, color: '#909399' },
      { title: '包装规格', desc: '外包装规格档案', path: '/pack-specs', icon: Box, color: '#909399' },
    ],
  },
]

function go(path: string) {
  router.push(path)
}
</script>

<template>
  <div class="dashboard">
    <div class="page-hd">
      <h2>工作台</h2>
      <p>常用功能与查询入口，点击卡片即可跳转</p>
    </div>

    <section v-for="sec in sections" :key="sec.title" class="section">
      <div class="section-title">{{ sec.title }}</div>
      <el-row :gutter="14">
        <el-col
          v-for="item in sec.items"
          :key="item.path"
          :xs="24"
          :sm="12"
          :md="8"
          :lg="6"
        >
          <el-card class="tile" shadow="hover" @click="go(item.path)">
            <div class="tile-body">
              <div class="icon-wrap" :style="{ background: item.color + '14', color: item.color }">
                <el-icon :size="22"><component :is="item.icon" /></el-icon>
              </div>
              <div class="meta">
                <div class="title">{{ item.title }}</div>
                <div class="desc">{{ item.desc }}</div>
              </div>
            </div>
          </el-card>
        </el-col>
      </el-row>
    </section>
  </div>
</template>

<style scoped>
.dashboard {
  max-width: 1280px;
}
.page-hd {
  margin-bottom: 20px;
}
.page-hd h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #303133;
}
.page-hd p {
  margin: 6px 0 0;
  font-size: 13px;
  color: #909399;
}
.section {
  margin-bottom: 22px;
}
.section-title {
  font-size: 14px;
  font-weight: 600;
  color: #606266;
  margin-bottom: 10px;
  padding-left: 8px;
  border-left: 3px solid var(--el-color-primary);
}
.tile {
  margin-bottom: 14px;
  cursor: pointer;
  transition: border-color 0.15s, transform 0.15s;
  border: 1px solid var(--el-border-color-lighter);
}
.tile:hover {
  border-color: var(--el-color-primary-light-5);
  transform: translateY(-1px);
}
.tile :deep(.el-card__body) {
  padding: 16px;
}
.tile-body {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.icon-wrap {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.meta {
  min-width: 0;
}
.title {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 4px;
}
.desc {
  font-size: 12px;
  color: #909399;
  line-height: 1.45;
}
</style>
