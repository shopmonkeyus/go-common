package string

import (
	"strings"
)

func Mask(s string) string {
	l := len(s)
	if l == 0 {
		return s
	}
	if l == 1 {
		return "*"
	}
	h := int(l / 2)
	return s[0:h] + strings.Repeat("*", l-h)
}
