# task167-keyproof · 多方密钥轮换覆盖证明服务

安全基础设施工程师在根密钥、租户密钥和数据密钥分批轮换时，
需要证明每个受保护对象在过渡窗口内既不会失去可解密路径，也不会继续依赖已退休密钥。
本服务登记密钥层级、对象加密关系和轮换批次，对每一步计算可解密覆盖集合、
授权主体与残留引用，生成覆盖缺口或退休阻断的最短证据链；
只有每一步均满足覆盖与退休约束的计划才能批准执行。

## 业务闭环

1. 登记密钥层级（根 → 租户 → 数据）、主体、对象封装关系与授权边。
2. 创建轮换计划并追加步骤（退休密钥 / 重新封装对象 / 授权 / 撤销授权 / 移除封装）。
3. 对计划逐步骤验证：计算覆盖缺口、退休残留与退休阻断，输出最短证据链。
4. 全部步骤通过后批准并执行；支持受限回滚与重启恢复（幂等推进）。

## 实体与状态机

- 密钥：`candidate → active → retiring → retired`；`active/retiring → revoked`
- 加密对象：`protected → migrating → rewrapped`；无覆盖时 `orphaned`
- 轮换计划：`drafted → validating → blocked | executable → completed`
- 证明：`pending | sufficient | gap | residual`；执行记录：`started → applied | rolled_back | failed`

## 并发与安全约束

- 拒绝循环封装、无授权主体的根链、已退休密钥的新写入引用、跨计划交叉执行。
- 同一计划一次只推进一个步骤；重复请求按步骤幂等返回。
- 回滚仅可作用于尚未退休的密钥；已完成步骤的输入边与证明哈希不可改写（SHA-256 指纹校验）。

## 标准命令

```bash
# 构建 / 静态检查 / 单元测试
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go vet   ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go test  ./...

# 离线端到端自检（覆盖证明 / 缺口链 / 退休阻断 / 重启恢复 / 幂等）
go run ./cmd/keyproof --smoke-test

# 启动服务
go run ./cmd/keyproof --addr :8080 --db keyproof.db
```

## API 入口（前缀 /api，共 36 个）

| 分组 | 方法与路径 |
| --- | --- |
| 密钥 | `POST /api/keys`、`GET /api/keys`、`GET /api/keys/{id}`、`POST /api/keys/{id}/activate`、`POST /api/keys/{id}/retiring`、`POST /api/keys/{id}/retire`、`POST /api/keys/{id}/revoke`、`PUT /api/keys/{id}/parent` |
| 主体/授权 | `POST /api/subjects`、`GET /api/subjects`、`GET /api/subjects/{id}`、`POST /api/subjects/{id}/grants`、`DELETE /api/subjects/{id}/grants/{keyId}`、`DELETE /api/subjects/{id}` |
| 对象/封装 | `POST /api/objects`、`GET /api/objects`、`GET /api/objects/{id}`、`POST /api/objects/{id}/wrap`、`POST /api/objects/{id}/rewrap`、`GET /api/objects/{id}/coverage` |
| 计划 | `POST /api/plans`、`GET /api/plans`、`GET /api/plans/{id}`、`POST /api/plans/{id}/steps`、`POST /api/plans/{id}/validate`、`POST /api/plans/{id}/approve`、`POST /api/plans/{id}/execute`、`POST /api/plans/{id}/rollback`、`GET /api/plans/{id}/recovery` |
| 证明 | `POST /api/proofs/compute`、`GET /api/proofs`、`GET /api/proofs/{id}`、`GET /api/plans/{id}/gaps`、`GET /api/plans/{id}/residuals` |
| 执行/统计 | `GET /api/executions`、`GET /api/stats` |

## 快速示例

```bash
# 1. 登记三层密钥并启用
curl -s -XPOST localhost:8080/api/keys -d '{"name":"root","kind":"root"}'
curl -s -XPOST localhost:8080/api/keys -d '{"name":"tenant","kind":"tenant","parentId":"key-xxx"}'
curl -s -XPOST localhost:8080/api/keys -d '{"name":"data","kind":"data","parentId":"key-yyy"}'
# 2. 登记主体并授权根密钥；登记对象并封装到数据密钥
# 3. 创建计划 -> 追加步骤 -> validate -> approve -> execute
```

## 持久化

数据落盘 SQLite（`modernc.org/sqlite` 纯 Go 驱动，CGO 无关）：
密钥版本、授权边、对象封装边、计划步骤、证明快照与执行记录。
重启后重新打开数据库，`GET /api/plans/{id}/recovery` 从最后已应用步骤继续验证；
已完成步骤的输入边与证明哈希保持原样，重复执行不重复计数。
