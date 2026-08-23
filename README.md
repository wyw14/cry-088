# cry-088 项目制工时平台

这是一个离线可运行的项目制工时填报、审批与成本分析平台。后端以 Go 1.24、Gin、pgx、validator、zap 和 PostgreSQL 组成；前端使用 Vue 3、TypeScript、Vite、Pinia 与 Element Plus。外部通知、文件和时间能力都通过接口隔离，并提供本地确定性演示实现。

## 架构与目录职责

`internal/domain` 保存实体、值对象、状态机和策略；`internal/application` 保存用例及事务边界；`internal/repository/postgres` 保存 PostgreSQL 仓储实现；`internal/transport/http` 处理 HTTP、请求关联、错误响应和安全头；`internal/platform` 提供时钟、ID、幂等、文件和 outbox 端口；`migrations` 保存可重复执行的表结构与种子；`web` 按 pages、features、components、stores、router、services、types、composables 和 tests 拆分。

核心状态机为 `draft -> submitted -> approved -> locked`，退回进入 `returned` 后可再次提交；锁定记录不能直接编辑，必须通过冲销和更正单；结算期由 `open -> closing -> closed`，关闭后普通角色不可写入。

## 本地启动

```sh
cp .env.example .env
go mod download
go run ./cmd/server
```

演示配置会在不连接数据库时启动本地 API 展示数据；生产部署通过 `DATABASE_URL` 连接 PostgreSQL，并执行 `make migrate seed`。演示登录账号为 `demo@example.com`，密码为 `DemoPassword123!`。

## 容器启动

```sh
docker compose up --build
```

Compose 启动应用和 PostgreSQL，Go 镜像构建阶段支持官方 amd64/arm64 基础镜像，运行层使用非 root 用户。生产环境必须替换 JWT 密钥和数据库密码。

## 接口示例

```sh
curl http://localhost:8080/healthz
curl -X POST http://localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' -d '{"email":"demo@example.com","password":"DemoPassword123!"}'
curl -H 'Authorization: Bearer <access_token>' 'http://localhost:8080/api/v1/projects/project-demo/report'
```

所有错误包含稳定 `code`、可读 `message`、字段错误和 `request_id`。成本金额只有 finance/admin 角色的服务端主体可以读取。

## 验证

```sh
gofmt -w $(find . -name '*.go')
go test ./...
go test -race ./...
go vet ./...
cd web && npm test -- --run && npm run typecheck && npm run build
```

基线验证结果：Go 代码通过 `go test ./...`、`go vet ./...` 和生产构建；前端测试、类型检查和构建在安装 Node 依赖后执行。限制：默认演示适配器为内存数据，完整生产写入路径使用 PostgreSQL 仓储和迁移；文件下载需要配置持久化文件根目录。
