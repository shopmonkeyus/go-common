package intake

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	cstr "github.com/shopmonkeyus/go-common/string"
)

type Intake struct {
	dir string
}

type intakeEvent struct {
	Subject string            `json:"subject"`
	Data    any               `json:"data"`
	Headers map[string]string `json:"headers"`
}

// Write will write an event to disk.
func (i *Intake) Write(subject string, data any, headers map[string]string) error {
	var event intakeEvent
	event.Subject = subject
	event.Data = data
	event.Headers = headers
	msgId := headers["Nats-Msg-Id"]
	if msgId == "" {
		id, err := cstr.GenerateRandomString(16)
		if err != nil {
			return fmt.Errorf("failed to generate random string for msg id: %w", err)
		}
		msgId = id
		headers["Nats-Msg-Id"] = msgId
	}
	fn := filepath.Join(i.dir, strings.ReplaceAll(fmt.Sprintf("%d-%s.json", time.Now().UnixNano(), msgId), "/", "-"))
	return os.WriteFile(fn, []byte(cstr.JSONStringify(event)), 0600)
}

// NewIntake creates a new Intake instance.
func NewIntake(dir string) *Intake {
	return &Intake{
		dir: dir,
	}
}
