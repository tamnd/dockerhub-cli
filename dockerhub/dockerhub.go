// Package dockerhub is the library behind the dhub command: the HTTP client,
// request shaping, and the typed data models for Docker Hub.
//
// The Docker Hub v2 API is open for public image data: no auth required.
// Base URL: https://hub.docker.com/v2
package dockerhub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultUserAgent identifies the client to Docker Hub.
const DefaultUserAgent = "dhub/dev (+https://github.com/tamnd/dockerhub-cli)"

// ErrNotFound is returned when the API returns 404 for a repo or namespace.
var ErrNotFound = errors.New("not found")

// Config holds constructor parameters for NewClient.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Retries   int
	Workers   int
	Timeout   time.Duration
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://hub.docker.com/v2",
		UserAgent: DefaultUserAgent,
		Rate:      100 * time.Millisecond,
		Retries:   3,
		Workers:   8,
		Timeout:   30 * time.Second,
	}
}

// Client talks to the Docker Hub v2 API.
type Client struct {
	// Rate is the minimum time between requests. Zero means no pacing.
	Rate time.Duration
	// Retries is the number of additional attempts on transient errors.
	Retries int
	// Workers is the maximum number of concurrent requests.
	Workers int
	// UserAgent overrides the default User-Agent header.
	UserAgent string

	baseURL    string
	httpClient *http.Client
	mu         sync.Mutex
	last       time.Time
}

// NewClient returns a Client. An optional Config may be provided to override
// defaults; if none is given, DefaultConfig is used.
func NewClient(cfgs ...Config) *Client {
	cfg := DefaultConfig()
	if len(cfgs) > 0 {
		c := cfgs[0]
		if c.BaseURL != "" {
			cfg.BaseURL = c.BaseURL
		}
		if c.UserAgent != "" {
			cfg.UserAgent = c.UserAgent
		}
		if c.Rate != 0 {
			cfg.Rate = c.Rate
		}
		if c.Retries != 0 {
			cfg.Retries = c.Retries
		}
		if c.Workers != 0 {
			cfg.Workers = c.Workers
		}
		if c.Timeout != 0 {
			cfg.Timeout = c.Timeout
		}
	}
	return &Client{
		baseURL:    cfg.BaseURL,
		httpClient: &http.Client{Timeout: cfg.Timeout},
		UserAgent:  cfg.UserAgent,
		Rate:       cfg.Rate,
		Retries:    cfg.Retries,
		Workers:    cfg.Workers,
	}
}

// Get fetches a URL with pacing and retries, returning the raw body bytes.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	ua := c.UserAgent
	if ua == "" {
		ua = DefaultUserAgent
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, ErrNotFound
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// getJSON fetches and JSON-decodes into v.
func (c *Client) getJSON(ctx context.Context, rawURL string, v any) error {
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return err
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "null" {
		return ErrNotFound
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decode %s: %w", rawURL, err)
	}
	return nil
}

// ─── API methods ─────────────────────────────────────────────────────────────

// Search searches Docker Hub repositories by query.
// onlyOfficial and onlyAutomated are client-side filters applied after fetch.
func (c *Client) Search(ctx context.Context, query string, limit int, onlyOfficial, onlyAutomated bool) ([]Image, error) {
	if limit <= 0 {
		limit = 25
	}
	pageSize := limit
	if pageSize > 100 {
		pageSize = 100
	}

	params := url.Values{}
	params.Set("query", query)
	params.Set("page_size", strconv.Itoa(pageSize))

	var out []Image
	page := 1
	for {
		params.Set("page", strconv.Itoa(page))
		rawURL := c.baseURL + "/search/repositories/?" + params.Encode()
		var resp searchResp
		if err := c.getJSON(ctx, rawURL, &resp); err != nil {
			return out, err
		}
		for _, e := range resp.Results {
			if onlyOfficial && !e.IsOfficial {
				continue
			}
			if onlyAutomated && !e.IsAutomated {
				continue
			}
			out = append(out, searchEntryToImage(e, len(out)+1))
			if len(out) >= limit {
				return out, nil
			}
		}
		if resp.Next == nil || len(resp.Results) == 0 {
			break
		}
		page++
	}
	return out, nil
}

// ImageDetail fetches metadata for a single image.
// name can be "nginx" (official) or "user/repo".
func (c *Client) ImageDetail(ctx context.Context, name string) (Image, error) {
	namespace, repo := splitImageName(name)
	rawURL := fmt.Sprintf("%s/repositories/%s/%s/", c.baseURL, url.PathEscape(namespace), url.PathEscape(repo))
	var resp repoResp
	if err := c.getJSON(ctx, rawURL, &resp); err != nil {
		return Image{}, fmt.Errorf("image %q: %w", name, err)
	}
	return repoRespToImage(resp), nil
}

// Tags lists tags for an image.
// name can be "nginx" (official) or "user/repo".
func (c *Client) Tags(ctx context.Context, name string, limit int) ([]Tag, error) {
	if limit <= 0 {
		limit = 25
	}
	namespace, repo := splitImageName(name)
	pageSize := limit
	if pageSize > 100 {
		pageSize = 100
	}

	params := url.Values{}
	params.Set("page_size", strconv.Itoa(pageSize))

	var out []Tag
	page := 1
	for {
		params.Set("page", strconv.Itoa(page))
		rawURL := fmt.Sprintf("%s/repositories/%s/%s/tags/?%s",
			c.baseURL, url.PathEscape(namespace), url.PathEscape(repo), params.Encode())
		var resp tagsResp
		if err := c.getJSON(ctx, rawURL, &resp); err != nil {
			return out, fmt.Errorf("tags %q: %w", name, err)
		}
		for _, e := range resp.Results {
			out = append(out, tagEntryToTag(e))
			if len(out) >= limit {
				return out, nil
			}
		}
		if resp.Next == nil || len(resp.Results) == 0 {
			break
		}
		page++
	}
	return out, nil
}

// Official lists Docker Official Images (library namespace).
func (c *Client) Official(ctx context.Context, limit int) ([]Image, error) {
	if limit <= 0 {
		limit = 25
	}
	pageSize := 100

	params := url.Values{}
	params.Set("page_size", strconv.Itoa(pageSize))

	var out []Image
	page := 1
	for {
		params.Set("page", strconv.Itoa(page))
		rawURL := c.baseURL + "/repositories/library/?" + params.Encode()
		var resp reposResp
		if err := c.getJSON(ctx, rawURL, &resp); err != nil {
			return out, err
		}
		for _, e := range resp.Results {
			if e.Namespace == "" {
				e.Namespace = "library"
			}
			out = append(out, repoEntryToImage(e, len(out)+1))
			if len(out) >= limit {
				return out, nil
			}
		}
		if resp.Next == nil || len(resp.Results) == 0 {
			break
		}
		page++
	}
	return out, nil
}

// UserRepos lists public repos for a user or organisation.
func (c *Client) UserRepos(ctx context.Context, username string, limit int) ([]Image, error) {
	if limit <= 0 {
		limit = 25
	}
	pageSize := limit
	if pageSize > 100 {
		pageSize = 100
	}

	params := url.Values{}
	params.Set("page_size", strconv.Itoa(pageSize))

	var out []Image
	page := 1
	for {
		params.Set("page", strconv.Itoa(page))
		rawURL := fmt.Sprintf("%s/repositories/%s/?%s", c.baseURL, url.PathEscape(username), params.Encode())
		var resp reposResp
		if err := c.getJSON(ctx, rawURL, &resp); err != nil {
			return out, fmt.Errorf("user %q: %w", username, err)
		}
		for _, e := range resp.Results {
			if e.Namespace == "" {
				e.Namespace = username
			}
			out = append(out, repoEntryToImage(e, len(out)+1))
			if len(out) >= limit {
				return out, nil
			}
		}
		if resp.Next == nil || len(resp.Results) == 0 {
			break
		}
		page++
	}
	return out, nil
}
