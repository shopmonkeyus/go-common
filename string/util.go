package string

import "strings"

// Contains returns true if the search string is found in the slice
func Contains(slice []string, search string, caseInsensitive bool) bool {
	for _, s := range slice {
		if caseInsensitive {
			if strings.EqualFold(s, search) {
				return true
			}
			continue
		}

		if s == search {
			return true
		}
	}
	return false
}
