package string

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContains(t *testing.T) {
	assert.True(t, Contains([]string{"A", "B", "C"}, "A", false), "they should match")
	assert.False(t, Contains([]string{"A", "B", "C"}, "D", false), "they should not match")
	assert.True(t, Contains([]string{"A", "B", "C"}, "a", true), "they should match")
	assert.False(t, Contains([]string{"A", "B", "C"}, "f", true), "they should not match")
}
