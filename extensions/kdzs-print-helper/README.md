# OSMS 快递助手打单助手（Chrome 扩展）

从发货中心「打单发货」带入：**快递模板名、订单单号、勾选商品** → 在快递助手批打页自动勾选订单并尽量点「选择发货」、选模板；**打印由人工点击**。

## 任务传递（云端）

1. 发货中心把任务写入 ShippingCore：`POST /api/v1/admin/kdzs/helper-handoff-sessions`  
2. 打开快递助手时 URL 带 `_osms_ht=<token>`（`window.name` 备份）  
3. 本扩展在 KDZS 页向云端拉取：  
   `GET https://osms.zfcycle.com/apps/shipping/api/v1/mobile/kdzs-helper-handoff/<token>`  
4. 拉取成功后自动选单（**不自动点打印**）

任务约 **30 分钟**过期。

## 安装（开发者模式）

1. Chrome 打开 `chrome://extensions`  
2. 开启「开发者模式」  
3. 「加载已解压的扩展程序」→ 本目录  
4. 确认版本 **1.0.2+**；改代码后点「重新加载」

## 使用

1. 部署/更新 **ShippingCore API + Web**（含云端 handoff 接口）  
2. 重新加载本扩展  
3. 发货中心 → 打单发货 → 选模板 →「打开快递助手」  
4. 右下角面板应显示「已加载任务（云端）」  
5. 人工确认后点打印 → 回发货中心同步单号并确认发货  

面板若仍无任务：看日志是否「云端拉取失败」；确认 API 已发布且扩展能访问 `osms.zfcycle.com`。

## 权限说明

- `storage`：缓存已拉取任务  
- `tabs` / host：`*.kdzs.com`、`*.zfcycle.com`  
