package director

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/shopmonkeyus/go-common/logger"
)

type DirectorRegister interface {
	// Close will unregister and then shutdown
	Close() error
}

type directorRegistration struct {
	logger   logger.Logger
	hostname string
	hostIP   string
	port     int
	config   directorRegistrationOpts
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

var _ DirectorRegister = (*directorRegistration)(nil)

func (r *directorRegistration) Close() error {
	r.unregister()
	r.cancel()
	r.wg.Wait() // wait for it to finish
	return nil
}

func (r *directorRegistration) do(registration Registration) error {
	u, err := url.JoinPath(r.config.url, "/--/register")
	if err != nil {
		return err
	}
	buf, err := json.Marshal(registration)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(r.ctx, "POST", u, bytes.NewBuffer(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "go-common (+https://shopmonkey.io)")
	if r.config.authorization != "" {
		req.Header.Set("Authorization", r.config.authorization)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("status code: %d (%s)", res.StatusCode, string(body))
	} else {
		io.Copy(io.Discard, res.Body)
		r.logger.Trace("%s: %s => %s:%d (%d)", registration.Status, registration.Hostname, registration.IPAddress, registration.GetPort(), res.StatusCode)
	}
	return nil
}

func (r *directorRegistration) register() error {
	var registration Registration
	if r.config.timestamp.IsZero() {
		registration.Timestamp = time.Now()
	} else {
		registration.Timestamp = r.config.timestamp
	}
	registration.Status = UP
	registration.Hostname = r.hostname
	registration.IPAddress = r.hostIP
	registration.Port = &r.port
	registration.Region = r.config.region
	return r.do(registration)
}

func (r *directorRegistration) unregister() error {
	var registration Registration
	if r.config.timestamp.IsZero() {
		registration.Timestamp = time.Now()
	} else {
		registration.Timestamp = r.config.timestamp
	}
	registration.Status = DOWN
	registration.Hostname = r.hostname
	registration.IPAddress = r.hostIP
	registration.Port = &r.port
	registration.Region = r.config.region
	return r.do(registration)
}

func (r *directorRegistration) run() {
	defer r.wg.Done()
	ticker := time.NewTicker(r.config.interval)
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			if err := r.register(); err != nil {
				r.logger.Error("register: %s", err)
			}
		}
	}
}

type directorRegistrationOpts struct {
	url           string
	interval      time.Duration
	region        string
	authorization string
	timestamp     time.Time // for testing only
}

type DirectorOptsFunc func(config *directorRegistrationOpts) error

func defaultDirectorOpts() directorRegistrationOpts {
	region := os.Getenv("SM_SUPER_REGION")
	if region == "" {
		region = "dev"
	}
	return directorRegistrationOpts{
		url:      "https://api.shopmonkey.cloud",
		interval: time.Minute * 50,
		region:   region,
	}
}

func withTimestamp(tv time.Time) DirectorOptsFunc {
	return func(config *directorRegistrationOpts) error {
		config.timestamp = tv
		return nil
	}
}

// WithURL will allow the director url to be overriden from the default value
func WithURL(url string) DirectorOptsFunc {
	return func(config *directorRegistrationOpts) error {
		config.url = url
		return nil
	}
}

// WithInterval will change the duration renewal time
func WithInterval(interval time.Duration) DirectorOptsFunc {
	return func(config *directorRegistrationOpts) error {
		config.interval = interval
		return nil
	}
}

// WithRegion will change the region
func WithRegion(region string) DirectorOptsFunc {
	return func(config *directorRegistrationOpts) error {
		config.region = region
		return nil
	}
}

// WithAuthorization will set the authorization
func WithAuthorization(authorization string) DirectorOptsFunc {
	return func(config *directorRegistrationOpts) error {
		config.authorization = authorization
		return nil
	}
}

var ErrInvalidHostname = errors.New("invalid hostname")
var ErrInvalidHostIP = errors.New("invalid hostname or ip address")
var ErrInvalidPort = errors.New("invalid port")

// NewRegistration will create a director registration for hostname with upstream hostIP and port
func NewRegistration(logger logger.Logger, hostname string, hostIP string, port int, opts ...DirectorOptsFunc) (DirectorRegister, error) {
	if hostname == "" {
		return nil, ErrInvalidHostname
	}
	if hostIP == "" {
		return nil, ErrInvalidHostIP
	}
	if port <= 0 {
		return nil, ErrInvalidPort
	}
	config := defaultDirectorOpts()
	for _, fn := range opts {
		if err := fn(&config); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	director := &directorRegistration{
		logger:   logger.WithPrefix("[director-reg]"),
		hostname: hostname,
		hostIP:   hostIP,
		port:     port,
		config:   config,
		ctx:      ctx,
		cancel:   cancel,
	}
	if err := director.register(); err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}
	director.wg.Add(1)
	go director.run()
	return director, nil
}
