package subagent

import (
	"context"
	"fmt"
	"sync"
)

type Registry struct {
	mu     sync.RWMutex
	agents map[string]Agent
}

func NewRegistry() *Registry {
	return &Registry{agents: map[string]Agent{}}
}

func (r *Registry) Register(name string, agent Agent) error {
	if r == nil {
		return fmt.Errorf("subagent registry is nil")
	}
	if name == "" {
		return fmt.Errorf("subagent name is empty")
	}
	if agent == nil {
		return fmt.Errorf("subagent %q is nil", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[name] = agent
	return nil
}

func (r *Registry) Get(name string) (Agent, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	agent, ok := r.agents[name]
	return agent, ok
}

func (r *Registry) Run(ctx context.Context, name string, task *Task) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	agent, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("subagent %q not found", name)
	}
	result, err := agent.Run(ctx, task)
	if err != nil {
		return nil, err
	}
	if result != nil && result.AgentName == "" {
		result.AgentName = name
	}
	return result, nil
}
