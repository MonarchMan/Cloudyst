This is a microservices-based cloud drive platform inspired by Cloudreve, built around the Go Kratos framework with a React 18 frontend.
**Architecture & Tech Stack**
The backend is written in Go 1.25 using Kratos v2, exposing dual-protocol APIs via gRPC and HTTP using Protocol Buffers. It follows Clean Architecture conventions with `biz`, `data`, and `service` layers. Dependency injection is handled by Google Wire, and Ent serves as the schema-driven ORM across all services. The frontend uses React 18, TypeScript, Vite, MUI, and Redux Toolkit.
**Core Services**The system comprises four main services:
- **Gateway:** Unified entry point with middleware for auth, CORS, logging, tracing, rate limiting, circuit breaking, and metrics.
- **File:** Core drive logic including directory listing, chunked uploads, thumbnails, sharing, trash, and WebDAV/WOPI support
- **User:** Registration, password reset, two-factor authentication, and Passkey/WebAuthn.
- **AI:** Chat roles, model management, API key handling, and a full RAG pipeline backed by the Milvus vector database.
- **Key Features:**
A standout feature is the multi-backend storage abstraction, supporting S3, OSS, COS, Qiniu, KS3, OBS, OneDrive, local, remote, and Upyun. The file service implements a “navigator” pattern that provides distinct logical views—my files, shared, trash, and shared-with-me—over the same underlying data. The inclusion of a full AI-powered knowledge base with vector search and conversational retrieval inside a personal cloud drive is also notably advanced.