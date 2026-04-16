package entmodule

type Status string

const (
	StatusActive      Status = "active"
	StatusInactive    Status = "inactive"
	StatusUnspecified Status = ""
)

func (Status) Values() []string {
	return []string{string(StatusActive), string(StatusInactive)}
}

var StatusProtoValues = map[string]int32{
	string(StatusActive):   1,
	string(StatusInactive): 2,
}

// ProtoToStatusValues Proto -> Ent 的反向映射字典 (为了查询效率，建议预先定义好)
var ProtoToStatusValues = map[int32]Status{
	1: StatusActive,
	2: StatusInactive,
}

// ToProto 泛型函数：将 Ent 的枚举转为任意 Proto 枚举
func ToProto[T ~int32, P ~string](mapping map[string]int32, s P) T {
	if v, ok := mapping[string(s)]; ok {
		return T(v)
	}
	return T(0)
}

// FromProto 泛型函数：将任意 Proto 枚举转回 Ent 枚举
func FromProto[T ~int32, P ~string](mapping map[int32]P, p T) P {
	if val, ok := mapping[int32(p)]; ok {
		return val
	}
	return P("")
}

func StatusFromProto[T ~int32](p T) Status {
	return FromProto(ProtoToStatusValues, p)
}
