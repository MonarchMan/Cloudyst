package factory

import (
	"fmt"
	"sync"

	"github.com/cloudwego/eino/components/tool"
)

type ToolConfig struct {
	Name       string `json:"name"`       // 对应 schema.ToolInfo.Name
	Desc       string `json:"desc"`       // 对应 schema.ToolInfo.Desc
	Type       string `json:"type"`       // "http" | "builtin" | "mcp" 等
	Parameters string `json:"parameters"` // JSON Schema 字符串，存 object 类型的完整 schema
}

// ToolFactory 工厂函数，用于创建可调用的工具，conf 为 nil 时，使用默认配置
type ToolFactory func(conf *ToolConfig) (tool.InvokableTool, error)

type ToolRegistry struct {
	mu        sync.RWMutex
	factories map[string]ToolFactory
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		factories: make(map[string]ToolFactory),
	}
}

func (r *ToolRegistry) Register(actionName string, factory ToolFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[actionName] = factory
}

func (r *ToolRegistry) GetFactory(actionName string) (ToolFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, exists := r.factories[actionName]
	return factory, exists
}

func (r *ToolRegistry) BuildTools(tls []*ToolConfig) ([]tool.InvokableTool, error) {
	var tools []tool.InvokableTool
	for _, tl := range tls {
		factory, exists := r.factories[tl.Name]
		if !exists {
			return nil, fmt.Errorf("tool factory not found for %s", tl.Name)
		}
		t, err := factory(tl)
		if err != nil {
			return nil, fmt.Errorf("build tool %s failed: %w", tl.Name, err)
		}
		tools = append(tools, t)
	}
	return tools, nil
}

func (r *ToolRegistry) BuildTool(tl *ToolConfig) (tool.InvokableTool, error) {
	factory, exists := r.GetFactory(tl.Name)
	if !exists {
		return nil, fmt.Errorf("tool factory not found for %s", tl.Name)
	}
	return factory(tl)
}
