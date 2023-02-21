package string

import (
	"crypto/sha256"
	"fmt"
)

// SHA256 will return a sha 256 hash of the data in hex format
func SHA256(data []byte, extra ...[]byte) string {
	h := sha256.New()
	h.Write(data)
	for _, d := range extra {
		h.Write(d)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
