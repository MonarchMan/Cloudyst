package safety_test

import (
	"context"
	"testing"

	"ai/internal/pkg/eino/agent/safety"
)

func TestStaticToolPermissionPolicyAllowsMatchingRule(t *testing.T) {
	policy := safety.NewStaticToolPermissionPolicy([]safety.ToolRule{
		{
			ToolName:                "file_search",
			AllowedRoles:            []string{"member"},
			AllowedActions:          []string{"read"},
			AllowedResourcePrefixes: []string{"file:"},
			RequireUser:             true,
		},
	})

	result, err := policy.AllowTool(context.Background(), &safety.ToolPermissionInput{
		UserID:   "user-1",
		Role:     "member",
		ToolName: "file_search",
		Action:   "read",
		Resource: "file:doc-1",
	})
	if err != nil {
		t.Fatalf("allow tool failed: %v", err)
	}
	if !result.Allowed {
		t.Fatalf("allowed = false, want true: %#v", result)
	}
	if result.Reason != safety.ReasonAllowed {
		t.Fatalf("reason = %q, want allowed", result.Reason)
	}
}

func TestStaticToolPermissionPolicyDeniesUnknownToolByDefault(t *testing.T) {
	policy := safety.NewStaticToolPermissionPolicy(nil)

	result, err := policy.AllowTool(context.Background(), &safety.ToolPermissionInput{
		ToolName: "web_search",
	})
	if err != nil {
		t.Fatalf("allow tool failed: %v", err)
	}
	if result.Allowed {
		t.Fatalf("allowed = true, want false: %#v", result)
	}
	if result.Reason != safety.ReasonNoToolRule {
		t.Fatalf("reason = %q, want no_tool_rule", result.Reason)
	}
}

func TestStaticToolPermissionPolicyDeniesRoleActionAndResource(t *testing.T) {
	policy := safety.NewStaticToolPermissionPolicy([]safety.ToolRule{
		{
			ToolName:                "file_write",
			AllowedRoles:            []string{"admin"},
			AllowedActions:          []string{"write"},
			AllowedResourcePrefixes: []string{"file:"},
		},
	})

	result, err := policy.AllowTool(context.Background(), &safety.ToolPermissionInput{
		Role:     "member",
		ToolName: "file_write",
		Action:   "delete",
		Resource: "external:doc-1",
	})
	if err != nil {
		t.Fatalf("allow tool failed: %v", err)
	}
	if result.Allowed {
		t.Fatalf("allowed = true, want false: %#v", result)
	}
	if result.Reason != safety.ReasonRoleDenied {
		t.Fatalf("reason = %q, want first violation role_denied", result.Reason)
	}
	if len(result.Violations) != 3 {
		t.Fatalf("violations = %d, want 3", len(result.Violations))
	}
}

func TestStaticToolPermissionPolicyRequiresUser(t *testing.T) {
	policy := safety.NewStaticToolPermissionPolicy([]safety.ToolRule{
		{
			ToolName:    "knowledge_retrieval",
			RequireUser: true,
		},
	})

	result, err := policy.AllowTool(context.Background(), &safety.ToolPermissionInput{
		ToolName: "knowledge_retrieval",
	})
	if err != nil {
		t.Fatalf("allow tool failed: %v", err)
	}
	if result.Allowed {
		t.Fatalf("allowed = true, want false: %#v", result)
	}
	if result.Reason != safety.ReasonUserRequired {
		t.Fatalf("reason = %q, want user_required", result.Reason)
	}
}

func TestStaticToolPermissionPolicyDefaultAllow(t *testing.T) {
	policy := safety.StaticToolPermissionPolicy{DefaultAllow: true}

	result, err := policy.AllowTool(context.Background(), &safety.ToolPermissionInput{
		ToolName: "plain_llm",
	})
	if err != nil {
		t.Fatalf("allow tool failed: %v", err)
	}
	if !result.Allowed {
		t.Fatalf("allowed = false, want true: %#v", result)
	}
}
