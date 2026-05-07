package safety

import "context"

const (
	ReasonMissingToolName      = "missing_tool"
	ReasonNoToolRule           = "no_tool_rule"
	ReasonUserRequired         = "user_required"
	ReasonRoleDenied           = "role_denied"
	ReasonActionDenied         = "action_denied"
	ReasonResourceDenied       = "resource_denied"
	ReasonToolPermissionDenied = "tool_permission_denied"
)

type ToolPermissionInput struct {
	UserID   string
	Role     string
	ToolName string
	Action   string
	Resource string
	Metadata map[string]any
}

type ToolPermissionResult struct {
	Allowed    bool
	Reason     string
	Violations []Violation
	Metadata   map[string]any
}

type ToolPermissionPolicy interface {
	AllowTool(ctx context.Context, input *ToolPermissionInput) (*ToolPermissionResult, error)
}

type ToolPermissionPolicyFunc func(ctx context.Context, input *ToolPermissionInput) (*ToolPermissionResult, error)

func (f ToolPermissionPolicyFunc) AllowTool(ctx context.Context, input *ToolPermissionInput) (*ToolPermissionResult, error) {
	return f(ctx, input)
}

type ToolRule struct {
	ToolName                string
	AllowedRoles            []string
	DeniedRoles             []string
	AllowedActions          []string
	AllowedResourcePrefixes []string
	RequireUser             bool
}

type StaticToolPermissionPolicy struct {
	DefaultAllow bool
	Rules        []ToolRule
}

func NewStaticToolPermissionPolicy(rules []ToolRule) StaticToolPermissionPolicy {
	return StaticToolPermissionPolicy{Rules: rules}
}

func (p StaticToolPermissionPolicy) AllowTool(ctx context.Context, input *ToolPermissionInput) (*ToolPermissionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input == nil {
		return toolPermissionDenied(ReasonInvalidInput, Violation{Code: ReasonInvalidInput, Message: "tool permission input is nil"}), nil
	}
	if input.ToolName == "" {
		return toolPermissionDenied(ReasonMissingToolName, Violation{Code: ReasonMissingToolName, Message: "tool name is required"}), nil
	}

	rule, ok := p.findRule(input.ToolName)
	if !ok {
		if p.DefaultAllow {
			return toolPermissionAllowed(input.Metadata), nil
		}
		return toolPermissionDenied(ReasonNoToolRule, Violation{
			Code:    ReasonNoToolRule,
			Message: "tool has no permission rule",
			Subject: input.ToolName,
		}), nil
	}

	violations := rule.violations(input)
	if len(violations) > 0 {
		return toolPermissionDenied(violations[0].Code, violations...), nil
	}
	return toolPermissionAllowed(input.Metadata), nil
}

func (p StaticToolPermissionPolicy) findRule(toolName string) (ToolRule, bool) {
	for _, rule := range p.Rules {
		if rule.ToolName == toolName {
			return rule, true
		}
	}
	for _, rule := range p.Rules {
		if rule.ToolName == "*" {
			return rule, true
		}
	}
	return ToolRule{}, false
}

func (r ToolRule) violations(input *ToolPermissionInput) []Violation {
	var violations []Violation
	if r.RequireUser && input.UserID == "" {
		violations = append(violations, Violation{
			Code:    ReasonUserRequired,
			Message: "user id is required",
			Subject: input.ToolName,
		})
	}
	if len(r.DeniedRoles) > 0 && contains(r.DeniedRoles, input.Role) {
		violations = append(violations, Violation{
			Code:    ReasonRoleDenied,
			Message: "role is denied",
			Subject: input.Role,
		})
	}
	if len(r.AllowedRoles) > 0 && !contains(r.AllowedRoles, input.Role) {
		violations = append(violations, Violation{
			Code:    ReasonRoleDenied,
			Message: "role is not allowed",
			Subject: input.Role,
		})
	}
	if len(r.AllowedActions) > 0 && !contains(r.AllowedActions, input.Action) {
		violations = append(violations, Violation{
			Code:    ReasonActionDenied,
			Message: "action is not allowed",
			Subject: input.Action,
		})
	}
	if len(r.AllowedResourcePrefixes) > 0 && !hasAllowedPrefix(input.Resource, r.AllowedResourcePrefixes) {
		violations = append(violations, Violation{
			Code:    ReasonResourceDenied,
			Message: "resource is not allowed",
			Subject: input.Resource,
		})
	}
	if len(violations) > 0 {
		return violations
	}
	if r.ToolName == "" {
		return []Violation{{
			Code:    ReasonToolPermissionDenied,
			Message: "tool rule is empty",
			Subject: input.ToolName,
		}}
	}
	return nil
}

func toolPermissionAllowed(metadata map[string]any) *ToolPermissionResult {
	return &ToolPermissionResult{
		Allowed:  true,
		Reason:   ReasonAllowed,
		Metadata: metadata,
	}
}

func toolPermissionDenied(reason string, violations ...Violation) *ToolPermissionResult {
	return &ToolPermissionResult{
		Allowed:    false,
		Reason:     reason,
		Violations: violations,
	}
}

func hasAllowedPrefix(resource string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}
		if len(resource) >= len(prefix) && resource[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
