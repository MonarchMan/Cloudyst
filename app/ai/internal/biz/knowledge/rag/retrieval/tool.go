package retrieval

import (
	"ai/internal/biz/types"
	"ai/internal/pkg/eino/tool/factory"
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

const (
	defaultToolName           = "kb_retrieval"
	defaultToolDesc           = "当问题涉及公司内部文档、产品手册等私有知识时使用"
	defaultGraphRAGToolName   = "kb_graphrag_retrieval"
	defaultGraphRAGToolDesc   = "Use the experimental GraphRAG retrieval path with neighbor chunk expansion."
	RetrievalToolName         = "knowledge_retrieval"
	GraphRAGRetrievalToolName = "knowledge_graphrag_retrieval"
)

type Config struct {
	ToolName string
	ToolDesc string
}

func (e *RetrieveEngine) registerTools() {
	e.tr.Register(RetrievalToolName, e.retrieveTool)
	e.tr.Register(GraphRAGRetrievalToolName, e.graphRAGRetrieveTool)
}

func (e *RetrieveEngine) retrieveTool(conf *factory.ToolConfig) (tool.InvokableTool, error) {
	if conf == nil {
		conf = &factory.ToolConfig{}
	}

	if conf.Name == "" {
		conf.Name = defaultToolName
	}
	if conf.Desc == "" {
		conf.Desc = defaultToolDesc
	}
	tl, err := utils.InferTool(
		conf.Name,
		conf.Desc,
		e.Retrieve,
		utils.WithMarshalOutput(e.marshalOutput),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to infer retrieval tool: %w", err)
	}
	return tl, nil
}

func (e *RetrieveEngine) graphRAGRetrieveTool(conf *factory.ToolConfig) (tool.InvokableTool, error) {
	if conf == nil {
		conf = &factory.ToolConfig{}
	}

	if conf.Name == "" {
		conf.Name = defaultGraphRAGToolName
	}
	if conf.Desc == "" {
		conf.Desc = defaultGraphRAGToolDesc
	}
	tl, err := utils.InferTool(
		conf.Name,
		conf.Desc,
		e.RetrieveGraphRAG,
		utils.WithMarshalOutput(e.marshalOutput),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to infer graphrag retrieval tool: %w", err)
	}
	return tl, nil
}

func (e *RetrieveEngine) RetrieveGraphRAG(ctx context.Context, args *types.SegmentSearchArgs) ([]*types.KnowledgeSegment, error) {
	if args == nil {
		return nil, fmt.Errorf("segment search args is nil")
	}
	next := *args
	next.UseGraphRAG = true
	return e.Retrieve(ctx, &next)
}

func (e *RetrieveEngine) marshalOutput(_ context.Context, output any) (string, error) {
	resp, ok := output.([]*types.KnowledgeSegment)
	if !ok {
		return "", fmt.Errorf("bocha: unexpected output type, want []*types.KnowledgeSegment but got %T", output)
	}

	bs, err := json.Marshal(resp)
	if err != nil {
		return "", fmt.Errorf("knowledgeBiz: marshal []*types.KnowledgeSegment failed: %w", err)
	}

	return string(bs), nil
}
