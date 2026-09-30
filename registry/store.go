// Package registry is the on-disk model registry and the lookups over it:
// models.toml (shareable definitions), models.secrets (API keys, never
// shared), and the addressing rules that turn a user-supplied model address
// into an endpoint, a model ID and a key.
//
// It is the package other tools import to read the registry, so that each one
// does not parse the files itself.
package registry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// API dialects an endpoint can speak.
const (
	APIOpenAI    = "openai"
	APIAnthropic = "anthropic"
)

// Endpoint is a provider: a base URL and the wire format it speaks.
type Endpoint struct {
	Name string `toml:"name"`
	// URL is the API base, including any version prefix the provider mounts
	// the API under, e.g. "https://api.openai.com/v1". mf appends only spec
	// paths ("/models", "/chat/completions") to it.
	URL string `toml:"url"`
	// API is the wire format. Empty means APIOpenAI.
	API string `toml:"api,omitempty"`
}

// Dialect returns the endpoint's API dialect, applying the default.
func (e Endpoint) Dialect() string {
	if e.API == "" {
		return APIOpenAI
	}
	return e.API
}

// Model is a named entry pointing at a model ID on an endpoint.
type Model struct {
	Name     string   `toml:"name"`
	Endpoint string   `toml:"endpoint"`
	Model    string   `toml:"model"`
	Tags     []string `toml:"tags,omitempty"`
	// Context and Output are token limits for the context window and a
	// single reply, as this endpoint serves the model. Zero means unset.
	Context int `toml:"context,omitempty"`
	Output  int `toml:"output,omitempty"`
}

// HasTag reports whether the entry carries tag.
func (m Model) HasTag(tag string) bool {
	for _, t := range m.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Config is the content of models.toml.
type Config struct {
	Endpoints []Endpoint `toml:"endpoint"`
	Models    []Model    `toml:"model"`
}

// Secret is one API key, scoped to an endpoint or to a single model on it.
type Secret struct {
	Endpoint string `toml:"endpoint"`
	Model    string `toml:"model,omitempty"`
	Key      string `toml:"key"`
}

// Secrets is the content of models.secrets.
type Secrets struct {
	Secrets []Secret `toml:"secret"`
}

// Store is a loaded registry rooted at a config directory.
type Store struct {
	dir     string
	Config  Config
	Secrets Secrets
}

// Dir reports the config directory backing this store.
func (s *Store) Dir() string { return s.dir }

// ConfigPath and SecretsPath are the two files the store owns.
func (s *Store) ConfigPath() string  { return filepath.Join(s.dir, "models.toml") }
func (s *Store) SecretsPath() string { return filepath.Join(s.dir, "models.secrets") }

// DefaultDir resolves the config directory: MF_CONFIG_DIR, else
// $XDG_CONFIG_HOME/mf, else ~/.config/mf.
func DefaultDir() (string, error) {
	if d := os.Getenv("MF_CONFIG_DIR"); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "mf"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, ".config", "mf"), nil
}

// Open loads the store from dir. A missing directory or missing files yield an
// empty store rather than an error, so first-run commands just work.
func Open(dir string) (*Store, error) {
	s := &Store{dir: dir}
	if err := readTOML(s.ConfigPath(), &s.Config); err != nil {
		return nil, err
	}
	if err := readTOML(s.SecretsPath(), &s.Secrets); err != nil {
		return nil, err
	}
	return s, nil
}

func readTOML(path string, dst any) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if err := toml.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	return nil
}

// Save writes models.toml. Comments and hand-written formatting are not
// preserved; the file is re-marshalled from the in-memory config.
func (s *Store) Save() error {
	return s.write(s.ConfigPath(), s.Config, 0o644)
}

// SaveSecrets writes models.secrets with owner-only permissions.
func (s *Store) SaveSecrets() error {
	return s.write(s.SecretsPath(), s.Secrets, 0o600)
}

func (s *Store) write(path string, v any, perm os.FileMode) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", s.dir, err)
	}
	b, err := toml.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	// Write to a temp file in the same directory, then rename, so an
	// interrupted write can never truncate a good config.
	tmp, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", s.dir, err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("setting permissions on %s: %w", path, err)
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}

// ---- lookups and mutations ----

// Endpoint returns the endpoint named name.
func (s *Store) Endpoint(name string) (Endpoint, bool) {
	for _, e := range s.Config.Endpoints {
		if e.Name == name {
			return e, true
		}
	}
	return Endpoint{}, false
}

// Model returns the model entry named name.
func (s *Store) Model(name string) (Model, bool) {
	for _, m := range s.Config.Models {
		if m.Name == name {
			return m, true
		}
	}
	return Model{}, false
}

// ModelsWithTag returns every entry carrying tag, in file order.
func (s *Store) ModelsWithTag(tag string) []Model {
	var out []Model
	for _, m := range s.Config.Models {
		if m.HasTag(tag) {
			out = append(out, m)
		}
	}
	return out
}

// PutEndpoint inserts or replaces an endpoint by name.
func (s *Store) PutEndpoint(e Endpoint) {
	for i, existing := range s.Config.Endpoints {
		if existing.Name == e.Name {
			s.Config.Endpoints[i] = e
			return
		}
	}
	s.Config.Endpoints = append(s.Config.Endpoints, e)
}

// RemoveEndpoint drops an endpoint along with every model referencing it and
// every secret scoped to it. It reports how many models were removed.
func (s *Store) RemoveEndpoint(name string) (models int, found bool) {
	kept := s.Config.Endpoints[:0]
	for _, e := range s.Config.Endpoints {
		if e.Name == name {
			found = true
			continue
		}
		kept = append(kept, e)
	}
	if !found {
		return 0, false
	}
	s.Config.Endpoints = kept

	keptModels := s.Config.Models[:0]
	for _, m := range s.Config.Models {
		if m.Endpoint == name {
			models++
			continue
		}
		keptModels = append(keptModels, m)
	}
	s.Config.Models = keptModels

	keptSecrets := s.Secrets.Secrets[:0]
	for _, sec := range s.Secrets.Secrets {
		if sec.Endpoint == name {
			continue
		}
		keptSecrets = append(keptSecrets, sec)
	}
	s.Secrets.Secrets = keptSecrets
	return models, true
}

// PutModel inserts or replaces a model entry by name.
func (s *Store) PutModel(m Model) {
	for i, existing := range s.Config.Models {
		if existing.Name == m.Name {
			s.Config.Models[i] = m
			return
		}
	}
	s.Config.Models = append(s.Config.Models, m)
}

// RemoveModel drops a model entry and any model-scoped secrets for it.
func (s *Store) RemoveModel(name string) bool {
	var found bool
	kept := s.Config.Models[:0]
	for _, m := range s.Config.Models {
		if m.Name == name {
			found = true
			continue
		}
		kept = append(kept, m)
	}
	if !found {
		return false
	}
	s.Config.Models = kept

	keptSecrets := s.Secrets.Secrets[:0]
	for _, sec := range s.Secrets.Secrets {
		if sec.Model == name {
			continue
		}
		keptSecrets = append(keptSecrets, sec)
	}
	s.Secrets.Secrets = keptSecrets
	return true
}

// PutSecret inserts or replaces a secret at the given scope. An empty model
// means the secret is endpoint-wide.
func (s *Store) PutSecret(endpoint, model, key string) {
	for i, sec := range s.Secrets.Secrets {
		if sec.Endpoint == endpoint && sec.Model == model {
			s.Secrets.Secrets[i].Key = key
			return
		}
	}
	s.Secrets.Secrets = append(s.Secrets.Secrets, Secret{
		Endpoint: endpoint, Model: model, Key: key,
	})
}

// EndpointNames lists configured endpoint names, for error messages.
func (s *Store) EndpointNames() string {
	names := make([]string, 0, len(s.Config.Endpoints))
	for _, e := range s.Config.Endpoints {
		names = append(names, e.Name)
	}
	if len(names) == 0 {
		return "(none configured)"
	}
	return strings.Join(names, ", ")
}
