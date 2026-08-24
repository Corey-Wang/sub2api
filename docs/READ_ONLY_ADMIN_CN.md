# 只读管理员

## 概述

系统新增 `readonly_admin` 角色。该角色可以登录管理控制台并查看管理数据，但不能执行管理面变更。

只读策略由后端强制执行，前端的提示和按钮状态仅用于改善使用体验，不能作为安全边界。

## 权限范围

### 允许

- 访问 `/admin/*` 和 `/admin/payment/*` 下的查询、统计、详情和监控接口；
- 访问管理控制台中的仪表盘、用户/分组/账号/渠道/运维等只读页面；
- 使用少量以 `POST` 承载复杂查询参数的纯查询接口，例如批量用量查询、外部账号同步预览、邮件模板预览和组合路由预览；
- 个人账户的自助操作（个人资料、自己的 API Key、登出等）保持原有行为。

### 拒绝

- 所有管理面上的 `POST`、`PUT`、`PATCH`、`DELETE` 请求，除明确列入纯查询白名单的接口外；
- 账号、代理、兑换码、备份下载和管理员 API Key 等凭证/导出读取；
- 账号刷新、探测、OAuth、测试、同步、恢复、重启、退款等会产生外部副作用的操作；
- 修改用户角色、余额、并发、状态、分组、系统设置或任何其他管理资源。

被拒绝的请求返回 HTTP `403`，错误码为 `READ_ONLY_ADMIN`。

## 角色管理

只有完整管理员（`admin`）可以创建或修改角色。创建或提升为 `admin` 或 `readonly_admin` 需要完成 step-up 二次验证。

完整管理员的最后一个账号不能降级为 `readonly_admin` 或普通用户，避免系统失去完整管理能力。

## API 与实现位置

- 角色常量：`backend/internal/domain/constants.go`
- 管理认证：`backend/internal/server/middleware/admin_auth.go`
- 只读守卫：`backend/internal/server/middleware/read_only_admin.go`
- 管理路由接线：`backend/internal/server/routes/admin.go`、`backend/internal/server/routes/payment.go`
- 管理员角色表单与提示：`frontend/src/components/admin/user/`、`frontend/src/components/layout/AppLayout.vue`
