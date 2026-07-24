// Package github implements the GitHub source-repository adapter.
package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/y-writings/driftline/src/internal/driftline"
)

type Client struct {
	apiBase    string
	token      string
	httpClient *http.Client
}

func NewClientFromEnv() *Client {
	return &Client{
		apiBase: "https://api.github.com",
		token:   os.Getenv("GITHUB_TOKEN"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) ResolveDefaultRef(repository string) (string, string, error) {
	if err := driftline.ValidateRepository(repository); err != nil {
		return "", "", err
	}
	var body struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := c.getJSON("/repos/"+repository, &body); err != nil {
		return "", "", err
	}
	if body.DefaultBranch == "" {
		return "", "", fmt.Errorf("github repository %s has no default branch", repository)
	}
	commit, err := c.ResolveRef(repository, body.DefaultBranch)
	if err != nil {
		return "", "", err
	}
	return body.DefaultBranch, commit, nil
}

func (c *Client) ResolveRef(repository string, ref string) (string, error) {
	if err := driftline.ValidateRepository(repository); err != nil {
		return "", err
	}
	if err := driftline.ValidateRef(ref); err != nil {
		return "", err
	}
	var body struct {
		SHA string `json:"sha"`
	}
	path := "/repos/" + repository + "/commits/" + url.PathEscape(ref)
	if err := c.getJSON(path, &body); err != nil {
		return "", err
	}
	if body.SHA == "" {
		return "", fmt.Errorf("github ref %q did not resolve to a commit", ref)
	}
	return body.SHA, nil
}

func (c *Client) ReadFile(repository string, commit string, path string) ([]byte, error) {
	if err := driftline.ValidateRepository(repository); err != nil {
		return nil, err
	}
	if err := driftline.ValidateRef(commit); err != nil {
		return nil, err
	}
	if err := driftline.ValidateConfigPath(path, "source"); err != nil {
		return nil, err
	}
	endpoint := "/repos/" + repository + "/contents/" + escapePathSegments(path) + "?ref=" + url.QueryEscape(commit)
	req, err := c.newRequest(http.MethodGet, endpoint)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github read file: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		io.Copy(io.Discard, res.Body)
		if res.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("github read file: %w", os.ErrNotExist)
		}
		return nil, c.httpError(res, "github read file")
	}
	return io.ReadAll(res.Body)
}

func (c *Client) newRequest(method string, endpoint string) (*http.Request, error) {
	base := strings.TrimRight(c.apiBase, "/")
	req, err := http.NewRequest(method, base+endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "driftline")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

func (c *Client) getJSON(endpoint string, out any) error {
	req, err := c.newRequest(http.MethodGet, endpoint)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		io.Copy(io.Discard, res.Body)
		return c.httpError(res, "github request")
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("decode github response: %w", err)
	}
	return nil
}

func (c *Client) httpError(res *http.Response, context string) error {
	if res.StatusCode == http.StatusForbidden && res.Header.Get("X-RateLimit-Remaining") == "0" {
		return fmt.Errorf("%s: rate limit exceeded; set GITHUB_TOKEN", context)
	}
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%s: repository inaccessible; set GITHUB_TOKEN", context)
	}
	if res.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%s: not found", context)
	}
	if res.StatusCode == http.StatusRequestEntityTooLarge {
		return fmt.Errorf("%s: source file is too large for GitHub Contents API", context)
	}
	return fmt.Errorf("%s: github returned %s", context, res.Status)
}

func escapePathSegments(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
