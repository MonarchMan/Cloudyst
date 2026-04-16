package middleware

import "strings"

func IsInWhitelist(path string, whitelist ...string) bool {
	for _, p := range whitelist {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
