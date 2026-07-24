package github

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"time"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func TestClientResolveDefaultRefUsesTokenAndUserAgent(t *testing.T) {
	os.Setenv("GITHUB_TOKEN", "secret-token")
	t.Cleanup(func() { os.Unsetenv("GITHUB_TOKEN") })

	var sawAuth, sawAgent bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer secret-token" {
			sawAuth = true
		}
		if strings.Contains(r.Header.Get("User-Agent"), "driftline") {
			sawAgent = true
		}
		switch r.URL.Path {
		case "/repos/y-writings/source-repo":
			w.Write([]byte(`{"default_branch":"main"}`))
		case "/repos/y-writings/source-repo/commits/main":
			w.Write([]byte(`{"sha":"0123456789abcdef0123456789abcdef01234567"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := NewClientFromEnv()
	client.apiBase = server.URL
	ref, commit, err := client.ResolveDefaultRef("y-writings/source-repo")
	if err != nil {
		t.Fatalf("resolve default ref failed: %v", err)
	}
	if ref != "main" || commit != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("unexpected ref/commit: %s %s", ref, commit)
	}
	if !sawAuth || !sawAgent {
		t.Fatalf("expected auth and user-agent headers, auth=%v agent=%v", sawAuth, sawAgent)
	}
}

func TestClientResolveRefEscapesSlashRef(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.RequestURI, "/repos/y-writings/source-repo/commits/feature%2Ffoo") {
			t.Fatalf("unexpected request URI: %s", r.RequestURI)
		}
		w.Write([]byte(`{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClientFromEnv()
	client.apiBase = server.URL
	commit, err := client.ResolveRef("y-writings/source-repo", "feature/foo")
	if err != nil {
		t.Fatalf("resolve ref failed: %v", err)
	}
	if commit != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected commit: %s", commit)
	}
}

func TestClientReadFileUsesContentsAPIAtCommit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/repos/y-writings/source-repo/contents/templates/my%20file.txt" {
			t.Fatalf("unexpected path: %s", r.URL.EscapedPath())
		}
		if r.URL.Query().Get("ref") != "0123456789abcdef0123456789abcdef01234567" {
			t.Fatalf("unexpected ref query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("Accept") != "application/vnd.github.raw+json" {
			t.Fatalf("unexpected Accept: %s", r.Header.Get("Accept"))
		}
		w.Write([]byte("file bytes\n"))
	}))
	t.Cleanup(server.Close)

	client := NewClientFromEnv()
	client.apiBase = server.URL
	data, err := client.ReadFile("y-writings/source-repo", "0123456789abcdef0123456789abcdef01234567", "templates/my file.txt")
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	if string(data) != "file bytes\n" {
		t.Fatalf("unexpected data: %q", data)
	}
}

func TestClientReadFileClassifiesOnlyNotFoundStatus(t *testing.T) {
	for _, tt := range []struct {
		name         string
		status       int
		wantNotFound bool
	}{
		{name: "not found", status: http.StatusNotFound, wantNotFound: true},
		{name: "server error", status: http.StatusInternalServerError, wantNotFound: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(server.Close)

			client := NewClientFromEnv()
			client.apiBase = server.URL
			_, err := client.ReadFile("y-writings/source-repo", "0123456789abcdef0123456789abcdef01234567", driftline.ContractPath)
			if err == nil {
				t.Fatal("expected read error")
			}
			if got := errors.Is(err, os.ErrNotExist); got != tt.wantNotFound {
				t.Fatalf("errors.Is(os.ErrNotExist) = %v, want %v: %v", got, tt.wantNotFound, err)
			}
		})
	}
}

func TestClientMapsRateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClientFromEnv()
	client.apiBase = server.URL
	_, err := client.ResolveRef("y-writings/source-repo", "main")
	if err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("expected token guidance, got %v", err)
	}
	if strings.Contains(err.Error(), "API rate limit exceeded") {
		t.Fatalf("response body must not be included in error: %v", err)
	}
}

func TestClientHasTimeout(t *testing.T) {
	client := NewClientFromEnv()
	if client.httpClient.Timeout != 30*time.Second {
		t.Fatalf("expected 30s timeout, got %s", client.httpClient.Timeout)
	}
}
