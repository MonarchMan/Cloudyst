# Biz

`internal/biz` 是 AI 服务的业务用例层。它位于 `service` 和 `data` 之间，负责会话、模型、图片、知识库、RAG、队列任务等业务编排；数据库、RPC、Milvus 和 Eino 组件都通过接口注入进来。

## 入口与依赖注入

- `biz.go` 定义 Wire `ProviderSet`，当前向上层提供 `ChatBiz`、`KnowledgeBiz`、`ImageBiz`、`ModelBiz`、RAG ingestion/retrieval 引擎和 Milvus indexer。
- 业务代码大多以接口暴露能力，以私有 struct 持有 repository、模型管理器、向量库、工具注册表、配置和日志。

## 核心业务包

- `chat/` 负责聊天会话和消息用例。它会校验会话、角色、模型和用户归属，组装历史上下文、角色系统提示、附件图片、知识库召回结果、联网搜索结果和工具列表，并基于 Eino graph 执行普通生成与流式生成。
- `model/` 负责模型和 API Key 用例。它从数据层取激活模型，再通过 `internal/pkg/eino/model` 的模型管理器创建或复用 Eino `ToolCallingChatModel`；也处理后台模型/API Key 的增删改查。
- `image/` 负责图片记录的查询、更新和删除，主要是数据层图片 repository 的业务包装。
- `knowledge/` 负责知识库、文档和分段的生命周期。它创建/更新知识库文档时生成索引或重建索引任务，删除文档/分段时同步清理向量，支持知识库统计、检索召回、文档复制和文档归属变更。
- `workflow/` 负责异步任务的用户侧查询、取消、恢复和进度读取。它会结合当前用户权限过滤任务，并用缓存记录一段时间内可恢复的任务 ID。
- `queue/` 定义 AI 模块自己的队列管理器和 DB-backed task 包装，当前主要管理知识库 ingest/reindex 两类队列。
- `tool/` 是工具业务的雏形，当前可以查询工具并遍历激活工具用于注册。
- `types/` 放置业务层共享类型和枚举，例如知识分段、RAG 检索参数、聊天内部状态、图片状态、模型类型、文档处理进度、Milvus 字段名和支持的文本解析类型。

## RAG 相关包

- `knowledge/rag/ingestion/` 构建文档入库 graph：从文件服务加载远程文档，按 MIME/扩展名解析，按文档策略选择 markdown/semantic/recursive splitter，可选执行文档增强，然后写入 Milvus 并回写分段数据。该流程支持 checkpoint 和 interrupt，用于长任务暂停/恢复。
- `knowledge/rag/retrieval/` 构建检索链：默认使用 Milvus retriever 加 score reranker，也可以走 RAGGraph 路径；同时把检索能力注册为工具，供聊天工具调用。
- `knowledge/rag/raggraph/` 实现更完整的 RAG graph：准备请求、查询改写、多查询召回、融合、邻近分段扩展、重排、上下文压缩、引用组装、fallback、回答生成、答案验证、评估和 trace。
- `knowledge/rag/task/` 定义知识库 ingest/reindex 任务，`retrieve.go` 当前只是包占位。任务状态会序列化到数据库，执行时从上下文取引擎和 repository 依赖，并维护分阶段进度、失败计数、checkpoint 和 interrupt 信息。

## 典型调用链

- 聊天：`ChatService` 解码请求和 ID 后调用 `ChatBiz`，`ChatBiz` 组装输入、构建工具/RAG/搜索上下文，调用模型生成并保存消息。
- 知识库入库：`KnowledgeService` 创建文档后调用 `KnowledgeBiz`，`KnowledgeBiz` 创建 ingest 任务，任务执行时由 `IngestEngine` 完成加载、切分、索引和统计回写。
- 知识库检索：聊天或知识库搜索调用 `RetrieveEngine`，先查 Milvus，再按向量 ID 回查业务分段并更新检索次数。
