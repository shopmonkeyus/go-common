package string

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskHeaders(t *testing.T) {
	assert.Equal(t, map[string]string{"a": "*"}, MaskHeaders(http.Header{"a": []string{"b"}}, []string{"a"}), "they should be equal")
	assert.Equal(t, map[string]string{"a": "b"}, MaskHeaders(http.Header{"a": []string{"b"}}, []string{"b"}), "they should be equal")
	assert.Equal(t, map[string]string{"a": "*", "b": "*"}, MaskHeaders(http.Header{"a": []string{"b"}, "b": []string{"c"}}, []string{"a", "b"}), "they should be equal")
}
