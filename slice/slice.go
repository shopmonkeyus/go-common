package slice

// Contains returns true if the slice contains the value.
func Contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// ContainsAny returns true if the slice contain any of the values [val].
func ContainsAny(slice []string, val ...string) bool {
	for _, s := range slice {
		if Contains(val, s) {
			return true
		}
	}
	return false
}

// Omit returns a new slice with the values [val] omitted.
func Omit(slice []string, val ...string) []string {
	var result []string
	for _, s := range slice {
		if !Contains(val, s) {
			result = append(result, s)
		}
	}
	return result
}
