# RESTful HTTP 接口说明

## 概述

`restful.go` 是一个纯标准库实现的 RESTful HTTP 服务，对外提供用户资源的增删改查接口。

- 仅使用 Go 标准库，无需 `go.mod` 和第三方依赖
- 资源数据保存在内存 `map` 中，通过 `sync.RWMutex` 保证并发安全
- 统一返回 JSON，遵循标准 HTTP 状态码

## 启动服务

```bash
cd restfulapi
go run restful.go          # 默认监听 :8080
ADDR=:9000 go run restful.go  # 自定义端口
```

启动后终端输出：

```
REST API 正在监听端口 8080
```

## 接口列表

| 方法 | 路径 | 说明 | 成功状态码 |
|---|---|---|---|
| GET | `/healthz` | 健康检查 | 200 |
| GET | `/users` | 资源列表，支持 `?limit=2&offset=1` 分页 | 200 |
| POST | `/users` | 创建资源 | 201 |
| GET | `/users/{id}` | 查询单个资源 | 200 |
| PUT | `/users/{id}` | 全量更新 | 200 |
| PATCH | `/users/{id}` | 部分更新 | 200 |
| DELETE | `/users/{id}` | 删除资源 | 204 |

## 请求与响应示例

```bash
# 健康检查
curl http://localhost:8080/healthz
# {"status":"ok"}

# 列表（分页）
curl 'http://localhost:8080/users?limit=2&offset=0'

# 创建
curl -X POST http://localhost:8080/users \
  -H 'Content-Type: application/json' \
  -d '{"name":"Tom","email":"tom@example.com"}'
# {"id":3,"name":"Tom","email":"tom@example.com"}

# 查询单个
curl http://localhost:8080/users/3

# 部分更新
curl -X PATCH http://localhost:8080/users/3 \
  -H 'Content-Type: application/json' \
  -d '{"name":"Tom2"}'

# 全量更新
curl -X PUT http://localhost:8080/users/3 \
  -H 'Content-Type: application/json' \
  -d '{"name":"Tom3","email":"t3@example.com"}'

# 删除
curl -X DELETE http://localhost:8080/users/3
# 204 No Content
```

## 设计要点

- **统一 JSON 响应**：成功返回资源或列表，错误返回 `{"error":"..."}`。
- **正确的状态码**：创建 `201`、删除 `204`、参数或 JSON 错误 `400`、资源不存在 `404`、方法不匹配由 ServeMux 自动返回 `405`。
- **请求体限制**：`MaxBytesReader` 限制为 1MB，防止超大请求体。
- **并发安全**：内存存储用 `sync.RWMutex` 保护。
- **超时保护**：`http.Server` 设置了 `ReadHeaderTimeout: 5s`。

## 分页说明

`limit` 表示每页返回条数（默认 20），`offset` 表示跳过的记录数（默认 0）。数据先按 ID 排序再切片：

```
offset = (页码 - 1) × limit
```

例如 `?limit=2&offset=2` 返回第 3、4 条记录。当 `offset` 超出总数时返回空数组 `[]`。

## 验证记录

`go vet` 静态检查通过。实测启动输出监听提示，健康检查、创建、查询、部分更新、全量更新、删除、404 全部符合预期，测试用的临时进程与文件已清理。

## 备注

- IDE 可能对 `restful.go` 报 `main redeclared in this block`，这是因为项目根目录还有 `server.go`、`hello.go` 等其他包含 `main` 函数的文件，被当作同一个包分析。用 `go run restful.go` 单文件运行不受影响。
- 当前存储是内存实现，服务重启后数据丢失。如需持久化，可引入 SQLite、PostgreSQL 等存储，建议把存储逻辑抽象为独立接口，使 HTTP 层与存储层解耦。
