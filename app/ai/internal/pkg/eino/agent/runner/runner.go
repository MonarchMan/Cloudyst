package runner

import (
	"context"
	"fmt"

	"ai/internal/pkg/eino/agent/budget"
	"ai/internal/pkg/eino/agent/observe"
	"ai/internal/pkg/eino/agent/planner"
	"ai/internal/pkg/eino/agent/policy"
	"ai/internal/pkg/eino/agent/router"
	"ai/internal/pkg/eino/agent/safety"
	"ai/internal/pkg/eino/agent/trace"
	"ai/internal/pkg/eino/agent/verify"

	"github.com/cloudwego/eino/schema"
)

type Input struct {
	Query            string
	Messages         []*schema.Message
	Route            *router.RouteInput
	RequireCitations bool
	Security         *safety.SecurityContext
	AgentName        string
	DelegationDepth  int
	ContextItems     int
}

type Output struct {
	Decision    *router.RouteDecision
	Plan        *planner.Plan
	ToolResult  *observe.ToolResult
	Observation *observe.Observation
	Answer      string
	Grounding   *verify.GroundResult
	Trace       trace.Trace
}

type ToolInput struct {
	Query    string
	Decision *router.RouteDecision
	Plan     *planner.Plan
	Messages []*schema.Message
}

type ToolInvoker interface {
	Invoke(ctx context.Context, toolName string, input *ToolInput) (any, error)
}

type ToolInvokerFunc func(ctx context.Context, toolName string, input *ToolInput) (any, error)

func (f ToolInvokerFunc) Invoke(ctx context.Context, toolName string, input *ToolInput) (any, error) {
	return f(ctx, toolName, input)
}

type AnswerInput struct {
	Query       string
	Decision    *router.RouteDecision
	Plan        *planner.Plan
	ToolResult  *observe.ToolResult
	Observation *observe.Observation
	Messages    []*schema.Message
}

type Answerer interface {
	Answer(ctx context.Context, input *AnswerInput) (string, error)
}

type AnswererFunc func(ctx context.Context, input *AnswerInput) (string, error)

func (f AnswererFunc) Answer(ctx context.Context, input *AnswerInput) (string, error) {
	return f(ctx, input)
}

type Runner struct {
	Router     router.Router
	Planner    planner.Planner
	Invoker    ToolInvoker
	Normalizer observe.Normalizer
	Observer   observe.Observer
	Answerer   Answerer
	Grounder   verify.Grounder

	Budget         *budget.Tracker
	ToolSelector   policy.ToolSelectionPolicy
	RetryPolicy    policy.RetryPolicy
	FallbackPolicy policy.FallbackPolicy
	Security       *SecurityOptions
}

type Option func(*Runner)

type SecurityResolver interface {
	ResolveSecurity(ctx context.Context, input *Input) (*safety.SecurityContext, error)
}

type SecurityResolverFunc func(ctx context.Context, input *Input) (*safety.SecurityContext, error)

func (f SecurityResolverFunc) ResolveSecurity(ctx context.Context, input *Input) (*safety.SecurityContext, error) {
	return f(ctx, input)
}

type SecurityOptions struct {
	Resolver        SecurityResolver
	Permission      safety.ToolPermissionPolicy
	Guard           safety.GuardPolicy
	AgentName       string
	DefaultAction   string
	DefaultResource string
	ToolActions     map[string]string
	ToolResources   map[string]string
	ToolMetadata    map[string]map[string]any
	Metadata        map[string]any
}

type SecurityDeniedError struct {
	Stage      string
	Reason     string
	Denied     []safety.ToolSelectionDenial
	Violations []safety.Violation
}

func (e *SecurityDeniedError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("runner security denied: stage=%s reason=%s", e.Stage, e.Reason)
}

func WithSecurity(options SecurityOptions) Option {
	return func(r *Runner) {
		r.Security = &options
	}
}

func New(r router.Router, p planner.Planner, invoker ToolInvoker, opts ...Option) *Runner {
	agent := &Runner{
		Router:       r,
		Planner:      p,
		Invoker:      invoker,
		Normalizer:   observe.NewDefaultNormalizer(),
		Observer:     observe.NewSummaryObserver(0),
		Grounder:     verify.NewCitationGrounder(),
		ToolSelector: policy.FirstToolPolicy{},
		RetryPolicy:  policy.NoRetryPolicy{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	return agent
}

func (r *Runner) Run(ctx context.Context, input *Input) (*Output, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, fmt.Errorf("runner input is nil")
	}
	if r == nil {
		return nil, fmt.Errorf("runner is nil")
	}
	if r.Budget != nil {
		var cancel context.CancelFunc
		ctx, cancel = r.Budget.WithTimeout(ctx)
		defer cancel()
	}

	recorder := trace.NewInMemoryRecorder()
	out := &Output{}

	decision, err := r.route(ctx, input)
	if err != nil {
		return nil, err
	}
	out.Decision = decision
	selectedTools, err := r.selectTools(ctx, input, decision)
	if err != nil {
		return nil, err
	}
	recorder.Record(trace.Event{
		Node:          "router",
		InputSummary:  input.Query,
		OutputSummary: string(decision.Target),
		Tool:          first(selectedTools),
	})

	if err := r.consume(ctx, budget.ResourceStep, 1); err != nil {
		return nil, err
	}
	plan, err := r.plan(ctx, input)
	if err != nil {
		return nil, err
	}
	out.Plan = plan
	recorder.Record(trace.Event{
		Node:          "planner",
		InputSummary:  input.Query,
		OutputSummary: lastStepID(plan),
	})

	toolName := first(selectedTools)
	if toolName != "" {
		toolResult, observation, err := r.invokeAndObserve(ctx, toolName, input, decision, plan, recorder)
		if err != nil {
			return nil, err
		}
		out.ToolResult = toolResult
		out.Observation = observation
	}

	answer, grounding, err := r.answerAndGround(ctx, input, out, recorder)
	if err != nil {
		return nil, err
	}
	out.Answer = answer
	out.Grounding = grounding
	out.Trace = recorder.Finish()
	return out, nil
}

func (r *Runner) route(ctx context.Context, input *Input) (*router.RouteDecision, error) {
	if r.Router == nil {
		return nil, fmt.Errorf("runner router is nil")
	}
	routeInput := &router.RouteInput{}
	if input.Route != nil {
		cp := *input.Route
		routeInput = &cp
	}
	if routeInput.Query == "" {
		routeInput.Query = input.Query
	}
	if routeInput.Messages == nil {
		routeInput.Messages = input.Messages
	}
	return r.Router.Route(ctx, routeInput)
}

func (r *Runner) selectTools(ctx context.Context, input *Input, decision *router.RouteDecision) ([]string, error) {
	if r.Security != nil {
		return r.selectToolsWithSecurity(ctx, input, decision)
	}
	selector := r.ToolSelector
	if selector == nil {
		selector = policy.FirstToolPolicy{}
	}
	return selector.SelectTools(ctx, &policy.ToolSelectionInput{Decision: decision})
}

func (r *Runner) selectToolsWithSecurity(ctx context.Context, input *Input, decision *router.RouteDecision) ([]string, error) {
	security, err := r.resolveSecurity(ctx, input)
	if err != nil {
		return nil, err
	}

	selection, err := safety.NewPermissionToolSelector(r.ToolSelector, r.Security.Permission).SelectTools(ctx, &safety.PermissionToolSelectionInput{
		Decision:        decision,
		Security:        security,
		DefaultAction:   r.Security.DefaultAction,
		DefaultResource: r.Security.DefaultResource,
		ToolActions:     r.Security.ToolActions,
		ToolResources:   r.Security.ToolResources,
		ToolMetadata:    r.Security.ToolMetadata,
		Metadata:        r.Security.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if selection == nil || !selection.Allowed {
		reason := safety.ReasonNoAllowedTools
		var denied []safety.ToolSelectionDenial
		if selection != nil {
			reason = selection.Reason
			denied = selection.Denied
		}
		return nil, &SecurityDeniedError{
			Stage:  "permission",
			Reason: reason,
			Denied: denied,
		}
	}

	if r.Security.Guard == nil {
		return selection.Tools, nil
	}
	guard, err := r.Security.Guard.Guard(ctx, &safety.GuardInput{
		AgentName:       r.agentName(input),
		ToolNames:       selection.Tools,
		DelegationDepth: input.DelegationDepth,
		ContextItems:    contextItems(input),
		Budget:          r.Budget,
		Metadata:        r.Security.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if guard == nil || !guard.Allowed {
		reason := safety.ReasonInvalidInput
		var violations []safety.Violation
		if guard != nil {
			reason = guard.Reason
			violations = guard.Violations
		}
		return nil, &SecurityDeniedError{
			Stage:      "guard",
			Reason:     reason,
			Violations: violations,
		}
	}
	return selection.Tools, nil
}

func (r *Runner) resolveSecurity(ctx context.Context, input *Input) (*safety.SecurityContext, error) {
	if input != nil && input.Security != nil {
		return input.Security, nil
	}
	if r.Security != nil && r.Security.Resolver != nil {
		return r.Security.Resolver.ResolveSecurity(ctx, input)
	}
	return nil, nil
}

func (r *Runner) agentName(input *Input) string {
	if input != nil && input.AgentName != "" {
		return input.AgentName
	}
	if r.Security != nil {
		return r.Security.AgentName
	}
	return ""
}

func contextItems(input *Input) int {
	if input == nil {
		return 0
	}
	if input.ContextItems > 0 {
		return input.ContextItems
	}
	return len(input.Messages)
}

func (r *Runner) plan(ctx context.Context, input *Input) (*planner.Plan, error) {
	if r.Planner == nil {
		return nil, fmt.Errorf("runner planner is nil")
	}
	return r.Planner.Plan(ctx, input.Query, input.Messages)
}

func (r *Runner) invokeAndObserve(ctx context.Context, toolName string, input *Input, decision *router.RouteDecision, plan *planner.Plan, recorder trace.Recorder) (*observe.ToolResult, *observe.Observation, error) {
	if r.Invoker == nil {
		return nil, nil, fmt.Errorf("runner tool invoker is nil")
	}
	if err := r.consume(ctx, budget.ResourceToolCall, 1); err != nil {
		return nil, nil, err
	}
	normalizer := r.Normalizer
	if normalizer == nil {
		normalizer = observe.NewDefaultNormalizer()
	}
	observer := r.Observer
	if observer == nil {
		observer = observe.NewSummaryObserver(0)
	}

	toolOutput, toolErr, attempts, err := r.invokeWithPolicy(ctx, toolName, &ToolInput{
		Query:    input.Query,
		Decision: decision,
		Plan:     plan,
		Messages: input.Messages,
	})
	if err != nil {
		return nil, nil, err
	}
	recorder.Record(trace.Event{
		Node:          "tool",
		Tool:          toolName,
		OutputSummary: fmt.Sprint(toolOutput),
		Error:         errorString(toolErr),
		Metadata:      map[string]any{"attempts": attempts},
	})

	toolResult, err := normalizer.Normalize(ctx, toolName, toolOutput, toolErr)
	if err != nil {
		return nil, nil, err
	}
	observation, err := observer.Summarize(ctx, toolResult)
	if err != nil {
		return nil, nil, err
	}
	recorder.Record(trace.Event{
		Node:          "observer",
		InputSummary:  toolResult.Type,
		OutputSummary: observation.Summary,
		Tool:          toolName,
	})
	return toolResult, observation, nil
}

func (r *Runner) invokeWithPolicy(ctx context.Context, toolName string, input *ToolInput) (any, error, int, error) {
	retryPolicy := r.RetryPolicy
	if retryPolicy == nil {
		retryPolicy = policy.NoRetryPolicy{}
	}

	var output any
	var toolErr error
	attempts := 0
	for {
		attempts++
		output, toolErr = r.Invoker.Invoke(ctx, toolName, input)
		if toolErr == nil {
			return output, nil, attempts, nil
		}
		shouldRetry, err := retryPolicy.ShouldRetry(ctx, &policy.RetryInput{Attempt: attempts, Err: toolErr})
		if err != nil {
			return nil, nil, attempts, err
		}
		if !shouldRetry {
			break
		}
	}

	fallback := r.FallbackPolicy
	if fallback == nil {
		return output, toolErr, attempts, nil
	}
	fallbackOutput, handled, err := fallback.Fallback(ctx, &policy.FallbackInput{
		ToolName: toolName,
		Attempts: attempts,
		Err:      toolErr,
	})
	if err != nil {
		return nil, nil, attempts, err
	}
	if handled {
		return fallbackOutput, nil, attempts, nil
	}
	return output, toolErr, attempts, nil
}

func (r *Runner) answerAndGround(ctx context.Context, input *Input, out *Output, recorder trace.Recorder) (string, *verify.GroundResult, error) {
	if r.Answerer == nil {
		return "", nil, nil
	}

	answer, err := r.Answerer.Answer(ctx, &AnswerInput{
		Query:       input.Query,
		Decision:    out.Decision,
		Plan:        out.Plan,
		ToolResult:  out.ToolResult,
		Observation: out.Observation,
		Messages:    input.Messages,
	})
	if err != nil {
		return "", nil, err
	}
	if r.Grounder == nil {
		return answer, nil, nil
	}

	var contextResults []*observe.ToolResult
	if out.ToolResult != nil {
		contextResults = append(contextResults, out.ToolResult)
	}
	grounding, err := r.Grounder.Ground(ctx, &verify.GroundInput{
		Answer:           answer,
		Context:          contextResults,
		RequireCitations: input.RequireCitations,
	})
	if err != nil {
		return "", nil, err
	}
	recorder.Record(trace.Event{
		Node:          "grounder",
		InputSummary:  answer,
		OutputSummary: passStatus(grounding.Passed),
	})
	return answer, grounding, nil
}

func (r *Runner) consume(ctx context.Context, resource budget.Resource, amount int) error {
	if r.Budget == nil {
		return nil
	}
	return r.Budget.Consume(ctx, resource, amount)
}

func first(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func lastStepID(plan *planner.Plan) string {
	if plan == nil || len(plan.Steps) == 0 {
		return ""
	}
	return plan.Steps[len(plan.Steps)-1].ID
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func passStatus(ok bool) string {
	if ok {
		return "passed"
	}
	return "failed"
}
