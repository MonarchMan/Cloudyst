package utils

import (
	"strconv"
	"strings"

	"github.com/samber/lo"
)

func ToIntSlice(str string) []int {
	if str == "" {
		return nil
	}
	raw := strings.Split(str, ",")
	slice := lo.Map(raw, func(s string, _ int) int {
		num, _ := strconv.Atoi(s)
		return num
	})
	return slice
}

// StringMapToAnyMap 将 map[string]string 转换为 map[string]any
func StringMapToAnyMap(m map[string]string) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
