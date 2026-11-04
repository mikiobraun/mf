// Addressing: turning a user-supplied model address into a concrete endpoint,
// model ID and API key.

package registry

import (
	"fmt"
	"os"
	"strings"
)

// Ref is a fully resolved model address.
type Ref struct {
	// Name is the entry alias, or "endpoint/model" when addressed directly.
	Name string
	// ModelID is the provider's model identifier.
	ModelID string
	// Endpoint is the endpoint the model lives on.
	Endpoint Endpoint
	// Tags are the entry's tags; nil for direct endpoint/model references.
	Tags []string
	// Context and Output are the entry's token limits; zero when unset or
	// for direct endpoint/model references.
	Context, Output int
	// Entry reports whether a [[model]] entry backs this reference. Only
	// entries can carry model-scoped secrets.
	Entry bool
	// TagMatches holds the other candidates when addressed by @tag.
	TagMatches []string
}

// Resolve accepts three address forms, in this precedence:
//
//	fast                    an alias matching a [[model]] name
//	anthropic/claude-opus   an endpoint name, a slash, then a model ID
//	@coding                 the first entry carrying that tag
//
// The endpoint/model form splits on the first slash only, so model IDs
// containing slashes ("digitalocean/deepseek-v4-pro") resolve correctly.
func Resolve(s *Store, addr string) (*Ref, error) {
	if addr == "" {
		return nil, fmt.Errorf("empty model address")
	}

	if tag, ok := strings.CutPrefix(addr, "@"); ok {
		return resolveTag(s, tag)
	}

	// An alias wins over an endpoint/model reading of the same string.
	if m, ok := s.Model(addr); ok {
		return fromEntry(s, m)
	}

	if epName, modelID, ok := strings.Cut(addr, "/"); ok {
		ep, found := s.Endpoint(epName)
		if !found {
			return nil, fmt.Errorf("no model or endpoint named %q (known endpoints: %s)", epName, s.EndpointNames())
		}
		if modelID == "" {
			return nil, fmt.Errorf("no model ID after %q", epName+"/")
		}
		return &Ref{
			Name:     addr,
			ModelID:  modelID,
			Endpoint: ep,
		}, nil
	}

	return nil, fmt.Errorf("no model named %q", addr)
}

func resolveTag(s *Store, tag string) (*Ref, error) {
	matches := s.ModelsWithTag(tag)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no model tagged %q", tag)
	}
	ref, err := fromEntry(s, matches[0])
	if err != nil {
		return nil, err
	}
	for _, m := range matches[1:] {
		ref.TagMatches = append(ref.TagMatches, m.Name)
	}
	return ref, nil
}

func fromEntry(s *Store, m Model) (*Ref, error) {
	ep, ok := s.Endpoint(m.Endpoint)
	if !ok {
		return nil, fmt.Errorf("model %q references unknown endpoint %q", m.Name, m.Endpoint)
	}
	return &Ref{
		Name:     m.Name,
		ModelID:  m.Model,
		Endpoint: ep,
		Tags:     m.Tags,
		Context:  m.Context,
		Output:   m.Output,
		Entry:    true,
	}, nil
}

// Key resolves the API key for the reference. Precedence, highest first:
//
//	MF_KEY_<ENDPOINT>   environment override, for CI and containers
//	model-scoped secret in models.secrets
//	endpoint-scoped secret in models.secrets
//
// It reports ok=false when no key is configured; callers decide whether that
// is fatal (a local ollama needs no key).
func (r *Ref) Key(s *Store) (key string, ok bool) {
	if v := os.Getenv(KeyEnvVar(r.Endpoint.Name)); v != "" {
		return v, true
	}
	if r.Entry {
		for _, sec := range s.Secrets.Secrets {
			if sec.Endpoint == r.Endpoint.Name && sec.Model == r.Name {
				return sec.Key, true
			}
		}
	}
	for _, sec := range s.Secrets.Secrets {
		if sec.Endpoint == r.Endpoint.Name && sec.Model == "" {
			return sec.Key, true
		}
	}
	return "", false
}

// EndpointKey resolves the key for a bare endpoint, used by commands that
// address an endpoint rather than a model (models, sync).
func EndpointKey(s *Store, endpoint string) (key string, ok bool) {
	if v := os.Getenv(KeyEnvVar(endpoint)); v != "" {
		return v, true
	}
	for _, sec := range s.Secrets.Secrets {
		if sec.Endpoint == endpoint && sec.Model == "" {
			return sec.Key, true
		}
	}
	return "", false
}

// KeyEnvVar is the environment variable that overrides an endpoint's key.
// Characters that cannot appear in a shell variable name become underscores.
func KeyEnvVar(endpoint string) string {
	var b strings.Builder
	b.WriteString("MF_KEY_")
	for _, r := range strings.ToUpper(endpoint) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
