package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/logger"
	cstring "github.com/shopmonkeyus/go-common/string"
)

var ErrTrackerClosed = errors.New("analytics: closed")

var region = os.Getenv("SM_REGION")
var branch = os.Getenv("SM_BRANCH")

type analyticsOpts struct {
	Region     string
	Branch     string
	CompanyId  string
	LocationId string
	UserId     string
	SessionId  string
	RequestId  string
	MessageId  string
	event      Event
	buf        []byte
}

type analyticsOptFn func(opts *analyticsOpts)

func init() {
	if region == "" {
		region = os.Getenv("SM_SUPER_REGION")
		if region == "" {
			region = "dev"
		}
	}
	if branch == "" {
		branch = "dev"
	}
}

// WithRegion will override the region setting on the event
func WithRegion(region string) analyticsOptFn {
	return func(opts *analyticsOpts) {
		opts.Region = region
	}
}

// WithBranch will override the branch setting on the event
func WithBranch(branch string) analyticsOptFn {
	return func(opts *analyticsOpts) {
		opts.Branch = branch
	}
}

// WithCompanyId will set the companyId on the event
func WithCompanyId(companyId string) analyticsOptFn {
	return func(opts *analyticsOpts) {
		opts.CompanyId = companyId
	}
}

// WithLocationId will set the locationId on the event
func WithLocationId(locationId string) analyticsOptFn {
	return func(opts *analyticsOpts) {
		opts.LocationId = locationId
	}
}

// WithUserId will set the userId on the event
func WithUserId(userId string) analyticsOptFn {
	return func(opts *analyticsOpts) {
		opts.UserId = userId
	}
}

// WithSessionId will set the sessionId on the event
func WithSessionId(sessionId string) analyticsOptFn {
	return func(opts *analyticsOpts) {
		opts.SessionId = sessionId
	}
}

// WithRequestId will set the requestId on the event
func WithRequestId(requestId string) analyticsOptFn {
	return func(opts *analyticsOpts) {
		opts.RequestId = requestId
	}
}

// WithMessageId will set the Nats-Msg-Id on the event
func WithMessageId(messageId string) analyticsOptFn {
	return func(opts *analyticsOpts) {
		opts.MessageId = messageId
	}
}

func defaultTrackerOpts() *analyticsOpts {
	return &analyticsOpts{
		Region: region,
		Branch: branch,
	}
}

// Analytics is a background service which is used for delivering analytics events in the background
type Analytics interface {
	// Queue an analytics event which will be delivered in the background
	Queue(name string, action string, data map[string]interface{}, opts ...analyticsOptFn) error

	// Close will flush all pending analytics events and close the background sender
	Close() error
}

type Event struct {
	Timestamp  time.Time              `json:"timestamp"`
	Branch     string                 `json:"branch"`
	Region     string                 `json:"region"`
	Name       string                 `json:"name"`
	Action     string                 `json:"action,omitempty"`
	Data       map[string]interface{} `json:"data,omitempty"`
	CompanyId  *string                `json:"companyId,omitempty"`
	LocationId *string                `json:"locationId,omitempty"`
	UserId     *string                `json:"userId,omitempty"`
	SessionId  *string                `json:"sessionId,omitempty"`
	RequestId  *string                `json:"requestId,omitempty"`
}

var replacer = regexp.MustCompile(`[\.:\s\/\+\*]`)

func safeToken(token string) string {
	return replacer.ReplaceAllString(token, "-")
}

type analytics struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger logger.Logger
	js     nats.JetStreamContext
	events chan analyticsOpts
	wg     sync.WaitGroup
	once   sync.Once
}

var _ Analytics = (*analytics)(nil)

func (t *analytics) Queue(name string, action string, data map[string]interface{}, opts ...analyticsOptFn) error {
	select {
	case <-t.ctx.Done():
		return ErrTrackerClosed
	default:
	}
	config := defaultTrackerOpts()
	for _, fn := range opts {
		fn(config)
	}
	config.event = Event{
		Timestamp: time.Now(),
		Name:      name,
		Action:    action,
		Data:      data,
		Branch:    config.Branch,
		Region:    config.Region,
	}

	if config.CompanyId != "" {
		config.event.CompanyId = &config.CompanyId
	}
	if config.LocationId != "" {
		config.event.LocationId = &config.LocationId
	}
	if config.UserId != "" {
		config.event.UserId = &config.UserId
	}
	if config.SessionId != "" {
		config.event.SessionId = &config.SessionId
	}
	if config.RequestId != "" {
		config.event.RequestId = &config.RequestId
	}

	buf, _ := json.Marshal(config.event)
	msgid := config.MessageId
	if msgid == "" {
		config.MessageId = cstring.SHA256(buf)
	}
	config.buf = buf

	t.events <- *config // send to our channel so we can background send analytics events

	return nil
}

func (t *analytics) Close() error {
	t.once.Do(func() {
		t.cancel()
		t.wg.Wait()
	})
	return nil
}

func (t *analytics) run() {
	t.wg.Add(1)
	defer t.wg.Done()
	for {
		select {
		case config := <-t.events:
			msg := nats.NewMsg(fmt.Sprintf("analytics.%s.%s", safeToken(config.event.Name), safeToken(config.event.Action)))
			msg.Header.Set("Nats-Msg-Id", config.MessageId)
			if config.CompanyId != "" {
				msg.Header.Set("x-company-id", config.CompanyId)
			}
			if config.UserId != "" {
				msg.Header.Set("x-user-id", config.UserId)
			}
			if config.LocationId != "" {
				msg.Header.Set("x-location-id", config.LocationId)
			}
			if config.Region != "" {
				msg.Header.Set("region", config.Region)
			}
			msg.Data = config.buf
			var tries int
			for tries < 3 {
				tries++
				_, err := t.js.PublishMsg(msg)
				if err != nil {
					t.logger.Warn("analytics: failed sending %s. %s (attempts=%d)", msg.Subject, err, tries)
					time.Sleep(time.Millisecond * time.Duration(50*tries))
					continue
				}
				break
			}
		default:
			// if we get here, we have no events in the queue
			select {
			case <-t.ctx.Done():
				return
			default:
				time.Sleep(time.Millisecond * 10) // prevent spin lock
			}
		}
	}
}

// New returns a Tracker instance
func New(ctx context.Context, logger logger.Logger, js nats.JetStreamContext) (Analytics, error) {
	_ctx, cancel := context.WithCancel(ctx)
	t := &analytics{
		ctx:    _ctx,
		cancel: cancel,
		logger: logger,
		js:     js,
		events: make(chan analyticsOpts, 250),
	}
	go t.run() // start background sender
	return t, nil
}
