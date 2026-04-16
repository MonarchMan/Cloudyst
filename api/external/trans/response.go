package trans

import (
	"encoding/json"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// MarshalOptions is a configurable JSON format marshaller.
var MarshalOptions = protojson.MarshalOptions{
	EmitUnpopulated: true,
	UseProtoNames:   true,
}

// Response 基础序列化器
type Response struct {
	Code          int               `json:"code"`
	Data          interface{}       `json:"data,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Msg           string            `json:"msg,omitempty"`
	Error         string            `json:"error,omitempty"`
	CorrelationID string            `json:"correlation_id,omitempty"`
}

// MarshalJSON 实现 json.Marshaler 接口
func (r *Response) MarshalJSON() ([]byte, error) {
	// 1. 准备 Data 字段的序列化结果
	var dataBytes []byte
	var err error

	if r.Data != nil {
		// encoding/json/json.go Marshal
		switch m := r.Data.(type) {
		case json.Marshaler:
			dataBytes, err = m.MarshalJSON()
		case proto.Message:
			dataBytes, err = MarshalOptions.Marshal(m)
		default:
			dataBytes, err = json.Marshal(m)
		}
		if err != nil {
			return nil, err
		}
	}

	// 2. 定义别名，避免递归调用 MarshalJSON
	type Alias Response

	// 3. 使用辅助结构体进行最终组合
	// 我们覆盖了 Data 字段，将其类型改为 json.RawMessage
	aux := &struct {
		Data json.RawMessage `json:"data,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(r), // 继承 Response 的其他字段 (Code, Msg 等)
	}

	// 只有当 dataBytes 不为空时才赋值，否则保持 nil 以触发 omitempty
	if len(dataBytes) > 0 {
		aux.Data = dataBytes
	}

	// 4. 对整体进行标准 JSON 序列化
	return json.Marshal(aux)
}
