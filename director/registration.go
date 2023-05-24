package director

import (
	"fmt"
	"regexp"
	"time"
)

type RegistrationStatus string

const (
	UP   = RegistrationStatus("UP")
	DOWN = RegistrationStatus("DOWN")
)

// Registration is a record for director registration
type Registration struct {
	Timestamp time.Time          `json:"timestamp"`
	Status    RegistrationStatus `json:"status"`
	Region    string             `json:"region"`
	IPAddress string             `json:"ipAddress"`
	Port      *int               `json:"port,omitempty"`
	Hostname  string             `json:"hostname"`
}

// IsUP returns true if this is an up status
func (r Registration) IsUP() bool {
	return r.Status == UP
}

// Key should return a suitable cache key using the host ip and port
func (r Registration) Key() string {
	return EncodeHostnameToKey(fmt.Sprintf("%s:%d", r.IPAddress, r.GetPort()))
}

// GetPort will return a port using default if not provided
func (r Registration) GetPort() int {
	port := 8080
	if r.Port != nil {
		port = *r.Port
	}
	return port
}

var replacer = regexp.MustCompile(`[\.:]`)

// EncodeHostnameToKey will replace the . and : characters in a hostname with a dash
func EncodeHostnameToKey(hostname string) string {
	return replacer.ReplaceAllString(hostname, "-")
}
