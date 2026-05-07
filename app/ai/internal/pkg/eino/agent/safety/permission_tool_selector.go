package safety

import (
	"context"

	"ai/internal/pkg/eino/agent/policy"
	"ai/internal/pkg/eino/agent/router"
)

const (
	ReasonPartialAllowed          = "partial_allowed"
	ReasonNoAllowedTools          = "no_allowed_tools"
	ReasonMissingPermissionPolicy = "missing_permission_policy"
)

type PermissionToolSelectionInput struct {
	Decision        *router.RouteDecision
	Security        *SecurityContext
	UserID          string
	Role            string
	DefaultAction   string
	DefaultResource string
	ToolActions     map[string]string
	ToolResources   map[string]string
	ToolMetadata    map[string]map[string]any
	Metadata        map[string]any
}

type PermissionToolSelectionResult struct {
	Allowed        bool
	Reason         string
	Tools          []string
	CandidateTools []string
	Denied         []ToolSelectionDenial
	Metadata       map[string]any
}

type ToolSelectionDenial struct {
	ToolName   string
	Reason     string
	Violations []Violation
	Metadata   map[string]any
}

type PermissionToolSelector struct {
	Selector   policy.ToolSelectionPolicy
	Permission ToolPermissionPolicy
}

func NewPermissionToolSelector(selector policy.ToolSelectionPolicy, permission ToolPermissionPolicy) PermissionToolSelector {
	return PermissionToolSelector{
		Selector:   selector,
		Permission: permission,
	}
}

func (s PermissionToolSelector) SelectTools(ctx context.Context, input *PermissionToolSelectionInput) (*PermissionToolSelectionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input == nil {
		return &PermissionToolSelectionResult{
			Allowed: false,
			Reason:  ReasonInvalidInput,
			Denied: []ToolSelectionDenial{{
				Reason:     ReasonInvalidInput,
				Violations: []Violation{{Code: ReasonInvalidInput, Message: "permission tool selection input is nil"}},
			}},
		}, nil
	}

	selector := s.Selector
	if selector == nil {
		selector = policy.FirstToolPolicy{}
	}
	candidates, err := selector.SelectTools(ctx, &policy.ToolSelectionInput{Decision: input.Decision})
	if err != nil {
		return nil, err
	}

	result := &PermissionToolSelectionResult{
		Allowed:        true,
		Reason:         ReasonAllowed,
		CandidateTools: candidates,
		Metadata:       input.Metadata,
	}
	if len(candidates) == 0 {
		return result, nil
	}
	if s.Permission == nil {
		result.Allowed = false
		result.Reason = ReasonMissingPermissionPolicy
		for _, toolName := range candidates {
			result.Denied = append(result.Denied, ToolSelectionDenial{
				ToolName: toolName,
				Reason:   ReasonMissingPermissionPolicy,
				Violations: []Violation{{
					Code:    ReasonMissingPermissionPolicy,
					Message: "tool permission policy is nil",
					Subject: toolName,
				}},
			})
		}
		return result, nil
	}

	for _, toolName := range candidates {
		permission, err := s.Permission.AllowTool(ctx, &ToolPermissionInput{
			UserID:   userIDForSelection(input),
			Role:     roleForSelection(input),
			ToolName: toolName,
			Action:   valueForTool(input.ToolActions, toolName, input.DefaultAction),
			Resource: valueForTool(input.ToolResources, toolName, input.DefaultResource),
			Metadata: metadataForTool(input.ToolMetadata, toolName, metadataForSelection(input)),
		})
		if err != nil {
			return nil, err
		}
		if permission != nil && permission.Allowed {
			result.Tools = append(result.Tools, toolName)
			continue
		}
		result.Denied = append(result.Denied, denialForTool(toolName, permission))
	}

	switch {
	case len(result.Denied) == 0:
		result.Allowed = true
		result.Reason = ReasonAllowed
	case len(result.Tools) > 0:
		result.Allowed = true
		result.Reason = ReasonPartialAllowed
	default:
		result.Allowed = false
		result.Reason = ReasonNoAllowedTools
	}
	return result, nil
}

func denialForTool(toolName string, permission *ToolPermissionResult) ToolSelectionDenial {
	if permission == nil {
		return ToolSelectionDenial{
			ToolName: toolName,
			Reason:   ReasonToolPermissionDenied,
			Violations: []Violation{{
				Code:    ReasonToolPermissionDenied,
				Message: "tool permission result is nil",
				Subject: toolName,
			}},
		}
	}
	return ToolSelectionDenial{
		ToolName:   toolName,
		Reason:     permission.Reason,
		Violations: permission.Violations,
		Metadata:   permission.Metadata,
	}
}

func valueForTool(values map[string]string, toolName string, fallback string) string {
	if values == nil {
		return fallback
	}
	if value := values[toolName]; value != "" {
		return value
	}
	return fallback
}

func userIDForSelection(input *PermissionToolSelectionInput) string {
	if input == nil {
		return ""
	}
	if input.UserID != "" {
		return input.UserID
	}
	if input.Security == nil {
		return ""
	}
	return input.Security.UserID
}

func roleForSelection(input *PermissionToolSelectionInput) string {
	if input == nil {
		return ""
	}
	if input.Role != "" {
		return input.Role
	}
	if input.Security == nil {
		return ""
	}
	return input.Security.Role
}

func metadataForSelection(input *PermissionToolSelectionInput) map[string]any {
	if input == nil {
		return nil
	}
	if input.Security == nil {
		return input.Metadata
	}
	return input.Security.MetadataWithIdentity(input.Metadata)
}

func metadataForTool(values map[string]map[string]any, toolName string, fallback map[string]any) map[string]any {
	if values == nil {
		return fallback
	}
	if value := values[toolName]; value != nil {
		return value
	}
	return fallback
}
