# OSMS 快递助手·手机版（Chrome 扩展）

目录：`extensions/kdzs-print-mobile` · 版本 **1.4.2+**

## 能力（仅此）

1. 生成配对码 ↔ OpsMobile 绑定  
2. 心跳在线 + 领取远程打单任务  
3. 打开快递助手 → 勾选 → 按配置打印机自动打印 → 发货 → 回执  

## 不含

- 发货中心「打开快递助手」桥接 / `_osms_ht`  
- 电脑端 postMessage handoff  

发货中心电脑端请用：`extensions/kdzs-print-helper`（勿与本扩展同时启用）。

## 安装

1. 移除旧合并版扩展  
2. 加载本目录 → 名称应为 **OSMS 快递助手·手机版**  
3. 重新配对；OpsMobile 设置页保存打印机全名
