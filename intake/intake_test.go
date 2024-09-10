package intake

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntake(t *testing.T) {
	dir, err := os.MkdirTemp("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)
	i := NewIntake(dir)
	err = i.Write("test", "test", map[string]string{"Nats-Msg-Id": "i89F6Sk4wtWKndap", "test": "test"})
	assert.NoError(t, err)
	files, err := os.ReadDir(dir)
	assert.NoError(t, err)
	assert.Len(t, files, 1)
	buf, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
	assert.NoError(t, err)
	assert.Equal(t, `{"subject":"test","data":"test","headers":{"Nats-Msg-Id":"i89F6Sk4wtWKndap","test":"test"}}`, string(buf))
	os.Remove(filepath.Join(dir, files[0].Name()))

	h := map[string]string{"test": "test"}
	err = i.Write("test", "test", h)
	assert.NoError(t, err)
	files, err = os.ReadDir(dir)
	assert.NoError(t, err)
	assert.Len(t, files, 1)
	buf, err = os.ReadFile(filepath.Join(dir, files[0].Name()))
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf(`{"subject":"test","data":"test","headers":{"Nats-Msg-Id":"%s","test":"test"}}`, h["Nats-Msg-Id"]), string(buf))
}
