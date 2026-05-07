package safety_test

import (
	"testing"

	"ai/internal/pkg/eino/agent/safety"
)

func TestSecurityContextHasScope(t *testing.T) {
	security := &safety.SecurityContext{
		Scopes: []string{"tool:read", "knowledge:read"},
	}

	if !security.HasScope("tool:read") {
		t.Fatal("HasScope returned false, want true")
	}
	if security.HasScope("file:write") {
		t.Fatal("HasScope returned true, want false")
	}
}

func TestSecurityContextMetadataWithIdentity(t *testing.T) {
	security := &safety.SecurityContext{
		Scopes: []string{"tool:read"},
		Metadata: map[string]any{
			"source": "session",
		},
	}

	metadata := security.MetadataWithIdentity(map[string]any{
		"source": "request",
	})
	if metadata["source"] != "request" {
		t.Fatalf("source = %v, want request override", metadata["source"])
	}
	scopes, ok := metadata["scopes"].([]string)
	if !ok || len(scopes) != 1 || scopes[0] != "tool:read" {
		t.Fatalf("scopes = %#v, want copied tool:read scope", metadata["scopes"])
	}
}
