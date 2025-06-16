package logger

import (
	"os"
	"testing"
)

func TestGetLevelFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     LogLevel
	}{
		{
			name:     "none level",
			envValue: "none",
			want:     LevelNone,
		},
		{
			name:     "trace level",
			envValue: "trace",
			want:     LevelTrace,
		},
		{
			name:     "debug level",
			envValue: "debug",
			want:     LevelDebug,
		},
		{
			name:     "info level",
			envValue: "info",
			want:     LevelInfo,
		},
		{
			name:     "warn level",
			envValue: "warn",
			want:     LevelWarn,
		},
		{
			name:     "error level",
			envValue: "error",
			want:     LevelError,
		},
		{
			name:     "case insensitive",
			envValue: "DEBUG",
			want:     LevelDebug,
		},
		{
			name:     "empty value",
			envValue: "",
			want:     LevelDebug,
		},
		{
			name:     "invalid value",
			envValue: "invalid",
			want:     LevelDebug,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.envValue != "" {
				os.Setenv("SM_LOG_LEVEL", tt.envValue)
			} else {
				os.Unsetenv("SM_LOG_LEVEL")
			}

			// Clean up after test
			defer os.Unsetenv("SM_LOG_LEVEL")

			// Run test
			got := GetLevelFromEnv()
			if got != tt.want {
				t.Errorf("GetLevelFromEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}
