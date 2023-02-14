package string

import (
	"net/http"
	"strings"
)

// MaskHeaders will return a stringified version of headers
// masking the headers passed in by name
func MaskHeaders(h http.Header, maskHeaders []string) map[string]string {
	hh := map[string]string{}
	for key, hk := range h {
		if Contains(maskHeaders, key, true) {
			hh[key] = Mask(strings.Join(hk, ", "))
		} else {
			hh[key] = strings.Join(hk, ", ")
		}
	}
	return hh
}
