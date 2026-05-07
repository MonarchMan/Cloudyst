package security_test

import (
	"context"
	"testing"

	agentsecurity "ai/internal/biz/agent/security"
	"ai/internal/pkg/eino/agent/runner"
	"ai/internal/pkg/eino/agent/safety"
)

func TestDirectResolverBuildsSecurityContext(t *testing.T) {
	resolved, err := agentsecurity.NewDirectResolver(&agentsecurity.ResolveInput{
		UserID: "user-1",
		Role:   "member",
		Scopes: []string{"tool:web_search"},
		Metadata: map[string]any{
			"source": "test",
		},
	}).ResolveSecurity(context.Background(), nil)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.UserID != "user-1" || resolved.Role != "member" {
		t.Fatalf("resolved = %#v, want user and role", resolved)
	}
	if !resolved.HasScope("tool:web_search") {
		t.Fatalf("scopes = %v, want tool:web_search", resolved.Scopes)
	}
	if resolved.Metadata["source"] != "test" {
		t.Fatalf("metadata = %#v, want source", resolved.Metadata)
	}
}

func TestContextResolverReadsSecurityContext(t *testing.T) {
	ctx := agentsecurity.WithContext(context.Background(), &safety.SecurityContext{
		UserID: "user-1",
		Role:   "member",
		Scopes: []string{"knowledge:read"},
	})

	resolved, err := agentsecurity.NewContextResolver(nil).ResolveSecurity(ctx, nil)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.UserID != "user-1" || resolved.Role != "member" {
		t.Fatalf("resolved = %#v, want context security", resolved)
	}
	if !resolved.HasScope("knowledge:read") {
		t.Fatalf("scopes = %v, want knowledge:read", resolved.Scopes)
	}
}

func TestContextResolverFallsBackToDirectResolver(t *testing.T) {
	resolver := agentsecurity.NewContextResolver(agentsecurity.NewDirectResolver(&agentsecurity.ResolveInput{
		UserID: "user-2",
		Role:   "admin",
	}))

	resolved, err := resolver.ResolveSecurity(context.Background(), nil)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.UserID != "user-2" || resolved.Role != "admin" {
		t.Fatalf("resolved = %#v, want fallback security", resolved)
	}
}

func TestContextResolverReturnsErrorWhenMissing(t *testing.T) {
	_, err := agentsecurity.NewContextResolver(nil).ResolveSecurity(context.Background(), nil)
	if err == nil {
		t.Fatal("resolve succeeded, want missing context error")
	}
}

func TestContextResolverReadsRunnerInputSecurity(t *testing.T) {
	resolved, err := agentsecurity.NewContextResolver(nil).ResolveSecurity(context.Background(), &runner.Input{
		Security: &safety.SecurityContext{
			UserID: "user-3",
			Role:   "member",
		},
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.UserID != "user-3" || resolved.Role != "member" {
		t.Fatalf("resolved = %#v, want runner input security", resolved)
	}
}

func TestSecurityContextIsCloned(t *testing.T) {
	original := &safety.SecurityContext{
		UserID: "user-1",
		Scopes: []string{
			"tool:web_search",
		},
		Metadata: map[string]any{"source": "original"},
	}
	ctx := agentsecurity.WithContext(context.Background(), original)
	original.Scopes[0] = "mutated"
	original.Metadata["source"] = "mutated"

	resolved := agentsecurity.FromContext(ctx)
	if resolved.HasScope("mutated") {
		t.Fatalf("resolved scopes = %v, want cloned original value", resolved.Scopes)
	}
	if resolved.Metadata["source"] != "original" {
		t.Fatalf("metadata source = %v, want original", resolved.Metadata["source"])
	}
}
