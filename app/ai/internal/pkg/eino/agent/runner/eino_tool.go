package runner

import (
	"context"
	"encoding/json"
	"fmt"

	einotool "github.com/cloudwego/eino/components/tool"
)

type ToolArgumentsBuilder interface {
	BuildArguments(ctx context.Context, toolName string, input *ToolInput) (string, error)
}

type ToolArgumentsBuilderFunc func(ctx context.Context, toolName string, input *ToolInput) (string, error)

func (f ToolArgumentsBuilderFunc) BuildArguments(ctx context.Context, toolName string, input *ToolInput) (string, error) {
	return f(ctx, toolName, input)
}

type QueryArgumentsBuilder struct {
	Field string
}

func (b QueryArgumentsBuilder) BuildArguments(ctx context.Context, toolName string, input *ToolInput) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	field := b.Field
	if field == "" {
		field = "query"
	}
	query := ""
	if input != nil {
		query = input.Query
	}
	bs, err := json.Marshal(map[string]string{field: query})
	if err != nil {
		return "", fmt.Errorf("build tool arguments: %w", err)
	}
	return string(bs), nil
}

type EinoToolInvoker struct {
	Tools            map[string]einotool.InvokableTool
	ArgumentsBuilder ToolArgumentsBuilder
}

func NewEinoToolInvoker(ctx context.Context, tools ...einotool.InvokableTool) (*EinoToolInvoker, error) {
	invoker := &EinoToolInvoker{
		Tools:            map[string]einotool.InvokableTool{},
		ArgumentsBuilder: QueryArgumentsBuilder{},
	}
	for _, t := range tools {
		if t == nil {
			continue
		}
		info, err := t.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("inspect eino tool: %w", err)
		}
		if info == nil || info.Name == "" {
			return nil, fmt.Errorf("eino tool name is empty")
		}
		invoker.Tools[info.Name] = t
	}
	return invoker, nil
}

func NewEinoToolInvokerFromMap(tools map[string]einotool.InvokableTool) *EinoToolInvoker {
	cp := make(map[string]einotool.InvokableTool, len(tools))
	for name, t := range tools {
		if name != "" && t != nil {
			cp[name] = t
		}
	}
	return &EinoToolInvoker{
		Tools:            cp,
		ArgumentsBuilder: QueryArgumentsBuilder{},
	}
}

func (i *EinoToolInvoker) Invoke(ctx context.Context, toolName string, input *ToolInput) (any, error) {
	if i == nil {
		return nil, fmt.Errorf("eino tool invoker is nil")
	}
	t, ok := i.Tools[toolName]
	if !ok || t == nil {
		return nil, fmt.Errorf("eino tool %q not found", toolName)
	}
	builder := i.ArgumentsBuilder
	if builder == nil {
		builder = QueryArgumentsBuilder{}
	}
	args, err := builder.BuildArguments(ctx, toolName, input)
	if err != nil {
		return nil, err
	}
	return t.InvokableRun(ctx, args)
}
