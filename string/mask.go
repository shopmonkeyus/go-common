package string

import (
	"bytes"
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
	var buf bytes.Buffer
	buf.WriteString(s[0:h])
	buf.WriteString(strings.Repeat("*", l-h))
	return buf.String()
}
