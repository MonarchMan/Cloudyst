package tool

import (
	"ai/internal/biz/types"
	"ai/internal/data"
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type toolBiz struct {
	tc data.ToolClient
}

func (b *toolBiz) ToInvokableTool(ctx context.Context, info types.ToolInfo) (tool.InvokableTool, error) {
	// 1. 解析 Parameters JSON Schema 字符串
	var params map[string]*schema.ParameterInfo
	if err := json.Unmarshal([]byte(info.Parameters), &params); err != nil {
		return nil, fmt.Errorf("parse parameters failed: %w", err)
	}

	// 2. 构建 schema.ToolInfo（eino 的元数据描述）
	toolSchema := &schema.ToolInfo{
		Name:        info.Name,
		Desc:        info.Desc,
		ParamsOneOf: schema.NewParamsOneOfByParams(params),
	}

	// 3. 根据 Type 构建不同的可调用实现
	switch info.Type {
	case "http":
		return buildHTTPTool(toolSchema, info)
	case "builtin":
		return b.buildBuiltinTool(ctx, toolSchema, info)
	default:
		return nil, fmt.Errorf("unsupported tool type: %s", info.Type)
	}
}

func buildHTTPTool(toolSchema *schema.ToolInfo, info types.ToolInfo) (tool.InvokableTool, error) {
	return compose.NewLambdaTool(toolSchema,
		func(ctx context.Context, args map[string]any) (map[string]any, error) {
			// 根据 info.Endpoint、info.Method 动态发起 HTTP 请求
			return callHTTP(ctx, info.Endpoint, info.Method, args)
		},
	)
}

func (b *toolBiz) buildBuiltinTool(ctx context.Context, toolSchema *schema.ToolInfo, info *ToolInfo) (tool.InvokableTool, error) {
	// 根据 Name 映射到内置实现
	switch info.Name {
	case "recall_knowledge_segment":
		return b.knowledgeBiz.NewTool(ctx, toolSchema)
	default:
		return nil, fmt.Errorf("unknown builtin tool: %s", info.Name)
	}
}
