package dns

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shopmonkeyus/go-common/cache"
	"github.com/stretchr/testify/assert"
)

type closeRecorder struct {
	io.Reader
	closed bool
}

func (c *closeRecorder) Close() error {
	c.closed = true
	return nil
}

type stubTransport struct {
	status int
	body   *closeRecorder
}

func (s *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: s.status,
		Header:     http.Header{"Content-Type": {"application/dns-json"}},
		Body:       s.body,
		Request:    req,
	}, nil
}

// Lookup calls http.DefaultClient directly, so the stub replaces its transport for the test.
func useStubTransport(t *testing.T, status int, body string) *closeRecorder {
	rec := &closeRecorder{Reader: strings.NewReader(body)}
	orig := http.DefaultClient.Transport
	http.DefaultClient.Transport = &stubTransport{status: status, body: rec}
	t.Cleanup(func() { http.DefaultClient.Transport = orig })
	return rec
}

func TestLookupClosesResponseBody(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{"Success", http.StatusOK, `{"Status":0,"Answer":[{"name":"example.com","type":1,"ttl":60,"data":"93.184.216.34"}]}`, false},
		{"BadStatus", http.StatusInternalServerError, `oops`, true},
		{"BadJSON", http.StatusOK, `not json`, true},
		{"LookupFailed", http.StatusOK, `{"Status":3}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := useStubTransport(t, tt.status, tt.body)
			c := cache.NewInMemory(context.Background(), time.Second)
			defer c.Close()
			_, _, err := New(c).Lookup(context.Background(), "example.com")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.True(t, rec.closed, "response body was not closed")
		})
	}
}
