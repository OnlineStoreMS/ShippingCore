# OSMS 快递助手·手机版（Chrome 扩展）— **已弃用**

> **Deprecated（2026-09）**  
> 远程打单已迁移到 **WindowsAgent** 任务类型「快递助手远程打单」。  
> 请改用 Agent：填写快递助手账号密码 → 生成配对码 → 手机「快递助手远程打单」页绑定。  
> 本目录仅保留对照/应急，不再作为正式接入方式。

目录：`extensions/kdzs-print-mobile`

## 历史能力

1. 生成配对码 ↔ OpsMobile 绑定  
2. 心跳在线 + 领取远程打单任务  
3. 打开快递助手 → 勾选 → 打印 → 发货  

## 仍在使用

发货中心电脑端 **本地打单** 请继续用：`extensions/kdzs-print-helper`（与 WindowsAgent 远程打单互不冲突）。

## 对照移植

WindowsAgent 注入脚本参考：`WindowsAgent/src/WindowsAgent.Skills/Scripts/kdzs-automate.js`（由本扩展自动化逻辑移植）。
