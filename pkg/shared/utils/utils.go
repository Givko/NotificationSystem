package utils

import (
	"strings"
)

// IsNilEmptyOrWhitespace checks if a string pointer is nil, empty, or only contains whitespace.
// If the pointer is nil, the function returns true.
func IsNilEmptyOrWhitespace(s *string) bool {
	// Check if the pointer is nil.
	if s == nil {
		return true
	}
	// Check if the string is empty or only contains whitespace.
	return strings.TrimSpace(*s) == ""
}
