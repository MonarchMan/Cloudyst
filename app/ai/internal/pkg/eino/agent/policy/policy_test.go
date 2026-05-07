package policy_test

import (
	"context"
	"errors"
	"testing"

	"ai/internal/pkg/eino/agent/policy"
	"ai/internal/pkg/eino/agent/router"
)

func TestToolSelectionPolicies(t *testing.T) {
	decision := &router.RouteDecision{Tools: []string{"", "web_search", "knowledge"}}

	first, err := policy.FirstToolPolicy{}.SelectTools(context.Background(), &policy.ToolSelectionInput{Decision: decision})
	if err != nil {
		t.Fatalf("first policy failed: %v", err)
	}
	if len(first) != 1 || first[0] != "web_search" {
		t.Fatalf("first = %v, want [web_search]", first)
	}

	all, err := (policy.AllToolsPolicy{Max: 2}).SelectTools(context.Background(), &policy.ToolSelectionInput{Decision: decision})
	if err != nil {
		t.Fatalf("all policy failed: %v", err)
	}
	if len(all) != 2 || all[0] != "web_search" || all[1] != "knowledge" {
		t.Fatalf("all = %v, want [web_search knowledge]", all)
	}
}

func TestFixedRetryPolicy(t *testing.T) {
	retry := policy.FixedRetryPolicy{MaxAttempts: 2}
	again, err := retry.ShouldRetry(context.Background(), &policy.RetryInput{Attempt: 1, Err: errors.New("temporary")})
	if err != nil {
		t.Fatalf("retry policy failed: %v", err)
	}
	if !again {
		t.Fatal("want retry after first failed attempt")
	}

	again, err = retry.ShouldRetry(context.Background(), &policy.RetryInput{Attempt: 2, Err: errors.New("temporary")})
	if err != nil {
		t.Fatalf("retry policy failed: %v", err)
	}
	if again {
		t.Fatal("did not expect retry at max attempts")
	}
}

func TestFallbackPolicies(t *testing.T) {
	result, handled, err := (policy.ErrorMessageFallbackPolicy{Prefix: "fallback"}).Fallback(
		context.Background(),
		&policy.FallbackInput{ToolName: "web_search", Attempts: 2, Err: errors.New("down")},
	)
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}
	if !handled {
		t.Fatal("fallback was not handled")
	}
	if result != "fallback: down" {
		t.Fatalf("result = %v, want fallback: down", result)
	}
}
