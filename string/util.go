package string

import "strings"

// Contains returns true if needle is found in haystack based on caseinsensitive
func Contains(haystack []string, needle string, caseinsensitive bool) bool {
	for _, v := range haystack {
		if caseinsensitive && strings.EqualFold(v, needle) {
			return true
		}
		if v == needle {
			return true
		}
	}
	return false
}
