package model

type Platform string

const (
	PlatformOpenAI   Platform = "openai"
	PlatformQwen     Platform = "qwen"
	PlatformDeepseek Platform = "deepseek"
	PlatformArk      Platform = "ark"
	PlatformQianFan  Platform = "qianfan"
	PlatformOllama   Platform = "ollama"
	PlatformGemini   Platform = "gemini"
	PlatformClaude   Platform = "claude"
)
