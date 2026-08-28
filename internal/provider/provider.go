// Package provider talks to model endpoints over HTTP.
package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mikiobraun/mf/internal/store"
)

// Client is the wire-format-specific surface mf needs from an endpoint.
type Client interface {
	// ListModels returns the model IDs the endpoint reports.
	ListModels(ctx context.Context) ([]string, error)
	// Chat sends a single user message and returns the reply text.
	Chat(ctx context.Context, modelID, message string) (string, error)
}

// New returns a client for the endpoint's dialect.
func New(ep store.Endpoint, key string) (Client, error) {
	switch ep.Dialect() {
	case store.APIOpenAI:
		return &openAIClient{base: BaseURL(ep.URL), key: key, http: httpClient()}, nil
	case store.APIAnthropic:
		return nil, fmt.Errorf("endpoint %q uses the %q API, which is not implemented yet; only %q endpoints are supported",
			ep.Name, store.APIAnthropic, store.APIOpenAI)
	default:
		return nil, fmt.Errorf("endpoint %q has unknown api %q (expected %q or %q)",
			ep.Name, ep.API, store.APIOpenAI, store.APIAnthropic)
	}
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second}
}

// BaseURL normalises an endpoint URL for joining with spec paths. It strips a
// trailing slash so joining cannot produce "//", and changes nothing else: the
// URL is the API base, so any version prefix the provider uses is part of it.
func BaseURL(url string) string {
	return strings.TrimRight(url, "/")
}

// AuthError means the endpoint was reached but rejected the credentials.
type AuthError struct {
	Status int
	Detail string
}

func (e *AuthError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("authentication failed (HTTP %d)", e.Status)
	}
	return fmt.Sprintf("authentication failed (HTTP %d): %s", e.Status, e.Detail)
}

// StatusError is any other non-2xx response.
type StatusError struct {
	Status int
	Detail string
}

func (e *StatusError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("HTTP %d", e.Status)
	}
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Detail)
}
