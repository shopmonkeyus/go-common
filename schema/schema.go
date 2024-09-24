package schema

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"
)

const defaultExpireAfter = 30 * time.Minute

type Cache struct {
	TTL int `json:"ttl"`
}

type Changefeed struct {
	AppendFieldsToSubject []string `json:"appendFieldsToSubject"`
	PartitionKeys         []string `json:"partitionKeys"`
}

type Model struct {
	Cache        *Cache      `json:"cache"`
	Changefeed   *Changefeed `json:"changefeed"`
	ModelVersion string      `json:"modelVersion"`
	Public       bool        `json:"public"`
}

type Result struct {
	Success bool   `json:"success"`
	Model   *Model `json:"data"`
}

type Fetcher interface {
	FetchTable(ctx context.Context, table string) (io.ReadCloser, error)
}

type ModelRegistry interface {
	Get(ctx context.Context, table string) (*Model, error)
}

type APIFetcher struct {
	URL    string
	APIKey string
}

var _ Fetcher = (*APIFetcher)(nil)

func (f *APIFetcher) FetchTable(ctx context.Context, table string) (io.ReadCloser, error) {
	u, err := url.Parse(f.URL)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join("v3/schema/private/schema", table)
	u.RawQuery = url.Values{"apikey": {f.APIKey}}.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch model: %s", resp.Status)
	}

	return resp.Body, nil
}

// NewAPIFetcher creates a new API fetcher using the provided URL and API key.
func NewAPIFetcher(url, apiKey string) *APIFetcher {
	return &APIFetcher{URL: url, APIKey: apiKey}
}

type modelCache struct {
	model   *Model
	fetched time.Time
}

type modelRegistry struct {
	models      map[string]*modelCache
	lock        sync.Mutex
	fetcher     Fetcher
	expireAfter time.Duration
}

var _ ModelRegistry = (*modelRegistry)(nil)

func (r *modelRegistry) Get(ctx context.Context, table string) (*Model, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if cache, ok := r.models[table]; ok {
		if time.Since(cache.fetched) < r.expireAfter {
			return cache.model, nil
		}
	}

	rc, err := r.fetcher.FetchTable(ctx, table)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var res Result
	if err := json.NewDecoder(rc).Decode(&res); err != nil {
		return nil, err
	}

	if !res.Success {
		return nil, fmt.Errorf("failed to fetch model: %s", table)
	}

	item := &modelCache{
		model:   res.Model,
		fetched: time.Now(),
	}
	r.models[table] = item

	return item.model, nil
}

// NewModelRegistry creates a new model registry using the provided fetcher.
func NewModelRegistry(fetcher Fetcher, opts ...WithOption) ModelRegistry {
	opt := &ModelRegistryOption{
		ExpireAfter: defaultExpireAfter,
	}

	for _, fn := range opts {
		fn(opt)
	}

	return &modelRegistry{
		models:      make(map[string]*modelCache),
		fetcher:     fetcher,
		expireAfter: opt.ExpireAfter,
	}
}

type ModelRegistryOption struct {
	ExpireAfter time.Duration
}

type WithOption func(opt *ModelRegistryOption)

// WithExpireAfter sets the expire after duration for an item in the model registry.
func WithExpireAfter(expireAfter time.Duration) WithOption {
	return func(opt *ModelRegistryOption) {
		opt.ExpireAfter = expireAfter
	}
}
