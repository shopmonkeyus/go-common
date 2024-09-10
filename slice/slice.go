package slice

// SliceContains returns true if the slice contains the value.
func SliceContains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// SliceContainsAny returns true if the slice contain any of the values [val].
func SliceContainsAny(slice []string, val ...string) bool {
	for _, s := range slice {
		if SliceContains(val, s) {
			return true
		}
	}
	return false
}

// SliceOmit returns a new slice with the values [val] omitted.
func SliceOmit(slice []string, val ...string) []string {
	var result []string
	for _, s := range slice {
		if !SliceContains(val, s) {
			result = append(result, s)
		}
	}
	return result
}
