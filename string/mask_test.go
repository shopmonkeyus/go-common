package string

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMasking(t *testing.T) {
	assert.Equal(t, "foo***", Mask("foobar"), "they should be equal")
	assert.Equal(t, "f**", Mask("foo"), "they should be equal")
	assert.Equal(t, "*", Mask("f"), "they should be equal")
	assert.Equal(t, "", Mask(""), "they should be equal")
}
