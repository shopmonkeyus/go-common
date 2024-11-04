package env

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeOSEnvWithSecret(t *testing.T) {
	assert.Equal(t, "FOO=${BAR}", EncodeOSEnvWithSecret("FOO", "BAR"))
}

func TestWriteEnvFile(t *testing.T) {
	fn := filepath.Join(t.TempDir(), "env.txt")
	defer os.Remove(fn)
	err := WriteEnvFile(fn, []EnvLine{{Key: "FOO", Val: "BAR"}, {Key: "BAR", Val: "SECRET", IsSecret: true}})
	assert.NoError(t, err)
	contents, err := os.ReadFile(fn)
	assert.NoError(t, err)
	assert.Equal(t, "FOO=BAR\nBAR=${SECRET}\n", string(contents))
}

func TestEncodeOSEnv(t *testing.T) {
	assert.Equal(t, "FOO=BAR", EncodeOSEnv("FOO", "BAR"))
}

func TestProcessEnvLine(t *testing.T) {
	assert.Equal(t, EnvLine{Key: "FOO", Val: "BAR", IsSecret: false}, ProcessEnvLine("FOO=BAR"))
	assert.Equal(t, EnvLine{Key: "FOO", Val: "BAR", IsSecret: true}, ProcessEnvLine("FOO=${BAR}"))
}
