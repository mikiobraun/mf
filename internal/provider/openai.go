package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// openAIClient speaks the OpenAI-compatible wire format: GET {base}/models,
// POST {base}/chat/completions, bearer auth. Ollama, vLLM, LM Studio, OpenAI and
// most gateways all expose this shape.
type openAIClient struct {
	base string
	key  string
	http *http.Client
}

func (c *openAIClient) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Cap the read so a misconfigured URL returning a huge page cannot
	// exhaust memory.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return nil, &AuthError{Status: resp.StatusCode, Detail: errDetail(data)}
	case resp.StatusCode == http.StatusNotFound:
		// Most often this is a base URL missing its version prefix.
		return nil, &StatusError{Status: resp.StatusCode, Detail: withBaseURLHint(c.base, path, errDetail(data))}
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return nil, &StatusError{Status: resp.StatusCode, Detail: errDetail(data)}
	}
	return data, nil
}

// errDetail pulls a human-readable message out of an error body, falling back
// to a truncated raw body.
func errDetail(data []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &e) == nil {
		if e.Error.Message != "" {
			return e.Error.Message
		}
		if e.Message != "" {
			return e.Message
		}
	}
	s := strings.TrimSpace(string(data))
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// baseURLAdvice is the standing explanation for a URL that does not point at
// an API root.
const baseURLAdvice = "the endpoint URL must be the API base, including any " +
	"version prefix the provider uses (usually .../v1)"

// withBaseURLHint names the URL mf actually requested.
func withBaseURLHint(base, path, detail string) string {
	hint := fmt.Sprintf("no %s under %s; %s", path, base, baseURLAdvice)
	if detail == "" {
		return hint
	}
	return detail + " — " + hint
}

// decode parses a response body, reporting a non-JSON body as a base URL
// problem: a URL pointing at a web root commonly serves HTML with a 200.
func (c *openAIClient) decode(path string, data []byte, dst any) error {
	if err := json.Unmarshal(data, dst); err != nil {
		snippet := strings.TrimSpace(string(data))
		if len(snippet) > 60 {
			snippet = snippet[:60] + "..."
		}
		return fmt.Errorf("%s did not return JSON (body starts %q); %s",
			c.base+path, snippet, baseURLAdvice)
	}
	return nil
}

func (c *openAIClient) ListModels(ctx context.Context) ([]string, error) {
	data, err := c.do(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.decode("/models", data, &out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (c *openAIClient) Chat(ctx context.Context, modelID, message string) (string, error) {
	req := map[string]any{
		"model": modelID,
		"messages": []map[string]string{
			{"role": "user", "content": message},
		},
	}
	data, err := c.do(ctx, http.MethodPost, "/chat/completions", req)
	if err != nil {
		return "", err
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := c.decode("/chat/completions", data, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("endpoint returned no choices")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
