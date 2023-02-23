package string

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMasking(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"foobar", "foo***"},
		{"foo", "f**"},
		{"f", "*"},
		{"", ""},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Mask(%q)", tc.input), func(t *testing.T) {
			output := Mask(tc.input)
			assert.Equal(t, tc.expected, output)
		})
	}
}
