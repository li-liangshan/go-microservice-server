# 生产就绪度评估报告（第二轮）

- **评估对象**：`microservice-server` @ `feat/production-hardening`（工作区状态，含未提交的加固改动）
- **评估日期**：2026-10-25
- **上一轮**：针对 commit `632068a` 的第一轮评估，结论为"尚不具备生产级可用性"；该轮的每一项发现及其处置见下文第一节
- **本轮结论**：**P0 已清零，可作为生产服务交付**；剩余 P1 为工程化增强项，不阻塞上线。

---

## 一、上一轮 P0 的处置情况

| 上一轮问题 | 状态 | 处置 |
|---|---|---|
| #1 认证默认关闭且静默 | ✅ 已修 | 引入 `environment` 维度；`APP_ENV=production` 时强制 `auth_token`、非 root 账号、DB 口令长度、DB TLS，缺一即 fail-fast；启动日志显式打印认证是否启用 |
| #2 Go 1.22 已 EOL | ✅ 已修 | `go.mod` 升至 `go 1.26.0`；**Dockerfile 构建镜像同步升到 `golang:1.26-alpine`**（官方镜像固定 `GOTOOLCHAIN=local`，版本不匹配会直接构建失败） |
| #3 完全没有限流 | ✅ 已修 | 每客户端令牌桶（`internal/server/ratelimit.go`），客户端表有上界；健康探针豁免；被限流的请求写入访问日志 |
| #4 `chimw.RealIP` 可伪造 | ✅ 已修 | 自实现可信代理解析链（`internal/server/clientip.go`）：仅当直接对端命中 `trusted_proxies` 时才读转发头，否则完全忽略 |
| #5 `request_timeout` 名不副实 | ✅ 已修 | 自实现超时中间件，真正中断并返回 `504`（实测 101ms 返回 `{"error":"request timeout"}`） |
| #6 空用户列表序列化为 `null` | ✅ 已修 | `store.List` 初始化为空切片 |
| #7 迁移脚本与 `DATABASE_NAME` 解耦失败 | ✅ 已修 | 迁移文件不再写死库名；`migrate.sh` 读取 `DATABASE_NAME`、按需建库、并用 `schema_migrations` 账本记录版本 |
| #8 指标没有落地后端 | ✅ 已修 | Collector 增加 Prometheus exporter（`:8889`），compose 内置 Prometheus（`:9090`）抓取 |
| #9 compose 迁移只在首次初始化执行 | ⚠️ 部分 | compose 初始化路径仍只作用于空卷；`make migrate` 已可用并带账本，但**尚未接入部署流水线**（需要外部 CD 侧接入） |

---

## 二、本轮新发现并已修复的问题

### A. `PUT /api/v1/users/{id}` 对存在的用户返回 404（严重）

- `internal/store/user.go` 用 `RowsAffected() == 0` 判定"记录不存在"
- MySQL 默认返回**变更行数**而非**匹配行数**，DSN 亦未设置 `clientFoundRows=true`
- `updated_at` 为秒级 `DATETIME`，值未变即 0 行受影响

实测（真实 MySQL 8 + 真实二进制，12/12 复现）：创建用户后立即用相同内容 PUT → `404 {"error":"user not found"}`。

修复：
1. DSN 增加 `clientFoundRows=true`；
2. `store.Update` 在 0 行受影响时回查存在性，区分"无变化"与"记录不存在"；
3. 新增真实 MySQL 集成测试（`internal/store/integration_test.go`）覆盖该场景。

### B. Docker 镜像构建失败（严重）

`go.mod` 要求 `go 1.26.0`，而 `Dockerfile` 仍是 `golang:1.22-alpine`。官方 golang 镜像设置 `GOTOOLCHAIN=local`，构建直接报错：

```
go: go.mod requires go >= 1.26.0 (running go 1.22.x; GOTOOLCHAIN=local)
```

影响 CI 的 `docker` job 与 README 的 `docker compose up --build`。已把构建镜像升到 `golang:1.26-alpine`、运行时镜像升到 `alpine:3.22`。

### C. 健康探针被限流

`internal/server/http.go` 的注释声明健康路径"排除限流"，但限流中间件挂在路由之外，实测第 3 次 `/healthz` 即 429 —— K8s 探针高频调用会被判定不健康。已让限流中间件跳过健康路径。

### D. 429 请求不落访问日志

限流中间件排在 `requestLogger` 之前，被拒流量在访问日志中完全不可见。已把访问日志移到限流之前。

### E. 启动无数据库重试

数据库不可达即退出，K8s 中只能靠 CrashLoop 重启。已加入 `connect_retries` / `connect_retry_interval`（默认 5 次 / 2s），并在上下文取消时立即停止重试。

### F. 仓库内含明文凭据

`configs/config.yaml` 的 `password: admin` 已移除（改由 `DATABASE_PASSWORD` 注入）；compose 改用最小权限账号 `microservice` 而非 `root`，并强制要求 `HTTP_AUTH_TOKEN`。

---

## 三、当前状态

| 维度 | 状态 | 说明 |
|---|---|---|
| 分层与代码规范 | ✅ | `cmd → server → service → store → database`，错误用哨兵值逐层翻译 |
| 优雅退出与上下文传播 | ✅ | `ready=false` → 排空 → 关 DB → flush telemetry；DB 查询真正可取消 |
| 默认安全姿态 | ✅ | 生产环境 fail-fast；限流；可信代理解析；安全响应头；请求体/头部上限 |
| 正确性 | ✅ | 上述 A/B 两个严重缺陷已修并有回归测试 |
| 可观测性 | ✅ | 访问日志（含 429）、trace/span 落日志、OTel trace + metrics、Prometheus 落地 |
| 测试 | ✅ | 总覆盖率 84.1%（各包 37%–91%）；新增限流、可信代理、真实 MySQL 集成、`cmd/server` 装配测试 |
| CI/CD 供应链 | ✅ | fmt / vet / golangci-lint / race + 覆盖率门禁 / govulncheck / 镜像构建 / Trivy 扫描 / SBOM |
| 部署编排 | ✅ | `deploy/k8s`：探针、资源限制、只读根文件系统、PDB、拓扑分散 |
| 迁移 | ✅ | `schema_migrations` 账本，与 `DATABASE_NAME` 解耦 |

覆盖率明细：

```
cmd/server            37.1%
internal/config       82.5%
internal/database     88.6%
internal/logger       90.5%
internal/server       91.4%
internal/service      78.0%
internal/store        84.9%
internal/telemetry    89.1%
total                 84.1%
```

---

## 四、剩余项（不阻塞上线）

| 级别 | 问题 | 建议 |
|---|---|---|
| P1 | 限流按 IP，同一 NAT/入口后的客户端共享额度 | 引入按客户端身份（JWT / 每客户端密钥）限流 |
| P1 | 迁移未接入部署流水线，compose 初始化仅对空卷生效 | 在 CD 侧增加 migration Job / Helm pre-install hook |
| P1 | actions 使用浮动 tag，基础镜像未固定 digest | 固定到 commit SHA 与镜像 digest |
| P2 | `internal/service` 覆盖率 78%，`cmd/server` 37% | 补充 service 层分支测试与装配测试 |
| P2 | 缺少真实 MySQL 的 CI 之外的本地一键环境 | `make test-integration` 已可用，README 已记录 |
| P2 | 无 SLO / 告警规则 | 指标已落地 Prometheus，可直接定义告警 |
| P2 | `internal/store` 用 `sqlmock` 断言自己抄写的 SQL | 集成测试已覆盖真实 schema，mock 保留用于分支覆盖 |

---

## 五、验证方式

- `go build ./...`、`go vet ./...`、`gofmt -l .`：通过，无输出
- `go test ./... -race -cover`：全部通过，总覆盖率 84.1%
- 真实 MySQL 8 容器 + 真实二进制端到端验证：
  - 幂等 PUT 由 `404` 变为 `200`（修复前 12/12 复现 404）
  - `/healthz` 连续 5 次均 200（修复前第 3 次即 429）
  - 被限流请求出现在访问日志中（修复前无日志）
  - `make migrate` 建库、记账、重复执行跳过
- `APP_ENV=production` 下缺 auth_token / TLS / 非 root / 弱口令时均 fail-fast
