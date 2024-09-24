package schema

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	cstr "github.com/shopmonkeyus/go-common/string"
	"github.com/stretchr/testify/assert"
)

type testFetcher struct {
	reader io.ReadCloser
	err    error
	called bool
}

func (t *testFetcher) FetchTable(ctx context.Context, table string) (io.ReadCloser, error) {
	t.called = true
	return t.reader, t.err
}

func TestNewModelRegistry(t *testing.T) {
	var m Model
	m.Public = true
	m.ModelVersion = "1234"
	fetcher := &testFetcher{io.NopCloser(bytes.NewReader([]byte(cstr.JSONStringify(Result{Success: true, Model: &m})))), nil, false}
	r := NewModelRegistry(fetcher)
	model, err := r.Get(context.Background(), "table")
	if err != nil {
		t.Fatal(err)
	}
	assert.True(t, fetcher.called)
	if model.Public != m.Public || model.ModelVersion != m.ModelVersion {
		t.Fatalf("expected model to be %v, got %v", m, model)
	}
	fetcher.called = false
	model, err = r.Get(context.Background(), "table")
	if err != nil {
		t.Fatal(err)
	}
	assert.False(t, fetcher.called)
	if model.Public != m.Public || model.ModelVersion != m.ModelVersion {
		t.Fatalf("expected model to be %v, got %v", m, model)
	}
}

func TestNewModelRegistryWithAPIFetcher(t *testing.T) {
	var m Model
	m.Public = true
	m.ModelVersion = "1234"
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		assert.Equal(t, "/v3/schema/private/schema/table", r.URL.Path)
		if r.URL.Query().Get("apikey") != "test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, cstr.JSONStringify(Result{Success: true, Model: &m}))
	}))
	defer ts.Close()
	fetcher := NewAPIFetcher(ts.URL, "test")
	r := NewModelRegistry(fetcher)
	model, err := r.Get(context.Background(), "table")
	if err != nil {
		t.Fatal(err)
	}
	assert.True(t, called)
	if model.Public != m.Public || model.ModelVersion != m.ModelVersion {
		t.Fatalf("expected model to be %v, got %v", m, model)
	}
	called = false
	model, err = r.Get(context.Background(), "table")
	if err != nil {
		t.Fatal(err)
	}
	assert.False(t, called)
	if model.Public != m.Public || model.ModelVersion != m.ModelVersion {
		t.Fatalf("expected model to be %v, got %v", m, model)
	}
}
