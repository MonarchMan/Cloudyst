# Pkg

`internal/pkg` 是 AI 服务内部共享组件层。它不直接承载业务用例，而是给 `biz`、`data`、`server` 等层提供 Eino 组件、模型工厂、工具工厂、MCP 管理、文档解析、日志、追踪、HashID 和通用转换函数。

## 入口与基础设施

- `package.go` 定义 Wire `ProviderSet`，提供模型管理器、工具注册表、MCP 客户端管理器、HashID 编码器、Kratos logger、OpenTelemetry tracer/propagator、Ollama/OpenAI 兼容 embedder、扩展文档 parser、URL loader 和 Eino checkpoint store。
- `HasherWrapper` 使用服务配置里的 hash salt 创建 ID 编码器，供 service 层把内部整数 ID 转成对外字符串 ID。
- `LoggerWrapper` 创建带时间、调用点、trace/span 字段和日志级别过滤的 Kratos logger。
- `TracerProvider` 和 `Propagator` 初始化 OpenTelemetry OTLP exporter 和 TraceContext/Baggage 传播器。
- `ExtParser` 注册 PDF、DOCX、XLSX、HTML 和文本 fallback parser；`URLLoader` 基于 Eino URL loader 加载远程内容。

## Eino 组件

- `eino/model/` 管理 LLM 和 embedding。`manager.go` 按平台和模型配置缓存 `ToolCallingChatModel`，当前内置 OpenAI、DeepSeek、Ark、Qwen、Ollama、Qianfan、Gemini、Claude 等 factory；`embedding.go` 提供 Ollama/OpenAI 兼容 embedding helper。
- `eino/tool/factory/` 提供工具注册表和 HTTP 工具实现，可以根据工具配置构造 Eino `InvokableTool`。
- `eino/tool/search/` 封装 Bocha、Brave、Serper 三类联网搜索工具，并把搜索响应整理成可给模型消费的结果。
- `eino/tool/graphtool/` 把 Eino compose graph 包装成 invokable/streamable tool，并支持 graph checkpoint resume。
- `eino/doc/` 提供文档 loader、fetcher、URL reader、splitter、Milvus indexer 等文档处理能力。
- `eino/doc/enhance/` 是文档增强流水线，包含 normalizer、quality gate、deduper、metadata/enricher、trimmer 和 chunk 相关逻辑。
- `eino/doc/rerank/` 提供 reranker 接口和实现，包括 API reranker、Ollama reranker、score reranker。
- `eino/interrupt/` 用缓存驱动实现 Eino checkpoint store，用于可中断/可恢复的 graph。
- `eino/message/` 放置 Eino tool/schema 之间的转换辅助。
- `eino/validate/` 提供通用和实现特定的 option/validator 包装。

## Agent 实验组件

`eino/agent/` 是一套更通用的 agent runner 原型，按能力拆成多个小包：

- `budget` 控制步骤数、工具调用数、token 和超时预算。
- `router` 根据目标和能力做路由决策。
- `planner` 生成简单执行计划。
- `policy` 管理工具选择、重试和 fallback 策略。
- `runner` 串起 route、plan、invoke、observe、answer、ground 和 trace。
- `observe` 归一化工具结果并生成观察摘要。
- `memory` 聚合记忆检索结果。
- `context` 组装模型上下文。
- `citation` 从工具结果、观察和记忆生成引用块。
- `verify` 做回答 grounding 校验。
- `trace` 记录 agent 执行事件。

## MCP 与工具函数

- `mcp/manager.go` 维护命名 MCP client，支持注册已有 client 和动态创建远程 SSE MCP client。
- `utils/converter.go` 在 Ent 实体和 proto message 之间做转换，覆盖 API Key、模型、工具、会话、消息、网页、角色、图片、知识库、文档、分段和任务。
- `utils/common.go`、`utils/token.go` 提供字符串到整数切片、map 转换和 token 计数等小工具。
- `utils/callbacks/` 提供 Eino callback handler 的组合辅助。

## 使用边界

这一层适合放“可复用技术组件”。如果逻辑依赖用户权限、知识库状态、任务状态或聊天语义，应放在 `internal/biz`；如果逻辑直接读写 Ent/RPC/Milvus，应放在 `internal/data`。
