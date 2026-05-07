# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

Behavioral guidelines to reduce common LLM coding mistakes. Merge with project-specific instructions as needed.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.


This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Project Overview

This is a Go microservices monorepo for a cloud drive platform with AI capabilities. It uses the Go Kratos v2 framework, Protocol Buffers for APIs, Ent as the ORM, and Google Wire for dependency injection.

## Repository Structure

This is a Go workspace (`go.work`) with the following modules:

- `api/` — Shared Protocol Buffer definitions and generated Go code for all services.
- `pkg/` — Shared library code (Go module name: `common`). Contains cache, auth, constants, middleware, utilities.
- `module/entmodule/` — Shared Ent code generation configuration and helpers.
- `module/copiermodule/` — Shared copy utilities.
- `app/ai/` — AI service: chat, roles, model management, image generation, RAG knowledge base with Milvus vector DB.
- `app/file/` — File service: storage, uploads, thumbnails, sharing, trash, WebDAV/WOPI, multi-backend storage drivers.
- `app/user/` — User service: auth, registration, 2FA, Passkey/WebAuthn.
- `app/gateway/` — API gateway: unified entry point with routing, auth, rate limiting, circuit breaking, CORS, tracing.
- `app/audit/` — Audit logging service.
- `app/media/` — Media processing service.

Each service under `app/<service>/` is an independent Go module with its own `go.mod` and `Makefile`.

## Build & Development Commands

### Working in a Service

Most work happens inside a specific service directory, e.g., `app/ai/` or `app/file/`. Each service has its own Makefile.

```bash
cd app/ai

# Build the service binary to ./bin/
make build

# Generate API code from proto files (pb.go, grpc, http, openapi, validate)
make api

# Generate config proto code
make config

# Run all generators (api, config, go generate, tidy)
make all

# Generate Wire dependency injection code
cd app/cmd/ai && wire
```

### Proto / API Generation

Proto files are defined in the top-level `api/` module, organized by service:
- `api/api/ai/<domain>/v1/*.proto`
- `api/api/file/<domain>/v1/*.proto`
- `api/api/user/<domain>/v1/*.proto`
- `api/api/common/v1/common.proto`

The `api/` module has its own Makefile for generating external proto files. Service-level Makefiles generate service-specific proto code.

To regenerate APIs across the repo, run `make api` in both `api/` and the target service directory.

### Ent (ORM) Generation

Each service that uses Ent has an `ent/` directory and `ent/schema/` for entity definitions.

```bash
cd app/ai
# Regenerate Ent code from schemas
go generate ./ent/...
```

This executes `entc.go`, which uses the shared `module/entmodule` configuration.

### Running a Service

Services are typically started from their `cmd/<service>/` directory:

```bash
cd app/ai/app/cmd/ai
go run .
```

Most services expect a config path flag:
```bash
go run . -conf ../../../configs/config.yaml
```

Some services also expect a Consul address for service discovery and config loading:
```bash
go run . -conf ../../../configs -consul 127.0.0.1:8500
```

### Tests

There is no centralized test command. Run tests per module or package:

```bash
# Run all tests in the AI service
cd app/ai && go test ./...

# Run a specific test
cd app/ai && go test ./internal/pkg/eino/model/...
```

Tests are sparse; most validation is integration-level through the running services.

## Architecture Patterns

### Clean Architecture (Kratos Standard)

Each service follows Kratos layering:

- `internal/service/` — gRPC/HTTP handlers. Translates transport concerns to business logic. Files usually correspond to proto services (e.g., `chat.go`, `admin.go`).
- `internal/biz/` — Business logic / use cases. Contains domain-specific packages (e.g., `biz/chat/`, `biz/knowledge/`).
- `internal/data/` — Data access layer. Implements repository interfaces defined in `biz/`.
  - `data/*.go` — Ent-based repository implementations for local entities.
  - `data/rpc/*.go` — gRPC clients for calling other services (e.g., `data/rpc/user.go`, `data/rpc/file.go`).
  - `data/vector/*.go` — Vector DB clients (Milvus in the AI service).
- `internal/server/` — Wire up HTTP and gRPC servers, register middleware, and attach service handlers.
- `internal/conf/` — Configuration structs generated from proto.
- `app/cmd/<service>/` — Application entry point (`main.go`, `wire.go`, `wire_gen.go`).

### Dependency Injection with Wire

Each service uses Google Wire. The provider sets are defined in each layer's root file:
- `internal/biz/biz.go`
- `internal/data/data.go`
- `internal/service/service.go`
- `internal/server/server.go`
- `internal/pkg/pkg.go`

The `wire.go` file in `app/cmd/<service>/` assembles them into a `*kratos.App`. After changing provider sets, regenerate with `wire` in the `cmd/<service>/` directory.

**Note:** The `app/ai` service's `main.go` is currently commented out and may not be runnable as-is. Check if the active entry point is elsewhere or if it needs to be restored.

### File Service Lifecycle

The `file` service is architecturally different: it has an `app/app.go` that defines a custom `Server` interface with `Start()`, `PrintBanner()`, and `Close()`. The `Start()` method initializes background workers:
- Queue managers (media meta, entity recycle, I/O intense, remote download, thumbnail)
- Cron jobs (OAuth credential refresh)
- Mime manager and extractor state reload

The file service also supports a master/slave mode determined by `config.Server.Sys.Mode`.

### Gateway Architecture

The gateway (`app/gateway/`) is not a typical Kratos service. It is a reverse proxy that:
- Loads configuration from YAML (and optionally a control service).
- Builds a routing table from config endpoints to backend services.
- Applies middleware chains per route (auth, CORS, rate limiting, circuit breaking, logging, tracing, transcoding).
- Uses Consul for service discovery to resolve backend addresses.
- Supports hot config reloading via file watchers.

Gateway middlewares are registered in `cmd/gateway/main.go` via blank imports and created by name in `middleware/registry.go`.

### Inter-Service Communication

Services communicate via gRPC using proto-generated clients. RPC clients live in the caller's `internal/data/rpc/` directory and are injected via Wire. The gateway translates HTTP requests to gRPC for internal routing.

### Database & Storage

- **Ent ORM**: Schema-driven, with generated code under `ent/`. The `module/entmodule` package provides shared utilities for enum mapping between Proto and Ent, and SQL driver setup.
- **Supported DBs**: SQLite (default), MySQL, PostgreSQL.
- **Cache**: Abstraction in `pkg/cache/` supporting in-memory (memo store) and Redis.
- **Vector DB**: The AI service uses Milvus for RAG retrieval and indexing.

### Multi-Backend Storage (File Service)

The file service abstracts multiple storage backends: S3, OSS, COS, Qiniu, KS3, OBS, OneDrive, local, remote, and Upyun. Storage policies determine which backend a file uses. The "navigator" pattern provides logical views (my files, shared, trash, shared-with-me) over the same data.

### AI Service (Eino Framework)

The AI service uses the CloudWeGo Eino framework for LLM orchestration:
- `internal/pkg/eino/model/` — Model clients for various providers (OpenAI, Codex, DeepSeek, Gemini, Ollama, Qwen, etc.).
- `internal/pkg/eino/tool/` — Tool integrations (search, MCP).
- `internal/biz/knowledge/rag/` — RAG pipeline: ingestion (`ingestion/`) and retrieval (`retrieval/`).
- `internal/data/vector/milvus.go` — Milvus indexer and retriever.

## Important Conventions

- **Proto imports**: Use `api/api/<service>/<domain>/v1/` paths. The `go_package` option uses module-relative paths because `api/` is a separate Go module.
- **Response encoding**: Services customize HTTP responses. The AI service uses `app/ai/app/response/` for response and error encoders. The File service uses `file/app/response/`.
- **Middleware location**: Shared transport middleware lives in `api/external/middlewares/`. Gateway-specific middleware lives in `gateway/middleware/`.
- **Constants & versions**: Shared constants (including `BackendVersion`) are in `pkg/constants/`.
