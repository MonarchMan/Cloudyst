package utils

import (
	"fmt"

	"github.com/tiktoken-go/tokenizer"
)

// CountTokens 估算字符串的 Token 数量
func CountTokens(text string, modelName string) (int, error) {
	// 根据你的大模型名称获取对应的编码器 (比如 "gpt-4", "gpt-3.5-turbo")
	// 如果你用的是通用模型，通常传 tokenizer.Cl100kBase 编码即可
	enc, err := tokenizer.ForModel(tokenizer.Model(modelName))
	if err != nil {
		// 如果找不到对应模型的专属字典，退化为最常用的 cl100k_base 编码
		enc, err = tokenizer.Get(tokenizer.Cl100kBase)
		if err != nil {
			return 0, fmt.Errorf("failed to get tokenizer: %w", err)
		}
	}

	// 执行 Encode，返回的切片长度就是精确的 Token 数量
	ids, _, _ := enc.Encode(text)
	return len(ids), nil
}
