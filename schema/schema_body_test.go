package schema

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type closeRecorder struct {
	*strings.Reader
	closed        bool
	unreadAtClose int
}

func (c *closeRecorder) Close() error {
	c.closed = true
	c.unreadAtClose = c.Len()
	return nil
}

type stubTransport struct {
	status int
	body   *closeRecorder
}

func (s *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: s.status,
		Status:     http.StatusText(s.status),
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       s.body,
		Request:    req,
	}, nil
}

// APIFetcher calls http.DefaultClient directly, so the stub replaces its transport for the test.
func useStubTransport(t *testing.T, status int, body string) *closeRecorder {
	rec := &closeRecorder{Reader: strings.NewReader(body)}
	orig := http.DefaultClient.Transport
	http.DefaultClient.Transport = &stubTransport{status: status, body: rec}
	t.Cleanup(func() { http.DefaultClient.Transport = orig })
	return rec
}

// The padding keeps bytes unread after the JSON decoder stops.
var padding = strings.Repeat(" ", 8192)

func TestFetchTableDrainsAndClosesBodyOnBadStatus(t *testing.T) {
	rec := useStubTransport(t, http.StatusInternalServerError, "oops"+padding)
	rc, err := NewAPIFetcher("http://schema.test", "key").FetchTable(context.Background(), "table")
	assert.Error(t, err)
	assert.Nil(t, rc)
	assert.True(t, rec.closed, "response body was not closed")
	assert.Zero(t, rec.unreadAtClose, "response body was not read to the end before close")
}

func TestModelRegistryGetDrainsAndClosesBody(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"Success", `{"success":true,"data":{"modelVersion":"1"}}` + padding, false},
		{"NotSuccess", `{"success":false}` + padding, true},
		{"BadJSON", `not json` + padding, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := useStubTransport(t, http.StatusOK, tt.body)
			_, err := NewModelRegistry(NewAPIFetcher("http://schema.test", "key")).Get(context.Background(), "table")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.True(t, rec.closed, "response body was not closed")
			assert.Zero(t, rec.unreadAtClose, "response body was not read to the end before close")
		})
	}
}
