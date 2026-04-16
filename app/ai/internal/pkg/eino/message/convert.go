package message

import (
	"ai/internal/biz/tool"
	"encoding/json"

	"github.com/cloudwego/eino/schema"
)

func ToToolInfo(t *tool.ToolInfo) *schema.ToolInfo {
	var paramInfo map[string]*schema.ParameterInfo
	json.Unmarshal([]byte(t.Parameters), &paramInfo)
	extra := make(map[string]any)
	extra["type"] = t.Type
	return &schema.ToolInfo{
		Name:        t.Name,
		Desc:        t.Desc,
		Extra:       extra,
		ParamsOneOf: schema.NewParamsOneOfByParams(paramInfo),
	}
}
