package mapper

import (
	"time"

	"github.com/jinzhu/copier"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type ConvertType string

const (
	ConvertTypeString  ConvertType = "string"
	ConvertTypeInt64   ConvertType = "int64"
	ConvertTypeInt32   ConvertType = "int32"
	ConvertTypeFloat64 ConvertType = "float64"
	ConvertTypeFloat32 ConvertType = "float32"
	ConvertTypeBool    ConvertType = "bool"
	ConvertTypeBytes   ConvertType = "bytes"
	ConvertTypeTime    ConvertType = "time"
)

// BuildEntToWrapperPointer
// 泛型构建器：Ent 指针 -> Proto Wrapper
// T: Go 基础类型 (如 string, int64)
// W: Proto Wrapper 结构体 (如 wrapperspb.StringValue)
func BuildEntToWrapperPointer[T any, W any](constructor func(T) *W) copier.TypeConverter {
	return copier.TypeConverter{
		SrcType: (*T)(nil), // 自动推导源类型指针，如 *string
		DstType: (*W)(nil), // 自动推导目标类型指针，如 *wrapperspb.StringValue
		Fn: func(src interface{}) (interface{}, error) {
			val, ok := src.(*T)
			if !ok || val == nil {
				return nil, nil // 处理 nil 的情况
			}
			// 调用传入的构造函数，如 wrapperspb.String(*val)
			return constructor(*val), nil
		},
	}
}

// BuildWrapperToEntPointer
// 泛型构建器：Proto Wrapper -> Ent 指针
func BuildWrapperToEntPointer[T any, W any](extractor func(*W) T) copier.TypeConverter {
	return copier.TypeConverter{
		SrcType: (*W)(nil),
		DstType: (*T)(nil),
		Fn: func(src interface{}) (interface{}, error) {
			w, ok := src.(*W)
			if !ok || w == nil {
				return nil, nil
			}
			// 调用传入的提取函数获取底层的 Value
			v := extractor(w)
			return &v, nil
		},
	}
}

// BuildEnumToProto
// 泛型构建器：Ent 枚举 (基于 string) -> Proto 枚举 (基于 int32)
// E: Ent 中的枚举类型 (如 enums.Status)
// P: Proto 中的枚举类型 (如 pb.Document_Status)
func BuildEnumToProto[E ~string, P ~int32](mapping map[E]int32, fallback P) copier.TypeConverter {
	var zeroE E // 获取 E 类型的零值，用于让 copier 识别类型
	var zeroP P

	return copier.TypeConverter{
		SrcType: zeroE,
		DstType: zeroP,
		Fn: func(src interface{}) (interface{}, error) {
			val, ok := src.(E)
			if !ok {
				return fallback, nil
			}
			// 查字典，找到了就返回对应的 Proto 数字，找不到就用默认值
			if p, exists := mapping[val]; exists {
				return P(p), nil
			}
			return fallback, nil
		},
	}
}

// BuildProtoToEnum
// 泛型构建器：Proto 枚举 (基于 int32) -> Ent 枚举 (基于 string)
func BuildProtoToEnum[P ~int32, E ~string](mapping map[int32]E, fallback E) copier.TypeConverter {
	var zeroP P
	var zeroE E

	return copier.TypeConverter{
		SrcType: zeroP,
		DstType: zeroE,
		Fn: func(src interface{}) (interface{}, error) {
			val, ok := src.(P)
			if !ok {
				return fallback, nil
			}
			// 查字典进行反向映射
			if e, exists := mapping[int32(val)]; exists {
				return e, nil
			}
			return fallback, nil
		},
	}
}

// EntToWrapperConverters
// 组装：Ent -> Proto (利用 protobuf 自带的构造函数 wrapperspb.String 等)
var EntToWrapperConverters = map[ConvertType]copier.TypeConverter{
	ConvertTypeString:  BuildEntToWrapperPointer[string, wrapperspb.StringValue](wrapperspb.String),
	ConvertTypeInt64:   BuildEntToWrapperPointer[int64, wrapperspb.Int64Value](wrapperspb.Int64),
	ConvertTypeInt32:   BuildEntToWrapperPointer[int32, wrapperspb.Int32Value](wrapperspb.Int32),
	ConvertTypeBool:    BuildEntToWrapperPointer[bool, wrapperspb.BoolValue](wrapperspb.Bool),
	ConvertTypeFloat64: BuildEntToWrapperPointer[float64, wrapperspb.DoubleValue](wrapperspb.Double),
	ConvertTypeFloat32: BuildEntToWrapperPointer[float32, wrapperspb.FloatValue](wrapperspb.Float),
	// Bytes 稍微特殊一点，底层是 []byte
	ConvertTypeBytes: BuildEntToWrapperPointer[[]byte, wrapperspb.BytesValue](wrapperspb.Bytes),
	ConvertTypeTime: {
		// 处理时间: time.Time -> *timestamppb.Timestamp
		SrcType: time.Time{},
		DstType: &timestamppb.Timestamp{}, // Proto 生成的时间字段通常是指针
		Fn: func(src interface{}) (interface{}, error) {
			t, ok := src.(time.Time)
			if !ok || t.IsZero() {
				return nil, nil
			}
			return timestamppb.New(t), nil
		},
	},
}

// WrapperToEntConverters
// 组装：Proto -> Ent (传入一个简单的匿名函数来提取 Value 字段)
var WrapperToEntConverters = map[ConvertType]copier.TypeConverter{
	ConvertTypeString:  BuildWrapperToEntPointer[string, wrapperspb.StringValue](func(w *wrapperspb.StringValue) string { return w.Value }),
	ConvertTypeInt64:   BuildWrapperToEntPointer[int64, wrapperspb.Int64Value](func(w *wrapperspb.Int64Value) int64 { return w.Value }),
	ConvertTypeInt32:   BuildWrapperToEntPointer[int32, wrapperspb.Int32Value](func(w *wrapperspb.Int32Value) int32 { return w.Value }),
	ConvertTypeBool:    BuildWrapperToEntPointer[bool, wrapperspb.BoolValue](func(w *wrapperspb.BoolValue) bool { return w.Value }),
	ConvertTypeFloat64: BuildWrapperToEntPointer[float64, wrapperspb.DoubleValue](func(w *wrapperspb.DoubleValue) float64 { return w.Value }),
	ConvertTypeFloat32: BuildWrapperToEntPointer[float32, wrapperspb.FloatValue](func(w *wrapperspb.FloatValue) float32 { return w.Value }),
	ConvertTypeBytes:   BuildWrapperToEntPointer[[]byte, wrapperspb.BytesValue](func(w *wrapperspb.BytesValue) []byte { return w.Value }),
	ConvertTypeTime: {
		// 处理时间: *timestamppb.Timestamp -> time.Time
		SrcType: &timestamppb.Timestamp{},
		DstType: time.Time{},
		Fn: func(src interface{}) (interface{}, error) {
			t, ok := src.(*timestamppb.Timestamp)
			if !ok || t == nil {
				return time.Time{}, nil
			}
			return t.AsTime(), nil
		},
	},
}
