# 审批 API 说明

## 概述

`restful.go` 是一个纯标准库实现的两级审批系统 HTTP 服务。

- 仅使用 Go 标准库，无需 `go.mod` 和第三方依赖
- 数据保存在内存 `map` 中，通过 `sync.RWMutex` 保证并发安全（重启后数据重置）
- 固定两级审批节点：**需求owner → 需求owner上级**
- 审批人固定在审批单上（新建时填写），审批时无需再指定
- 任一级驳回后退回申请人，可修改后重新提交，从第一节点重新开始

## 审批单字段

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | int | 审批单 ID |
| `title` | string | 申请标题 |
| `applicant` | string | 申请人 |
| `reason` | string | 申请事由 |
| `owner` | string | 需求owner（第一节点审批人） |
| `owner_manager` | string | 需求owner上级（第二节点审批人） |
| `status` | string | 整体状态：`pending` / `approved` / `returned` |
| `current_step` | int | 当前待审批节点：0=需求owner，1=需求owner上级，2=已完成 |
| `steps` | array | 两个节点的审批记录（见下） |
| `last_reject_reason` | string | 最近一次驳回意见，供申请人查看 |
| `created_at` / `updated_at` | string | 创建/更新时间（RFC3339） |

`steps` 中每个节点：

| 字段 | 说明 |
|---|---|
| `name` | 节点名称：`需求owner` / `需求owner上级` |
| `approver` | 该节点审批人（来自 owner / owner_manager） |
| `status` | `pending` / `approved` / `rejected` |
| `remark` | 该节点的审批意见 |
| `updated_at` | 该节点处理时间 |

## 状态流转

```
新建 → pending(current_step=0)
  ├─ owner 通过 → pending(current_step=1)
  │    ├─ 上级通过 → approved(current_step=2)  完成
  │    └─ 上级驳回 → returned（退回申请人）
  └─ owner 驳回 → returned（退回申请人）

returned → 申请人修改(PATCH) → 重新提交(POST /resubmit) → pending(current_step=0) 重新开始
```

## 启动服务

```bash
cd restfulapi
go run restful.go          # 默认监听 :8080
ADDR=:9000 go run restful.go  # 自定义端口
```

## 接口列表

| 方法 | 路径 | 说明 | 成功状态码 |
|---|---|---|---|
| GET | `/healthz` | 健康检查 | 200 |
| GET | `/approvals` | 列表，支持 `?status=pending&limit=20&offset=0` | 200 |
| POST | `/approvals` | 新建申请 | 201 |
| GET | `/approvals/{id}` | 详情 | 200 |
| PATCH | `/approvals/{id}` | 修改申请（仅 `returned` 可改） | 200 |
| POST | `/approvals/{id}/resubmit` | 重新提交（仅 `returned` 可用） | 200 |
| POST | `/approvals/{id}/approve` | 通过当前节点 | 200 |
| POST | `/approvals/{id}/reject` | 驳回当前节点（退回申请人） | 200 |
| DELETE | `/approvals/{id}` | 删除 | 204 |

## 请求与响应示例

```bash
# 新建申请（必须填写 owner 与 owner_manager）
curl -X POST http://localhost:8080/approvals \
  -H 'Content-Type: application/json' \
  -d '{"title":"采购显示器","applicant":"赵六","reason":"开发需要","owner":"李四","owner_manager":"王五"}'

# 列表（只看审批中的）
curl 'http://localhost:8080/approvals?status=pending'

# 第一节点：需求owner通过（审批人自动取 owner，意见可留空）
curl -X POST http://localhost:8080/approvals/4/approve \
  -H 'Content-Type: application/json' -d '{"remark":"同意采购"}'
# current_step 变为 1，等待需求owner上级

# 第二节点：上级驳回
curl -X POST http://localhost:8080/approvals/4/reject \
  -H 'Content-Type: application/json' -d '{"remark":"预算不足"}'
# status=returned，last_reject_reason="预算不足"，节点重置

# 申请人修改后重新提交
curl -X PATCH http://localhost:8080/approvals/4 \
  -H 'Content-Type: application/json' -d '{"title":"采购显示器-精简版"}'
curl -X POST http://localhost:8080/approvals/4/resubmit
# status=pending，current_step=0，从第一节点重新开始

# 删除
curl -X DELETE http://localhost:8080/approvals/4
```

## 状态码与错误

- `200` 查询/更新/审批/重提成功；`201` 创建成功；`204` 删除成功
- `400` 参数错误或 JSON 非法（必填字段为空、`status` 过滤值非法）
- `404` 审批单不存在
- `409` 业务冲突：非 pending 状态执行审批、审批中执行修改、已通过执行修改、非 returned 执行重提、重复审批

错误统一格式：`{"error": "当前状态不能执行该操作"}`

## 业务规则

1. 新建时状态固定为 `pending`，审批人固定为填写的 `owner` / `owner_manager`。
2. 只有 `pending` 状态且对应节点才能审批；owner 通过后流转到上级，上级通过后整体 `approved`。
3. 任一级驳回即退回申请人（状态 `returned`），两个节点重置，并记录 `last_reject_reason`。
4. 只有 `returned` 状态可以修改和重新提交；重新提交清空驳回意见，从第一节点重新开始。
5. 列表按 ID 倒序返回，支持按 `status` 精确过滤。

## 验证记录

`go vet` 通过。实测完整流程：新建 → owner 通过（step 0→1）→ 审批中修改 409 → 上级驳回（returned）→ 申请人修改 → 重新提交（回到 step 0）→ owner 通过 → 上级通过（approved）→ 重复审批 409，全部符合预期。
