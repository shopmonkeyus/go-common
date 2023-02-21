package string

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSHA256(t *testing.T) {
	assert.Equal(t, "c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2", SHA256([]byte("foobar")), "should hash")
	assert.Equal(t, "c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2", SHA256([]byte("foo"), []byte("bar")), "should hash")
	assert.Equal(t, "c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2", SHA256([]byte("f"), []byte("o"), []byte("o"), []byte("b"), []byte("a"), []byte("r")), "should hash")
}
