# Data

`internal/data` 是 AI 服务的数据访问层，负责把业务层需要的持久化能力封装成接口。这里主要处理 Ent 数据库访问、缓存驱动选择、跨服务 RPC 客户端、Milvus 向量库访问，以及事务辅助方法。

## 入口与依赖注入

- `data.go` 定义 Wire `ProviderSet`，向上层提供模型、知识库、文档、分段、角色、会话、消息、图片、网页、工具、Milvus、文件服务 RPC、用户服务 RPC、设置服务 RPC、数据库客户端、缓存和数据库类型。
- `client.go` 根据配置创建 Ent 客户端，支持 SQLite/MySQL/PostgreSQL 这类由共享 `common.DBType` 描述的数据库类型，并包装 SQLite 驱动。
- `migration.go` 通过用户服务的设置接口记录/判断数据库版本，配合 Ent schema 做迁移。
- `tx.go` 提供 `WithTx`、`InheritTx`、`Commit`、`Rollback`，让各 repository 可以在同一事务上下文里切换 Ent client。

## 本地 Repository

- `model.go` 管理 `AiModel` 和 `AiApiKey`，包含列表筛选、默认模型、按类型获取激活模型、API Key 增删改查等能力。
- `chat_conversation.go`、`chat_message.go`、`chat_role.go` 管理聊天会话、消息和角色。它们负责用户维度查询、分页排序、批量删除、角色关联知识库/工具等基础数据操作。
- `knowledge.go` 管理知识库主体，包含公开状态、用户主知识库、知识库删除时关联文档/分段的清理入口。
- `knowledge_document.go` 管理知识库文档，包含文档状态、索引进度、内容长度/版本/索引统计、可索引文档扫描和检索次数更新。
- `knowledge_segment.go` 管理知识分段，包含按文档/分段索引查询、邻近分段查询、向量 ID 回写、按向量 ID 回查业务分段和检索次数统计。
- `image.go` 管理 AI 图片记录，支持按平台/模型/状态筛选、更新状态和批量删除。
- `tool.go` 管理可调用工具配置，支持启用状态筛选、分页和批量删除。
- `web_page.go` 持久化联网搜索结果，可按消息 ID 查询并关联到聊天消息。
- `task.go` 把 Ent `Task` 适配为共享 `queue.TaskRecord`，并提供任务创建、列表、取消、恢复、清理和完成标记。

## 外部系统访问

- `rpc/file.go` 封装文件服务调用，用于获取文件信息、下载 URL、列目录、创建文件和写入内容。
- `rpc/user.go` 封装用户服务和管理设置服务调用，用于查询用户信息、批量用户信息，以及读写系统设置。
- `vector/milvus.go` 封装 Milvus 访问，提供按文档删除向量、批量删除、统计文档分段数、向量检索、按向量 ID 取内容，以及集合初始化/统计等能力。

## 数据流位置

典型调用链是 `service -> biz -> data`。`data` 层只返回 Ent 实体、RPC 响应或向量库结果，不处理用户请求语义；权限、业务校验、任务编排和 RAG 流程放在 `biz` 层。
