# Service

`internal/service` 是 AI 服务的传输层，负责实现 proto 生成的 gRPC/HTTP service 接口。这里的代码主要做请求参数转换、HashID 解码、当前用户读取、基础校验、调用 `biz` 用例、把 Ent/domain 对象组装成 proto 响应，以及处理聊天流式输出。

## 入口与注册关系

- `service.go` 定义 Wire `ProviderSet`，当前注册 `AdminService`、`ChatService`、`KnowledgeService`、`ImageService`、`RoleService`。
- `internal/server/grpc.go` 和 `internal/server/http.go` 当前也注册 admin、chat、knowledge、image、role 这五类服务。
- `model.go` 和 `workflow.go` 中已有 `ModelService`、`WorkflowService` 实现；如果要对外启用，需要同步补充 provider 和 server 注册。

## Service 文件职责

- `admin.go` 实现后台管理接口，覆盖 API Key、模型、角色、知识库、文档、分段、图片、聊天会话、聊天消息、工具和任务/队列查询。它把后台通用列表条件转换成 data 层筛选参数，并复用 `biz` 和部分 data client 完成操作。
- `chat.go` 实现聊天接口，包括会话创建/更新/删除/列表、消息列表/删除、发送消息、重试消息、补丁消息，以及 HTTP/gRPC 流式聊天。它负责解码会话/消息/模型/角色 ID，准备模型和 RAG 检索工具，并通过 chunk callback 推送流式响应。
- `knowledge.go` 实现知识库用户接口，包括知识库 CRUD、统计、文档创建/批量创建/更新/删除/列表、文档进度、重建索引、检索、复制文档、创建/获取用户主知识库、变更文档归属和查询支持解析类型。
- `image.go` 实现图片查询、列表和删除接口，主要围绕图片 ID 校验、状态过滤和响应组装。
- `role.go` 实现聊天角色接口，负责角色 CRUD、当前用户角色列表，以及角色关联知识库/工具的 ID 校验。
- `model.go` 实现模型查询接口，包括模型列表、模型详情和默认模型查询。
- `workflow.go` 实现任务工作流接口，包括任务列表、任务详情、阶段进度、取消任务和恢复任务。
- `response.go` 集中放响应构造函数，把 Ent 实体、业务类型和任务状态转换为 proto 响应，并统一处理 HashID 编码、时间字段、引用分段、网页搜索结果、任务进度等输出结构。

## 请求处理约定

- 对外 ID 通常是 HashID 字符串，service 层通过 `validateID` 转成内部整数 ID；data/biz 层继续使用整数 ID。
- 当前用户通过 `api/external/trans.FromContext(ctx)` 获取，用户归属和管理员权限校验分布在 service 与 biz 中。
- 数据库和参数错误通过 `api/api/common/v1` 的错误构造函数包装，保持 HTTP/gRPC 响应格式一致。
- 聊天流式接口同时支持 gRPC stream 和额外 HTTP route：`/chat/ai/chat/message/send-stream`。

## 调用链

典型调用链是 `proto/HTTP request -> service -> biz -> data/pkg -> service response builder -> proto response`。这一层应尽量保持薄，只做传输协议相关工作；业务流程、RAG 编排和持久化细节分别放在 `biz` 和 `data/pkg`。
