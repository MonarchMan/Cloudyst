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

const (
	deepseekV4Flash = "deepseek-v4-flash"
	deepseekV4Pro   = "deepseek-v4-pro"
)
