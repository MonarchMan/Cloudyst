package safety_test

import (
	"context"
	"testing"

	"ai/internal/pkg/eino/agent/policy"
	"ai/internal/pkg/eino/agent/router"
	"ai/internal/pkg/eino/agent/safety"
)

func TestPermissionToolSelectorAllowsSelectedTool(t *testing.T) {
	selector := safety.NewPermissionToolSelector(
		policy.FirstToolPolicy{},
		safety.NewStaticToolPermissionPolicy([]safety.ToolRule{
			{
				ToolName:       "file_search",
				AllowedRoles:   []string{"member"},
				AllowedActions: []string{"read"},
				RequireUser:    true,
			},
		}),
	)

	result, err := selector.SelectTools(context.Background(), &safety.PermissionToolSelectionInput{
		Decision: &router.RouteDecision{Tools: []string{"file_search"}},
		UserID:   "user-1",
		Role:     "member",
		ToolActions: map[string]string{
			"file_search": "read",
		},
	})
	if err != nil {
		t.Fatalf("select tools failed: %v", err)
	}
	if !result.Allowed || result.Reason != safety.ReasonAllowed {
		t.Fatalf("result = %#v, want allowed", result)
	}
	if len(result.Tools) != 1 || result.Tools[0] != "file_search" {
		t.Fatalf("tools = %v, want file_search", result.Tools)
	}
	if len(result.Denied) != 0 {
		t.Fatalf("denied = %#v, want empty", result.Denied)
	}
}

func TestPermissionToolSelectorUsesSecurityContext(t *testing.T) {
	selector := safety.NewPermissionToolSelector(
		policy.FirstToolPolicy{},
		safety.ToolPermissionPolicyFunc(func(ctx context.Context, input *safety.ToolPermissionInput) (*safety.ToolPermissionResult, error) {
			if input.UserID != "user-1" {
				t.Fatalf("user id = %q, want user-1", input.UserID)
			}
			if input.Role != "member" {
				t.Fatalf("role = %q, want member", input.Role)
			}
			scopes, ok := input.Metadata["scopes"].([]string)
			if !ok || len(scopes) != 1 || scopes[0] != "knowledge:read" {
				t.Fatalf("scopes = %#v, want knowledge:read", input.Metadata["scopes"])
			}
			return &safety.ToolPermissionResult{Allowed: true, Reason: safety.ReasonAllowed}, nil
		}),
	)

	result, err := selector.SelectTools(context.Background(), &safety.PermissionToolSelectionInput{
		Decision: &router.RouteDecision{Tools: []string{"knowledge_retrieval"}},
		Security: &safety.SecurityContext{
			UserID: "user-1",
			Role:   "member",
			Scopes: []string{"knowledge:read"},
		},
	})
	if err != nil {
		t.Fatalf("select tools failed: %v", err)
	}
	if !result.Allowed {
		t.Fatalf("allowed = false, want true: %#v", result)
	}
}

func TestPermissionToolSelectorExplicitFieldsOverrideSecurityContext(t *testing.T) {
	selector := safety.NewPermissionToolSelector(
		policy.FirstToolPolicy{},
		safety.ToolPermissionPolicyFunc(func(ctx context.Context, input *safety.ToolPermissionInput) (*safety.ToolPermissionResult, error) {
			if input.UserID != "override-user" {
				t.Fatalf("user id = %q, want override-user", input.UserID)
			}
			if input.Role != "admin" {
				t.Fatalf("role = %q, want admin", input.Role)
			}
			return &safety.ToolPermissionResult{Allowed: true, Reason: safety.ReasonAllowed}, nil
		}),
	)

	result, err := selector.SelectTools(context.Background(), &safety.PermissionToolSelectionInput{
		Decision: &router.RouteDecision{Tools: []string{"file_search"}},
		Security: &safety.SecurityContext{
			UserID: "user-1",
			Role:   "member",
		},
		UserID: "override-user",
		Role:   "admin",
	})
	if err != nil {
		t.Fatalf("select tools failed: %v", err)
	}
	if !result.Allowed {
		t.Fatalf("allowed = false, want true: %#v", result)
	}
}

func TestPermissionToolSelectorFiltersDeniedTools(t *testing.T) {
	selector := safety.NewPermissionToolSelector(
		policy.AllToolsPolicy{},
		safety.NewStaticToolPermissionPolicy([]safety.ToolRule{
			{
				ToolName:       "web_search",
				AllowedRoles:   []string{"member"},
				AllowedActions: []string{"read"},
			},
			{
				ToolName:       "file_write",
				AllowedRoles:   []string{"admin"},
				AllowedActions: []string{"write"},
			},
		}),
	)

	result, err := selector.SelectTools(context.Background(), &safety.PermissionToolSelectionInput{
		Decision:      &router.RouteDecision{Tools: []string{"web_search", "file_write"}},
		Role:          "member",
		DefaultAction: "read",
		ToolActions: map[string]string{
			"file_write": "write",
		},
	})
	if err != nil {
		t.Fatalf("select tools failed: %v", err)
	}
	if !result.Allowed || result.Reason != safety.ReasonPartialAllowed {
		t.Fatalf("result = %#v, want partial allowed", result)
	}
	if len(result.Tools) != 1 || result.Tools[0] != "web_search" {
		t.Fatalf("tools = %v, want web_search", result.Tools)
	}
	if len(result.Denied) != 1 || result.Denied[0].ToolName != "file_write" {
		t.Fatalf("denied = %#v, want file_write", result.Denied)
	}
}

func TestPermissionToolSelectorDeniesWhenPermissionPolicyMissing(t *testing.T) {
	selector := safety.NewPermissionToolSelector(policy.FirstToolPolicy{}, nil)

	result, err := selector.SelectTools(context.Background(), &safety.PermissionToolSelectionInput{
		Decision: &router.RouteDecision{Tools: []string{"web_search"}},
	})
	if err != nil {
		t.Fatalf("select tools failed: %v", err)
	}
	if result.Allowed {
		t.Fatalf("allowed = true, want false: %#v", result)
	}
	if result.Reason != safety.ReasonMissingPermissionPolicy {
		t.Fatalf("reason = %q, want missing_permission_policy", result.Reason)
	}
	if len(result.Denied) != 1 || result.Denied[0].Reason != safety.ReasonMissingPermissionPolicy {
		t.Fatalf("denied = %#v, want missing permission denial", result.Denied)
	}
}

func TestPermissionToolSelectorDeniesWhenNoToolsRemain(t *testing.T) {
	selector := safety.NewPermissionToolSelector(
		policy.AllToolsPolicy{},
		safety.NewStaticToolPermissionPolicy([]safety.ToolRule{
			{
				ToolName:       "file_write",
				AllowedRoles:   []string{"admin"},
				AllowedActions: []string{"write"},
			},
		}),
	)

	result, err := selector.SelectTools(context.Background(), &safety.PermissionToolSelectionInput{
		Decision:      &router.RouteDecision{Tools: []string{"file_write"}},
		Role:          "member",
		DefaultAction: "write",
	})
	if err != nil {
		t.Fatalf("select tools failed: %v", err)
	}
	if result.Allowed {
		t.Fatalf("allowed = true, want false: %#v", result)
	}
	if result.Reason != safety.ReasonNoAllowedTools {
		t.Fatalf("reason = %q, want no_allowed_tools", result.Reason)
	}
}
