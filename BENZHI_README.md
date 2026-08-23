# task167-keyproof 评测说明（BENZHI）

多方密钥轮换覆盖证明服务：登记密钥层级/对象关系，创建轮换计划，
逐步骤验证覆盖与退休约束，生成最短证据链；批准后执行，支持受限回滚与重启恢复。

## 评测命令（必须真成功，禁止掩盖失败）

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go vet   ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go test  ./...
go run ./cmd/keyproof --smoke-test
```

`--smoke-test` 契约：不启动长驻服务，使用独立临时库执行端到端自检
（覆盖建立、最短缺口链、退休阻断、重新封装、退休密钥新引用拒绝、
重启恢复、幂等执行、证明保持），全部通过后以 **0 退出码**结束。

## Docker 双架构验证

```bash
bash build_benzhi_docker.sh my-project linux/amd64
docker run --rm --platform linux/amd64 my-project --smoke-test
bash build_benzhi_docker.sh my-project linux/arm64
docker run --rm --platform linux/arm64 my-project --smoke-test
```

镜像内入口 `/app/keyproof`，默认 `CMD ["--smoke-test"]`。

## 环境与版本锁

- Go 1.26.3（`GOTOOLCHAIN=local`）、`CGO_ENABLED=0`
- SQLite 3.46.1（`modernc.org/sqlite` v1.52.0，纯 Go，离线可构建）
- 版本锁见 `component-versions.json`

## API 摘要（前缀 /api，共 36 个）

- 密钥：登记/列表/详情/启用/退役待清理/退休/吊销/换父
- 主体与授权：登记/列表/详情/移除/新增授权/撤销授权
- 对象与封装：登记/列表/详情/封装/重新封装/覆盖查询
- 计划：创建/列表/详情/追加步骤/验证/批准/执行/回滚/恢复
- 证明：计算/列表/详情/缺口证据链/退休残留
- 执行与统计：执行记录列表/汇总统计

## 目录结构

```
cmd/keyproof/          入口（--addr / --db / --smoke-test）
internal/model/        实体与状态机、错误码
internal/store/        SQLite 持久化（建表迁移 + 各仓库）
internal/relation/     关系模块：层级/授权/封装边与约束
internal/proof/        证明模块：覆盖/缺口链/残留/指纹
internal/plan/         计划模块：状态机/单步验证/批准
internal/execution/    执行模块：推进/受限回滚/重启恢复
internal/service/      编排层
internal/httpapi/      HTTP 层
internal/smoke/        离线端到端自检
```
