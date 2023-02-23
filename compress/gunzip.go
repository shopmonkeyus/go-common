package compress

import (
	"bytes"
	"compress/gzip"
)

// Gunzip will unzip data and return buffer inline
func Gunzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var resB bytes.Buffer
	_, err = resB.ReadFrom(r)
	if err != nil {
		return nil, err
	}

	return append([]byte(nil), resB.Bytes()...), nil
}
