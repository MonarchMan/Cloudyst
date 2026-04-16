package model

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino-ext/components/model/qianfan"
	"github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/components/model"
	"google.golang.org/genai"
)

const modelCacheKeyPrefix = "ai_model"

type (
	AiModelManager interface {
		GetModel(cfg *ModelConfig) (model.ToolCallingChatModel, error)
		RegistryFactory(platform Platform, f ModelFactory)
	}

	aiModelManager struct {
		mu        sync.RWMutex
		models    map[string]model.ToolCallingChatModel
		factories map[Platform]ModelFactory
	}

	ModelConfig struct {
		Platform Platform
		APIKey   string // Qianfan中用来表示"AccessKey;SecretKey"
		Url      string
		Model    string
	}
)

type ModelFactory func(cfg *ModelConfig) (model.ToolCallingChatModel, error)

func (c *ModelConfig) ID() string {
	return fmt.Sprintf("%s:%s:%s:%s", modelCacheKeyPrefix, c.Platform, c.Model, c.APIKey)
}

func NewAiModelManager() AiModelManager {
	m := &aiModelManager{
		models:    make(map[string]model.ToolCallingChatModel),
		factories: make(map[Platform]ModelFactory),
	}
	m.init()
	return m
}

func (m *aiModelManager) init() {
	m.RegistryFactory(PlatformDeepseek, DeepseekModelFactory)
	m.RegistryFactory(PlatformQwen, QwenModelFactory)
	m.RegistryFactory(PlatformArk, ArkModelFactory)
	m.RegistryFactory(PlatformQianFan, QianFanModelFactory)
	m.RegistryFactory(PlatformOpenAI, OpenAIModelFactory)
	m.RegistryFactory(PlatformOllama, OllamaModelFactory)
	m.RegistryFactory(PlatformGemini, GeminiModelFactory)
	m.RegistryFactory(PlatformClaude, ClaudeModelFactory)
}

func (m *aiModelManager) RegistryFactory(platform Platform, f ModelFactory) {
	m.factories[platform] = f
}

func (m *aiModelManager) GetModel(cfg *ModelConfig) (model.ToolCallingChatModel, error) {
	id := cfg.ID()

	// 第一重检查：读锁
	m.mu.RLock()
	if inst, ok := m.models[id]; ok {
		m.mu.RUnlock()
		return inst, nil
	}
	m.mu.RUnlock()

	// 第二重检查：写锁 (Double-Check Locking)
	m.mu.Lock()
	defer m.mu.Unlock()

	if inst, ok := m.models[id]; ok {
		return inst, nil
	}

	// 执行 Factory 创建流程
	factory, ok := m.factories[cfg.Platform]
	if !ok {
		return nil, fmt.Errorf("platform %s not supported", cfg.Platform)
	}

	newInst, err := factory(cfg)
	if err != nil {
		return nil, err
	}

	m.models[id] = newInst
	return newInst, nil
}

func OpenAIModelFactory(cfg *ModelConfig) (model.ToolCallingChatModel, error) {
	mcfg := &openai.ChatModelConfig{
		APIKey: cfg.APIKey,
		Model:  cfg.Model,
	}
	if cfg.Url != "" {
		mcfg.BaseURL = cfg.Url
	}
	return openai.NewChatModel(context.Background(), mcfg)
}

func DeepseekModelFactory(cfg *ModelConfig) (model.ToolCallingChatModel, error) {
	mcfg := &deepseek.ChatModelConfig{
		APIKey: cfg.APIKey,
		Model:  cfg.Model,
	}
	if cfg.Url != "" {
		mcfg.BaseURL = cfg.Url
	}
	return deepseek.NewChatModel(context.Background(), mcfg)
}

func ArkModelFactory(cfg *ModelConfig) (model.ToolCallingChatModel, error) {
	mcfg := &ark.ChatModelConfig{
		APIKey: cfg.APIKey,
		Model:  cfg.Model,
	}
	if cfg.Url != "" {
		mcfg.BaseURL = cfg.Url
	}
	return ark.NewChatModel(context.Background(), mcfg)
}

func QwenModelFactory(cfg *ModelConfig) (model.ToolCallingChatModel, error) {
	mcfg := &qwen.ChatModelConfig{
		APIKey: cfg.APIKey,
		Model:  cfg.Model,
	}
	if cfg.Url != "" {
		mcfg.BaseURL = cfg.Url
	}
	return qwen.NewChatModel(context.Background(), mcfg)
}

func OllamaModelFactory(cfg *ModelConfig) (model.ToolCallingChatModel, error) {
	mcfg := &ollama.ChatModelConfig{
		Model: cfg.Model,
	}
	if cfg.Url != "" {
		mcfg.BaseURL = cfg.Url
	}
	return ollama.NewChatModel(context.Background(), mcfg)
}

func QianFanModelFactory(cfg *ModelConfig) (model.ToolCallingChatModel, error) {
	qcfg := qianfan.GetQianfanSingletonConfig()
	keys := strings.Split(cfg.APIKey, ";")
	qcfg.AccessKey = keys[0]
	qcfg.SecretKey = keys[1]
	return qianfan.NewChatModel(context.Background(), &qianfan.ChatModelConfig{
		Model: cfg.Model,
	})
}

func GeminiModelFactory(cfg *ModelConfig) (model.ToolCallingChatModel, error) {
	ctx := context.Background()
	clientConfig := genai.ClientConfig{APIKey: cfg.APIKey}
	if cfg.Url != "" {
		clientConfig.HTTPOptions = genai.HTTPOptions{
			BaseURL: cfg.Url,
		}
	}
	client, err := genai.NewClient(ctx, &clientConfig)
	if err != nil {
		return nil, err
	}
	return gemini.NewChatModel(ctx, &gemini.Config{
		Client: client,
		Model:  cfg.Model,
	})
}

func ClaudeModelFactory(cfg *ModelConfig) (model.ToolCallingChatModel, error) {
	mcfg := &claude.Config{
		APIKey: cfg.APIKey,
		Model:  cfg.Model,
	}
	if cfg.Url != "" {
		mcfg.BaseURL = &cfg.Url
	}
	return claude.NewChatModel(context.Background(), mcfg)
}
