package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

type (
	MCPClientManager interface {
		GetClient(name string) (client.MCPClient, error)
		Register(name string, client client.MCPClient)
		AddRemoteSSEClient(ctx context.Context, name string, sseURL string) error
	}

	mcpClientManager struct {
		mu      sync.RWMutex
		clients map[string]client.MCPClient
	}
)

func (m *mcpClientManager) Register(name string, client client.MCPClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[name] = client
}

func (m *mcpClientManager) GetClient(name string) (client.MCPClient, error) {
	c, ok := m.clients[name]
	if !ok {
		return nil, fmt.Errorf("mcp client [%s] not found", name)
	}
	return c, nil
}

// AddRemoteSSEClient 动态建立远端 SSE 连接并注册
func (m *mcpClientManager) AddRemoteSSEClient(ctx context.Context, name string, sseURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. 检查是否已经存在
	if _, exists := m.clients[name]; exists {
		return fmt.Errorf("mcp client [%s] already exists", name)
	}

	// 2. 初始化 SSE 客户端
	// 注意：这里可能需要根据远端要求传入 HTTP Client (处理鉴权 Header 等)
	c, err := client.NewSSEMCPClient(sseURL)
	if err != nil {
		return fmt.Errorf("failed to create sse mcp client: %w", err)
	}

	// 3. 执行 MCP 协议的强制握手流程 (启动 Client)
	err = c.Start(ctx)
	if err != nil {
		return fmt.Errorf("failed to start mcp client: %w", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "my-kratos-ai-module",
		Version: "1.0.0",
	}

	_, err = c.Initialize(ctx, initReq)
	if err != nil {
		return fmt.Errorf("mcp handshake failed: %w", err)
	}

	// 4. 握手成功，存入内存
	m.clients[name] = c
	return nil
}
