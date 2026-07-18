# ShippingCore（发货中心）

OSMS 平台发货管理中心：承运商账号、寄件人档案、运单创建/打印/取消，对接 StoreSyncAgent 待发货订单与顺丰丰桥 BSP。

| 项 | 值 |
|----|-----|
| Go module | `shippingcore` |
| API | `:8096` |
| Web | `:5181` |
| Docker 镜像 | `shippingcore-api`、`shippingcore-web` |
| 平台编排 | `/home/asialeaf/projects/deploy` |

## 功能

- **承运商账号** (`carrier_accounts`)：顺丰丰桥 partnerID / checkword，支持 sandbox / prod、月结卡
- **寄件人档案** (`shipper_profiles`)：默认寄件地址
- **运单** (`shipments`)：从 StoreSyncAgent 订单快照创建 → 丰桥下单 → 云打印 → 取消
- **待发货订单**：代理 StoreSyncAgent `wait_send` 列表与收件人解密

## 本地开发

```bash
# API（可用 configs/config.local.yaml + SQLite）
cd ShippingCore
go run ./cmd/api -config configs/config.local.yaml

# Web
cd web && npm install && npm run dev
```

从 UserCore 应用中心进入时，走 `/auth/callback?token=...`。

## Admin API（`/api/v1/admin`，JWT）

| 资源 | 路径 |
|------|------|
| 承运商账号 | `GET/POST /carrier-accounts`, `GET/PUT/DELETE /carrier-accounts/:id` |
| 寄件人 | `GET/POST /shipper-profiles`, `GET/PUT/DELETE /shipper-profiles/:id`, `POST .../set-default` |
| 运单 | `GET /shipments`, `GET /shipments/:id`, `POST /shipments/from-order`, `POST .../create-waybill`, `POST .../print`, `POST .../cancel` |
| 待发货 | `GET /pending-orders`, `POST /pending-orders/decrypt` |

## 数据库

```bash
make init-db APP_PASSWORD=你的密码
make fix-db-perms   # PG15+ public schema 权限修复
```

## 配置

`integrations.storesyncagent_api_url` 默认 `http://127.0.0.1:8097`。

## Docker / ACR

推送 `main` 后 GitHub Actions 构建并推送：

- `registry.cn-hangzhou.aliyuncs.com/<ns>/shippingcore-api:latest`
- `registry.cn-hangzhou.aliyuncs.com/<ns>/shippingcore-web:latest`
