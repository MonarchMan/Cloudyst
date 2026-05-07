package safety

type SecurityContext struct {
	UserID   string
	Role     string
	Scopes   []string
	Metadata map[string]any
}

func (s *SecurityContext) HasScope(scope string) bool {
	if s == nil || scope == "" {
		return false
	}
	for _, item := range s.Scopes {
		if item == scope {
			return true
		}
	}
	return false
}

func (s *SecurityContext) MetadataWithIdentity(extra map[string]any) map[string]any {
	if s == nil && len(extra) == 0 {
		return nil
	}
	metadata := make(map[string]any)
	if s != nil {
		for key, value := range s.Metadata {
			metadata[key] = value
		}
		if len(s.Scopes) > 0 {
			metadata["scopes"] = append([]string(nil), s.Scopes...)
		}
	}
	for key, value := range extra {
		metadata[key] = value
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}
