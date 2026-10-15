# Redis Cluster（含 Azure Redis OSS Cluster）

Sub2API 默认继续使用单节点 Redis，原配置、键名和非零 DB 不变。外部 Redis 使用 OSS Cluster 时，可显式开启集群路由，解决单节点客户端的 `MOVED` 以及多键 Lua/事务的 `CROSSSLOT` 错误。

## 配置

使用 `deploy/docker-compose.standalone.yml` 连接外部 Redis，在受保护的 env 文件中设置：

```dotenv
REDIS_HOST=redis.example.com
REDIS_PORT=10000
REDIS_ENABLE_TLS=true
REDIS_CLUSTER_ENABLED=true
REDIS_DB=0
REDIS_KEY_PREFIX='{sub2api}:'
REDIS_USERNAME=
REDIS_PASSWORD=
```

端口应按服务提供商填写。所有应用节点使用相同的数据库、JWT/TOTP 密钥、Redis 凭据和键前缀。Azure 访问密钥认证通常使用默认 Redis 用户；ACL 认证按服务提供商要求填写用户名。

对应 YAML 为 `redis.cluster_enabled`、`redis.key_prefix`、`redis.enable_tls`。默认集群关闭、前缀为空。开启集群时必须使用 DB 0，且前缀以非空 hash tag 开头，配置不符会拒绝启动。密码只放权限为 600 的配置文件，不加入版本库或镜像。

客户端根据集群返回的槽地址连接分片，因此应用节点既要能访问入口，也要能访问 Redis 返回的分片地址和端口。启用 TLS 时，分片连接继续按配置的 Redis 域名校验证书。

## 键空间和容量

应用各缓存、锁、限流、刷新会话和 PubSub 频道统一添加部署前缀；键值和消息内容不变。SCAN 限定部署命名空间并返回原逻辑键名。Scheduler Lua 中经 ARGV 构造的快照键也使用物理前缀。

统一 hash tag 将整个部署的数据放在一个槽上，保留原有多键事务与 Lua 原子性。**因此本部署只使用一个 Redis 分片的容量和吞吐，不能依靠增加分片扩容。** 当前方案适合需要兼容 OSS Cluster 的试运行；进一步分散数据需要重新设计跨键操作。

命名空间适配器对应用已使用的命令明确重写键参数；未支持的命令返回错误，避免写入未隔离的键。新增 Redis 命令时应同步更新适配器，并做行为验证。

## 本地构建和验证

使用仓库 Dockerfile 在本地构建镜像，再把成品部署到服务器。也可在本地交叉编译验证程序；不需要在服务器修改或编译源码：

```bash
cd backend
go test -race ./internal/pkg/redisnamespace
go test -tags=unit -p 4 ./internal/config ./internal/repository -run 'TestLoadRedis|TestConfigKeysAreEnvReachable|TestBuildRedisOptions|TestSchedulerCache_|TestRefreshTokenCache'
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../build/redis-cluster-verify ./cmd/redis-cluster-verify
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../build/redis-cache-migrate ./cmd/redis-cache-migrate
```

在能访问真实 Redis 的机器上运行成品验证程序，提供 `REDIS_HOST`、`REDIS_PASSWORD`，可选 `REDIS_USERNAME`、`REDIS_PORT`（默认 10000）。该程序强制启用 TLS，创建独立随机命名空间，验证事务、跨客户端 MGET、EVAL/EVALSHA、SCAN、PubSub、应用并发上限和 Scheduler 快照切换/退休/重开，最后清理自己的键，不写入应用数据库。

## 从单节点缓存迁移

迁移会写入目标部署命名空间，执行前应备份并停止全部应用节点，确认源 Redis/DB 是所需的专用实例，目标前缀没有其他部署使用。迁移程序读取单节点源 DB 0；其他源 DB 不在本工具范围内。

通过受保护的 env 文件提供：

```dotenv
REDIS_SOURCE_ADDR=source-redis:6379
REDIS_SOURCE_PASSWORD=
REDIS_HOST=redis.example.com
REDIS_PORT=10000
REDIS_USERNAME=
REDIS_PASSWORD=
REDIS_KEY_PREFIX='{sub2api}:'
```

目标必须使用带非空 hash tag 的专用前缀；缺少源地址、目标地址或合法前缀时，程序会在写入前退出。目标同名键会替换为源值。工具支持 string/hash/set/zset/list，保留语义值和剩余 TTL，并逐键回读核对，不依赖不同 Redis 版本之间的 DUMP/RESTORE 格式兼容。遇到其他类型或校验失败会停止；检查输出后再启动应用。失败时目标可能已部分写入，应用仍需保持停止直到处理完成。

迁移完成后验证两节点健康、真实登录及跨节点 refresh，再停止原 Redis 并保留其数据供回退。原 Redis 只保留切换时的缓存，切换后的目标会话不会自动反向同步。

## 本次代码来源

`0.2.15` 分支基于官方上游提交 `9591136d45cef26453348ed9532a7ebbce7cf99c`；`0.2.16` 分支继续同步官方正式标签 `v0.2.16`（提交 `01842576b3d5c1e2545b5a99ad555d36e28ed306`），并保留已在两个试运行节点验证的 Redis 集群兼容修改。不包含实际服务器地址、部署凭据或运行数据。提交本身不更新现有服务器镜像。
