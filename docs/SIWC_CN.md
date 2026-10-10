# 官方 SIWC 授权（Sign in with ChatGPT）

账号类型 `siwc` 使用 OpenAI 官方的 [Sign in with ChatGPT token sharing](https://developers.openai.com/siwc/token-sharing-open-source) 预览。用户用自己的 ChatGPT 登录，sub2api 拿到只能调用 `https://api.openai.com/v1/responses` 和 `/v1/models` 的令牌。

这个计划只允许本人使用自己的 ChatGPT 套餐。把令牌放进多人共享分组违反 OpenAI 条款。管理后台创建该类型时会显示「仅限本人使用，不要放入多人共享分组」。服务端拒绝把账号绑到非专属分组，并且只接受所有者自己的 API Key（分组必须是专属分组，Key 的用户 ID 必须等于账号里的 `owner_user_id`）。

不要把这种账号和 Codex OAuth 混用。SIWC 不会访问 `chatgpt.com/backend-api`，也不会套用 Codex 的客户端指纹。

## 预览限制

上游只接受官方文档列出的能力。网关会强制 `store: false`、`stream: true`，并去掉这些字段后记一条日志：

`background`、`conversation`、`max_output_tokens`、`max_tool_calls`、`metadata`、`moderation`、`multi_agent`、`prompt`、`prompt_cache_retention`、`safety_identifier`、`temperature`、`top_logprobs`、`top_p`、`truncation`、`user`、`previous_response_id`。

下游如果请求非流式，网关仍向上游打开流，再把 SSE 聚合成一条 JSON。`/v1/responses` 上如果客户端自己带了 `previous_response_id`，现有的 HTTP 续传检查会直接拒绝（这个字段本来就不被 SIWC 支持）。Chat Completions / Anthropic Messages 转换层如果生成了 `previous_response_id`，发送前会被剥掉，因此只带上一轮 ID、不带完整上文的多轮对话会丢上下文。

模型列表只保留 `visibility == "list"` 的 `slug`。

## 本地启动

发布用的 `deploy/docker-compose.yml` 拉的是 `weishaw/sub2api:latest`，测这个功能要用本地源码：

```bash
cd deploy
docker compose -f docker-compose.dev.yml up --build
```

默认管理后台在 `http://127.0.0.1:8080`。

## 创建并授权

1. 在分组里建一个专属分组（不要用公共分组或默认共享分组）。
2. 账号管理 → 添加账号 → OpenAI →「官方 SIWC 授权」。
3. 填名称，分组只选那个专属分组。回调地址保持 `http://127.0.0.1:1455/auth/callback`（必须是 `127.0.0.1`，不能写 `localhost`）。
4. 生成授权链接，在本机浏览器打开并登录 ChatGPT，同意套餐调用。
5. 把浏览器地址栏里的完整回调 URL 粘贴回去，点完成授权，再提交创建。
6. 给这个账号的所有者发一把只绑该专属分组的 API Key。

重新授权和撤销在编辑账号面板里。撤销会调用发现文档里的 revocation endpoint，并清掉本地 access/refresh token。

登录会话存在签发授权链接的那一个进程内存里，30 分钟过期。多副本时，生成链接和粘贴回调必须打到同一实例。

## 用 curl 调用

```bash
curl http://127.0.0.1:8080/v1/responses \
  -H "Authorization: Bearer $SUB2API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.4","input":"ping"}'
```

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $SUB2API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.4","messages":[{"role":"user","content":"ping"}]}'
```

非流式响应由网关聚合。用量超限时客户端会看到 `subscription_sharing_usage_limit_exceeded`（HTTP 429）。账号会被本地限流，重置时间以 ChatGPT 用量页为准，网关不会猜测重置点。

## 用 Codex CLI 打到 sub2api

把 Codex 的 API 基址指到本机网关，并使用上面的 API Key。Codex 仍然走 `/v1/responses`。不要把 SIWC 账号配成 Codex OAuth 账号；OAuth 类型走的是另一套 ChatGPT 后端。

```bash
export OPENAI_BASE_URL=http://127.0.0.1:8080/v1
export OPENAI_API_KEY=$SUB2API_KEY
codex
```

如果客户端带了 `temperature` 或 `previous_response_id`，前者会被去掉，后者在 Responses 入口会被拒绝。需要多轮时请每次发送完整输入。

## 文档里不够明确、实现里采用的约定

- `earliest_refresh_at` 接受 Unix 秒、大于 `1e12` 的毫秒，或 RFC3339。其他格式忽略。它只推迟主动刷新；访问令牌已经过期时仍然刷新。
- ID Token 只接受非对称算法：RS256/384/512、PS256/384/512、ES256/384。拒绝 `none` 和 HMAC。
- 粘贴的回调 URL 不读取 `id_token_hint`。重新授权只用已保存的邮箱作为 `login_hint`。
- 模型可见性只认 `visibility == "list"`。
- 实例级 `ext_agent_host_id` 存在设置表 `siwc_ext_agent_host_id`，格式 `urn:uuid:...`。
- 个人使用模型是：专属分组 + 凭证里的 `owner_user_id`。创建账号的管理员就是所有者。
- 刷新发生在请求路径上，使用现有的按账号 Redis 锁。没有把 SIWC 注册进 Codex 的后台刷新器。
- 首次注册下发的 `client_id` 必须匹配 `^oaiapp_[A-Za-z0-9_-]{1,180}$`。`dynamic_agent_client` 不能当账号的 client id 保存。
- 不发送 `force_reconsent`。只有勾选重新请求同意时才带 `prompt=consent`。
- `refresh_token_reused` 等不可恢复的刷新错误会记成需要重新授权。
