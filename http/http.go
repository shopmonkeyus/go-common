package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	ghttp "net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/shopmonkeyus/go-common/dns"
	cstr "github.com/shopmonkeyus/go-common/string"
	"golang.org/x/sync/semaphore"
)

var ErrTooManyAttempts = errors.New("too many attempts")

const (
	userAgentHeaderValue = "Shopmonkey (+https://shopmonkey.io)"
)

type Request interface {
	URL() string
	Headers() map[string]string
	Payload() []byte
	MaxAttempts() uint
}

type HTTPRequest struct {
	url         string
	headers     map[string]string
	payload     []byte
	maxAttempts uint
}

func (r *HTTPRequest) URL() string {
	return r.url
}

func (r *HTTPRequest) Headers() map[string]string {
	return r.headers
}

func (r *HTTPRequest) Payload() []byte {
	return r.payload
}

func (r *HTTPRequest) MaxAttempts() uint {
	return r.maxAttempts
}

// NewHTTPRequest creates a new HTTPRequest that implements the Request interface.
func NewHTTPRequest(url string, headers map[string]string, payload []byte, maxAttempts uint) Request {
	return &HTTPRequest{url, headers, payload, maxAttempts}
}

type Response struct {
	StatusCode int               `json:"statusCode"`
	Body       []byte            `json:"body,omitempty"`
	Headers    map[string]string `json:"headers"`
	Attempts   uint              `json:"attempts"`
	Latency    time.Duration     `json:"latency"`
}

// Recorder is an interface for recording request / responses.
type Recorder interface {
	OnResponse(ctx context.Context, req Request, resp *Response)
}

type Http interface {
	// Deliver sends a payload to a URL with the given headers.
	Deliver(ctx context.Context, request Request) (*Response, error)
}

type http struct {
	transport *ghttp.Transport
	timeout   time.Duration // set for testing but defaults to 55 seconds otherwise
	dur       time.Duration // set for testing but defaults to 1 second otherwise
	recorder  Recorder
	count     uint64
	semaphore *semaphore.Weighted
}

var _ Http = (*http)(nil)

func (h *http) shouldRetry(resp *ghttp.Response, err error) bool {
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "connection reset") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "EOF") {
			return true
		}
	}
	if resp != nil {
		switch resp.StatusCode {
		case ghttp.StatusRequestTimeout, ghttp.StatusBadGateway, ghttp.StatusServiceUnavailable, ghttp.StatusGatewayTimeout, ghttp.StatusTooManyRequests:
			return true
		}
	}
	return false
}

func (h *http) toResponse(resp *ghttp.Response, attempt uint, latency time.Duration) (*Response, error) {
	if resp == nil {
		return nil, nil
	}
	var body []byte
	if resp.Body != nil {
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		body = b
	}
	headers := make(map[string]string)
	for k, v := range resp.Header {
		headers[k] = strings.Join(v, ", ")
	}
	return &Response{
		StatusCode: resp.StatusCode,
		Body:       body,
		Headers:    headers,
		Attempts:   attempt,
		Latency:    latency,
	}, nil
}

var isNumber = regexp.MustCompile("^[0-9]+$")

func (h *http) generateRequestId(req Request) string {
	count := atomic.AddUint64(&h.count, 1)
	return fmt.Sprintf("%d/%s", count, cstr.NewHash(req.URL(), req.Payload(), time.Now().UnixNano()))
}

func (h *http) Deliver(ctx context.Context, req Request) (*Response, error) {
	started := time.Now()
	if err := h.semaphore.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("error acquiring semaphore: %w", err)
	}
	defer h.semaphore.Release(1)
	var attempt uint
	var resp *ghttp.Response
	var response *Response
	var c context.Context
	var cancel context.CancelFunc
	maxAttempts := req.MaxAttempts()
	if maxAttempts == 1 {
		// for testing we want to make sure we don't wait too long
		c, cancel = context.WithTimeout(ctx, time.Second*3)
		// we only want to try once for tests
	} else {
		c, cancel = context.WithTimeout(ctx, h.timeout)
	}
	defer cancel()
	headers := req.Headers()
	for attempt < maxAttempts {
		attempt++
		reqId := h.generateRequestId(req)
		hreq, err := ghttp.NewRequestWithContext(c, ghttp.MethodPost, req.URL(), bytes.NewBuffer(req.Payload()))
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			hreq.Header.Set(k, v)
		}
		hreq.Header.Set("User-Agent", userAgentHeaderValue)
		hreq.Header.Set("X-Request-Id", reqId)
		hreq.Header.Set("X-Attempt", strconv.Itoa(int(attempt)))
		resp, err = h.transport.RoundTrip(hreq)
		if err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) {
			return nil, err
		}
		if h.recorder != nil && resp != nil /*&& !req.TestOnly*/ {
			r, err := h.toResponse(resp, attempt, time.Since(started))
			if err != nil {
				return nil, fmt.Errorf("error converting response: %w", err)
			}
			// update our headers
			headers["User-Agent"] = userAgentHeaderValue
			headers["X-Request-Id"] = reqId
			headers["X-Attempt"] = strconv.Itoa(int(attempt))
			h.recorder.OnResponse(ctx, req, r)
			response = r // set it so we don't try and re-read the body again
		}
		if h.shouldRetry(resp, err) /*&& !req.TestOnly*/ {
			select {
			case <-ctx.Done():
				return nil, context.Canceled
			default:
			}
			if attempt == maxAttempts {
				// don't worry about sleeping and reading the body if we're not going to retry
				break
			}
			ms := h.dur * time.Duration(attempt)
			if resp != nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				ra := resp.Header.Get("Retry-After")
				if ra != "" {
					if isNumber.MatchString(ra) {
						afterSeconds, _ := strconv.Atoi(ra)
						if afterSeconds > 0 {
							ms = time.Second * time.Duration(afterSeconds)
						}
					} else {
						if tv, err := time.Parse(ghttp.TimeFormat, ra); err == nil {
							ms = time.Until(tv)
						}
					}
				}
			}
			if ms > 0 {
				time.Sleep(ms)
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		if response == nil {
			return h.toResponse(resp, attempt, time.Since(started))
		}
		return response, nil
	}
	if response == nil && resp != nil {
		r, err := h.toResponse(resp, attempt, time.Since(started))
		if err != nil {
			return nil, err
		}
		return r, ErrTooManyAttempts
	}
	return response, ErrTooManyAttempts
}

type configOpts struct {
	recorder Recorder
	max      uint64
	dns      dns.DNS
	timeout  time.Duration
	dur      time.Duration
}

type ConfigOpt func(opts *configOpts)

// New returns a new HTTP implementation.
func New(opts ...ConfigOpt) Http {
	var c configOpts
	c.timeout = time.Second * 55
	c.dur = time.Second
	c.max = 100
	for _, opt := range opts {
		opt(&c)
	}
	if c.max <= 0 {
		panic("max was nil")
	}
	var transport *ghttp.Transport
	if c.dns != nil {
		transport = &ghttp.Transport{
			DialContext: func(ctx context.Context, network string, addr string) (conn net.Conn, err error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				ok, ip, err := c.dns.Lookup(ctx, host)
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, fmt.Errorf("dns lookup failed: couldn't find ip for %s", host)
				}
				var dialer net.Dialer
				conn, err = dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				return
			},
		}
	} else {
		transport = ghttp.DefaultTransport.(*ghttp.Transport)
	}
	return &http{
		transport: transport,
		timeout:   c.timeout,
		dur:       c.dur,
		recorder:  c.recorder,
		semaphore: semaphore.NewWeighted(int64(c.max)),
	}
}

// WithDNS sets the dns resolver for the http client.
func WithDNS(dns dns.DNS) ConfigOpt {
	return func(opts *configOpts) {
		opts.dns = dns
	}
}

// WithRecorder sets the recorder for the http client.
func WithRecorder(recorder Recorder) ConfigOpt {
	return func(opts *configOpts) {
		opts.recorder = recorder
	}
}

// WithMax sets the max number of concurrent requests.
func WithMax(max uint64) ConfigOpt {
	return func(opts *configOpts) {
		opts.max = max
	}
}

// WithTimeout sets the timeout for the http client.
func WithTimeout(timeout time.Duration) ConfigOpt {
	return func(opts *configOpts) {
		opts.timeout = timeout
	}
}

// WithDuration sets the duration for the http client.
func WithDuration(dur time.Duration) ConfigOpt {
	return func(opts *configOpts) {
		opts.dur = dur
	}
}
