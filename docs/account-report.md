# 账号报表

管理后台的「账号报表」位于 `/admin/account-report`，管理员和只读管理员可以查看。普通用户无法访问对应 API。页面仅展示账号数量、平台状态、套餐分布、汇总额度进度和来源同步状态，不展示单个账号明细，也不提供账号变更接口。

GPT / Gemini 从当前 Sub2API 数据库读取，每分钟采集一次。影子账号不重复计数。GPT 展示已有上游快照，超过 10 分钟或重置时间已过的快照标为过期，不推测为零；缺少上游快照的账号显示额度未知。Gemini 使用现有本站请求统计和配置配额，明确标为估算，不能覆盖在本站以外产生的用量。

## Kiro 配置

Gemini 汇总包含直接 Gemini 账号和 Antigravity 渠道账号。Antigravity 复用现有上游额度查询，只展示 Gemini 模型的使用率和重置时间，按模型分别计算账号平均值；不把 Claude 等其他模型的配额计入 Gemini。查询不修改账号配置、启停状态或刷新凭据。

设置环境变量 `ACCOUNT_REPORT_KIRO_INSTANCES_FILE=/app/data/kiro-report-instances.json`，或配置：

```yaml
account_report:
  kiro_instances_file: /app/data/kiro-report-instances.json
```

实例文件为 JSON 数组。URL 为后端可访问的 Kiro 服务 origin，不包含 `/admin`。例如：

```json
[
  {
    "id": "kiro-45",
    "name": "Kiro 45",
    "url": "http://10.247.9.7:8990",
    "admin_key_file": "/app/data/kiro-45-admin-key"
  },
  {
    "id": "kiro-104",
    "name": "Kiro 104",
    "url": "http://10.247.9.6:8990",
    "admin_key_file": "/app/data/kiro-104-admin-key"
  }
]
```

密钥文件仅包含 Kiro adminApiKey；实例文件也支持 `admin_key` 字段。含密钥的文件使用权限 600，通过只读挂载提供给容器，不提交到 Git。修改实例配置后重启应用。未配置文件时仍提供 GPT / Gemini 报表。

Kiro 每 5 分钟采集一次，最多并发 3 个额度查询，每次采集最多 90 秒。仅调用 `GET /api/admin/credentials` 和 `GET /api/admin/credentials/:id/balance`。查询额度可能触发 Kiro 自身已有的令牌续期和额度缓存更新；不调用增删、启停、修改优先级或强制刷新接口。

## 去重和口径

同一 Kiro API Key 哈希或 refresh token 哈希出现在多个实例时，在汇总中只计一个账号，额度取最新一次有效观察，不能跨实例累加。哈希仅用于后端匹配，API 不返回凭据、原始哈希或管理密钥。

同邮箱但凭据不同的记录标记为「账号归属待核对」，不纳入额度汇总。缺少可靠标识时保留实例独立记录，也不纳入额度汇总。确认确属同一账号后，可在对应实例配置 `account_aliases`，将凭据编号映射到同一个自定义账号标识：

```json
"account_aliases": { "1": "team-account-a", "7": "team-account-b" }
```

账号总数是已知且能按现有标识合并后的数量，待核对的身份可能仍有重复。Kiro 额度按订阅套餐分别汇总；GPT 百分比是有有效快照账号的算术平均；Gemini 同一配额维度按总已用量 / 总限额计算。不同平台和窗口不合成总百分比。

当前可用状态来自实例调度状态和有效额度，并不发送收费模型请求验证账号。Gemini 某个模型或分钟额度耗尽标为限流，只有共享日额度或 Pro / Flash 日额度同时耗尽才标为账号额度耗尽。平台汇总按配额维度分别展示。

## 缓存和故障

两个 Sub2API 副本应使用相同的实例配置和已有 Redis 命名空间。采集通过 Redis 租约互斥，页面请求只读取共享结果，不同步触发上游查询。仅保存最新结果（保留 7 天用于故障时展示），不创建历史记录表，也不向 `accounts` / `usage_logs` 导入 Kiro 数据。

来源失败时保留上一次成功账号列表，标记来源过期，状态转为未知；单账号额度查询失败时保留旧额度并标过期。过期、身份待核对的额度不计入当前汇总。已知账号总数可能不完整时，页面显示提示。Redis 不可用时读取进程内最后结果，不绕过共享锁重复采集。
