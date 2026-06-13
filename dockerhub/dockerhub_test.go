package dockerhub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(t *testing.T, mux *http.ServeMux) *Client {
	t.Helper()
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	cfg := DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	return NewClient(cfg)
}

func TestGetSendsUserAgent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte(`{}`))
	})

	c := newTestClient(t, mux)
	c.httpClient = &http.Client{Timeout: 5 * time.Second}

	body, err := c.Get(context.Background(), c.baseURL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{}` {
		t.Errorf("body = %q", body)
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})

	c := newTestClient(t, mux)
	c.Retries = 5
	c.httpClient = &http.Client{Timeout: 10 * time.Second}

	start := time.Now()
	body, err := c.Get(context.Background(), c.baseURL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{}` {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestSearchParsesResults(t *testing.T) {
	payload := searchResp{
		Count: 2,
		Results: []searchEntry{
			{
				RepoName:         "library/nginx",
				ShortDescription: "Official Nginx image",
				StarCount:        19000,
				PullCount:        2000000000,
				IsOfficial:       true,
				UpdatedAt:        "2024-01-15T10:30:00Z",
			},
			{
				RepoName:         "bitnami/nginx",
				ShortDescription: "Bitnami Nginx image",
				StarCount:        500,
				PullCount:        10000000,
				IsOfficial:       false,
				UpdatedAt:        "2024-01-10T08:00:00Z",
			},
		},
	}
	data, _ := json.Marshal(payload)

	mux := http.NewServeMux()
	mux.HandleFunc("/search/repositories/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})

	c := newTestClient(t, mux)

	images, err := c.Search(context.Background(), "nginx", 10, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 {
		t.Fatalf("got %d images, want 2", len(images))
	}

	e1 := images[0]
	if e1.Name != "nginx" {
		t.Errorf("name = %q, want nginx", e1.Name)
	}
	if e1.Stars != 19000 {
		t.Errorf("stars = %d, want 19000", e1.Stars)
	}
	if e1.Rank != 1 {
		t.Errorf("rank = %d, want 1", e1.Rank)
	}
	if !e1.Official {
		t.Error("official should be true")
	}
	if e1.Updated != "2024-01-15" {
		t.Errorf("updated = %q, want 2024-01-15", e1.Updated)
	}

	e2 := images[1]
	if e2.Name != "bitnami/nginx" {
		t.Errorf("name = %q, want bitnami/nginx", e2.Name)
	}
	if e2.Rank != 2 {
		t.Errorf("rank = %d, want 2", e2.Rank)
	}
}

func TestTagsParsesResults(t *testing.T) {
	entry := tagEntry{
		Name:       "latest",
		Digest:     "sha256:abc123",
		FullSize:   68000000,
		LastPushed: "2024-01-15T10:30:00Z",
		LastPuller: "anonymous",
		Images: []tagImageInfo{
			{OS: "linux", Architecture: "amd64"},
			{OS: "linux", Architecture: "arm64"},
		},
	}
	payload := tagsResp{Count: 1, Results: []tagEntry{entry}}
	data, _ := json.Marshal(payload)

	mux := http.NewServeMux()
	mux.HandleFunc("/repositories/library/nginx/tags/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})

	c := newTestClient(t, mux)

	tags, err := c.Tags(context.Background(), "nginx", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 {
		t.Fatalf("got %d tags, want 1", len(tags))
	}

	tag := tags[0]
	if tag.Name != "latest" {
		t.Errorf("name = %q, want latest", tag.Name)
	}
	if tag.Digest != "sha256:abc123" {
		t.Errorf("digest = %q", tag.Digest)
	}
	if tag.OS != "linux" {
		t.Errorf("os = %q, want linux", tag.OS)
	}
	if tag.Arch != "amd64" {
		t.Errorf("arch = %q, want amd64 (first manifest only)", tag.Arch)
	}
	if tag.Size != 68000000 {
		t.Errorf("size = %d, want 68000000", tag.Size)
	}
	if tag.LastPushed != "2024-01-15" {
		t.Errorf("last_pushed = %q, want 2024-01-15", tag.LastPushed)
	}
}

func TestImageNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message": "Object not found"}`))
	})

	c := newTestClient(t, mux)
	c.httpClient = &http.Client{Timeout: 5 * time.Second}

	_, err := c.Get(context.Background(), c.baseURL+"/")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestSplitImageName(t *testing.T) {
	cases := []struct {
		in       string
		wantNS   string
		wantRepo string
	}{
		{"nginx", "library", "nginx"},
		{"bitnami/nginx", "bitnami", "nginx"},
		{"my-org/my-app", "my-org", "my-app"},
	}
	for _, tc := range cases {
		ns, repo := splitImageName(tc.in)
		if ns != tc.wantNS {
			t.Errorf("splitImageName(%q) ns = %q, want %q", tc.in, ns, tc.wantNS)
		}
		if repo != tc.wantRepo {
			t.Errorf("splitImageName(%q) repo = %q, want %q", tc.in, repo, tc.wantRepo)
		}
	}
}

func TestFmtDate(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"2024-01-15T10:30:00Z", "2024-01-15"},
		{"", ""},
		{"invalid", "invalid"},
	}
	for _, tc := range cases {
		got := fmtDate(tc.in)
		if got != tc.want {
			t.Errorf("fmtDate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
