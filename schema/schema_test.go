package schema

import (
	"bytes"
	"context"
	"io"
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
	fetcher := &testFetcher{io.NopCloser(bytes.NewReader([]byte(cstr.JSONStringify(m)))), nil, false}
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
