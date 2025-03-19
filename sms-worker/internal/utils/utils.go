package utils

import (
	"strings"
)

func IsNilEmptyOrWhitespace(s *string) bool {
	// Check if the pointer is nil.
	if s == nil {
		return true
	}
	// Check if the string is empty or only contains whitespace.
	return strings.TrimSpace(*s) == ""
}
