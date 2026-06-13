package dockerhub

import (
	"fmt"
	"strings"
	"time"
)

// Image is the record emitted for Docker Hub image/repository results.
type Image struct {
	Rank        int    `json:"rank"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Stars       int    `json:"star_count"`
	Pulls       int64  `json:"pull_count"`
	Official    bool   `json:"is_official"`
	Automated   bool   `json:"is_automated"`
	Updated     string `json:"updated"`
	URL         string `json:"url"`
}

// Tag is the record emitted for a single image tag.
type Tag struct {
	Name       string `json:"name"`
	Digest     string `json:"digest"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	Size       int64  `json:"full_size"`
	LastPushed string `json:"last_pushed"`
	LastPuller string `json:"last_puller"`
}

// Namespace is the record emitted for a Docker Hub namespace.
type Namespace struct {
	Name string `json:"name"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

// ─── wire types ──────────────────────────────────────────────────────────────

// searchResp is the JSON shape from /v2/search/repositories/.
type searchResp struct {
	Count   int           `json:"count"`
	Next    *string       `json:"next"`
	Results []searchEntry `json:"results"`
}

type searchEntry struct {
	RepoName         string `json:"repo_name"`
	ShortDescription string `json:"short_description"`
	StarCount        int    `json:"star_count"`
	PullCount        int64  `json:"pull_count"`
	IsOfficial       bool   `json:"is_official"`
	IsAutomated      bool   `json:"is_automated"`
	UpdatedAt        string `json:"updated_at"`
}

// repoResp is the JSON shape from /v2/repositories/{namespace}/{name}/.
type repoResp struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Description string `json:"description"`
	StarCount   int    `json:"star_count"`
	PullCount   int64  `json:"pull_count"`
	IsOfficial  bool   `json:"is_official"`
	IsAutomated bool   `json:"is_automated"`
	LastUpdated string `json:"last_updated"`
}

// tagsResp is the JSON shape from /v2/repositories/{namespace}/{name}/tags/.
type tagsResp struct {
	Count   int        `json:"count"`
	Next    *string    `json:"next"`
	Results []tagEntry `json:"results"`
}

type tagEntry struct {
	Name       string         `json:"name"`
	Digest     string         `json:"digest"`
	FullSize   int64          `json:"full_size"`
	LastPushed string         `json:"last_pushed"`
	LastPuller string         `json:"last_puller"`
	Images     []tagImageInfo `json:"images"`
}

type tagImageInfo struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

// reposResp is the JSON shape from /v2/repositories/{namespace}/.
type reposResp struct {
	Count   int         `json:"count"`
	Next    *string     `json:"next"`
	Results []repoEntry `json:"results"`
}

type repoEntry struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Description string `json:"description"`
	StarCount   int    `json:"star_count"`
	PullCount   int64  `json:"pull_count"`
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// splitImageName splits an image name into namespace and repo.
// If name has no "/" it is an official image in the "library" namespace.
func splitImageName(name string) (namespace, repo string) {
	if ns, r, ok := strings.Cut(name, "/"); ok {
		return ns, r
	}
	return "library", name
}

// imageURL returns the canonical Docker Hub URL for an image.
func imageURL(namespace, repo string) string {
	if namespace == "library" {
		return fmt.Sprintf("https://hub.docker.com/_%s/%s", "/", repo)
	}
	return fmt.Sprintf("https://hub.docker.com/r/%s/%s", namespace, repo)
}

// fmtDate parses an RFC3339 timestamp and returns just the date portion.
// Returns the raw string if parsing fails.
func fmtDate(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// try without sub-seconds
		t, err = time.Parse("2006-01-02T15:04:05Z", s)
		if err != nil {
			return s
		}
	}
	return t.UTC().Format("2006-01-02")
}

// searchEntryToImage converts a search result wire entry to an Image record.
func searchEntryToImage(e searchEntry, rank int) Image {
	// repo_name is "namespace/name" for user images, "library/name" for official
	namespace, repo := splitImageName(e.RepoName)
	name := e.RepoName
	if namespace == "library" {
		name = repo
	}
	return Image{
		Rank:        rank,
		Name:        name,
		Description: e.ShortDescription,
		Stars:       e.StarCount,
		Pulls:       e.PullCount,
		Official:    e.IsOfficial,
		Automated:   e.IsAutomated,
		Updated:     fmtDate(e.UpdatedAt),
		URL:         imageURL(namespace, repo),
	}
}

// repoRespToImage converts a repository detail wire type to an Image record.
func repoRespToImage(r repoResp) Image {
	namespace := r.Namespace
	if namespace == "" {
		namespace = "library"
	}
	name := r.Name
	if namespace != "library" {
		name = namespace + "/" + r.Name
	}
	return Image{
		Rank:        0,
		Name:        name,
		Description: r.Description,
		Stars:       r.StarCount,
		Pulls:       r.PullCount,
		Official:    r.IsOfficial,
		Automated:   r.IsAutomated,
		Updated:     fmtDate(r.LastUpdated),
		URL:         imageURL(namespace, r.Name),
	}
}

// repoEntryToImage converts a user-repos list entry to an Image record.
func repoEntryToImage(e repoEntry, rank int) Image {
	namespace := e.Namespace
	if namespace == "" {
		namespace = "library"
	}
	name := e.Name
	if namespace != "library" {
		name = namespace + "/" + e.Name
	}
	return Image{
		Rank:        rank,
		Name:        name,
		Description: e.Description,
		Stars:       e.StarCount,
		Pulls:       e.PullCount,
		URL:         imageURL(namespace, e.Name),
	}
}

// tagEntryToTag converts a tag wire entry to a Tag record.
func tagEntryToTag(e tagEntry) Tag {
	var os, arch string
	if len(e.Images) > 0 {
		os = e.Images[0].OS
		arch = e.Images[0].Architecture
	}
	return Tag{
		Name:       e.Name,
		Digest:     e.Digest,
		OS:         os,
		Arch:       arch,
		Size:       e.FullSize,
		LastPushed: fmtDate(e.LastPushed),
		LastPuller: e.LastPuller,
	}
}
