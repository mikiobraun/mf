package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mikiobraun/mf/registry"
)

// BaseURL must pass the configured URL through untouched apart from a
// trailing slash, adding and removing no version segment.
func TestBaseURLIsVerbatim(t *testing.T) {
	cases := map[string]string{
		"https://api.openai.com/v1":     "https://api.openai.com/v1",
		"https://api.openai.com/v1/":    "https://api.openai.com/v1",
		"https://api.example.com":       "https://api.example.com",
		"https://api.example.com/":      "https://api.example.com",
		"https://gw.example.com/openai": "https://gw.example.com/openai",
		"http://localhost:11434/v1":     "http://localhost:11434/v1",
	}
	for in, want := range cases {
		if got := BaseURL(in); got != want {
			t.Errorf("BaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// A gateway may mount the API somewhere other than /v1.
func TestNonV1BasePathIsHonoured(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"data":[{"id":"m"}]}`))
	}))
	defer srv.Close()
	c, err := New(registry.Endpoint{Name: "gw", URL: srv.URL + "/openai/v2"}, "k")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/openai/v2/models" {
		t.Errorf("requested %q, want /openai/v2/models", gotPath)
	}
}

// The usual cause of a 404 is a URL missing its version prefix, so the error
// names the configured base URL.
func TestNotFoundMentionsTheBaseURL(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, err := c.ListModels(context.Background())
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("err = %v, want *StatusError", err)
	}
	if !strings.Contains(statusErr.Detail, "version prefix") {
		t.Errorf("404 detail lacks the base-URL hint: %q", statusErr.Detail)
	}
}

func TestAnthropicDialectIsNotImplemented(t *testing.T) {
	_, err := New(registry.Endpoint{Name: "a", URL: "https://x", API: registry.APIAnthropic}, "k")
	if err == nil {
		t.Fatal("expected an error for the anthropic dialect")
	}
}

func TestUnknownDialect(t *testing.T) {
	if _, err := New(registry.Endpoint{Name: "a", URL: "https://x", API: "gemini"}, "k"); err == nil {
		t.Fatal("expected an error for an unknown dialect")
	}
}

func newTestClient(t *testing.T, h http.HandlerFunc) Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(registry.Endpoint{Name: "test", URL: srv.URL}, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestListModelsSortsAndSkipsBlanks(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path = %q, want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		w.Write([]byte(`{"data":[{"id":"zeta"},{"id":"alpha"},{"id":""}]}`))
	})
	ids, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "alpha" || ids[1] != "zeta" {
		t.Errorf("ids = %v, want [alpha zeta]", ids)
	}
}

func TestListModelsAuthError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"bad key"}}`))
	})
	_, err := c.ListModels(context.Background())
	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("err = %v, want *AuthError", err)
	}
	if authErr.Detail != "bad key" {
		t.Errorf("Detail = %q, want 'bad key'", authErr.Detail)
	}
}

func TestListModelsStatusError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`upstream exploded`))
	})
	_, err := c.ListModels(context.Background())
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("err = %v, want *StatusError", err)
	}
	if statusErr.Status != 500 {
		t.Errorf("Status = %d, want 500", statusErr.Status)
	}
}

func TestChat(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"  hi there\n"}}]}`))
	})
	got, err := c.Chat(context.Background(), "m", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hi there" {
		t.Errorf("Chat = %q, want %q", got, "hi there")
	}
}

func TestChatNoChoices(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[]}`))
	})
	if _, err := c.Chat(context.Background(), "m", "hello"); err == nil {
		t.Fatal("expected an error when the endpoint returns no choices")
	}
}

func TestNoKeySendsNoAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["Authorization"]; ok {
			t.Error("Authorization header sent despite empty key")
		}
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()
	c, err := New(registry.Endpoint{Name: "local", URL: srv.URL}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListModels(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// A base URL pointing at a web root typically serves HTML with a 200 status.
func TestHTMLBodyWith200IsReportedAsABaseURLProblem(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<!doctype html><html><body>welcome</body></html>"))
	})
	_, err := c.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected an error for an HTML body")
	}
	if !strings.Contains(err.Error(), "version prefix") {
		t.Errorf("error lacks the base-URL hint: %v", err)
	}
	if !strings.Contains(err.Error(), "did not return JSON") {
		t.Errorf("error should say the body was not JSON: %v", err)
	}
}

func TestChatHTMLBodyAlsoHinted(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>nope</html>"))
	})
	if _, err := c.Chat(context.Background(), "m", "hi"); err == nil ||
		!strings.Contains(err.Error(), "version prefix") {
		t.Errorf("err = %v, want the base-URL hint", err)
	}
}
