package xerrors

import "sync"

var (
	// reason -> business_code
	paramMap       = make(map[string]int)
	isCommonLoaded = false
	mu             sync.RWMutex
)

// Register 供各个服务调用，注册自己的错误枚举
// passing map[string]int32 allowing direct usage of pb generated maps
func Register(errors map[string]int32) {
	mu.Lock()
	defer mu.Unlock()
	for k, v := range errors {
		paramMap[k] = int(v)
	}
}

func RegisterCommon(errors map[string]int32) {
	mu.Lock()
	defer mu.Unlock()
	if !isCommonLoaded {
		isCommonLoaded = true
		for k, v := range errors {
			paramMap[k] = int(v)
		}
	}
}

// GetCode ErrorEncoder 调用此方法
func GetCode(reason string) int {
	mu.RLock()
	defer mu.RUnlock()
	if code, ok := paramMap[reason]; ok {
		return code
	}
	return 0 // 未找到
}
