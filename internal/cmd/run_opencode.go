package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/mikiobraun/mf/registry"
	"github.com/spf13/cobra"
)

func newRunOpencodeCmd() *cobra.Command {
	var small string
	help := argHelp{
		min: 1, max: -1,
		args: [][2]string{
			{"<model>", "main model; " + addrArg},
			{"[<more>...]", "further models to offer in opencode's model list"},
			{"[-- <args>...]", "arguments passed through to opencode"},
		},
		example: "run opencode fast --small cheap -- run \"say hello\"",
	}
	c := &cobra.Command{
		Use:   "opencode <model> [<more>...] [-- <args>...]",
		Short: "Run opencode with models from the registry",
		Long: "Run opencode with the named models, replacing the mf process.\n\n" +
			"The configuration is passed in OPENCODE_CONFIG_CONTENT, which opencode\n" +
			"merges over its own config files, so nothing is written to disk. Each\n" +
			"endpoint becomes an opencode provider, and its key is passed in\n" +
			"MF_KEY_<ENDPOINT> and referenced as {env:MF_KEY_<ENDPOINT>}.\n\n" +
			"The first model becomes opencode's model and --small its small_model.",
		Args: func(c *cobra.Command, args []string) error {
			if err := help.validator()(c, args); err != nil {
				return err
			}
			if c.ArgsLenAtDash() == 0 {
				return fmt.Errorf("run opencode needs a model before --\n%s", help.details(c))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			addrs, rest := splitAtDash(cmd, args)
			s, err := openStore()
			if err != nil {
				return err
			}
			var refs []*registry.Ref
			for _, addr := range addrs {
				ref, err := registry.Resolve(s, addr)
				if err != nil {
					return err
				}
				warnAmbiguousTag(addr, ref.TagMatches)
				refs = append(refs, ref)
			}
			var smallRef *registry.Ref
			if small != "" {
				if smallRef, err = registry.Resolve(s, small); err != nil {
					return err
				}
				warnAmbiguousTag(small, smallRef.TagMatches)
			}

			cfg, keys, err := opencodeConfig(s, refs, smallRef)
			if err != nil {
				return err
			}
			content, err := json.Marshal(cfg)
			if err != nil {
				return err
			}

			path, err := exec.LookPath("opencode")
			if err != nil {
				return err
			}
			env := withEnv(os.Environ(), "OPENCODE_CONFIG_CONTENT", string(content))
			for name, key := range keys {
				env = withEnv(env, name, key)
			}
			return execve(path, append([]string{"opencode"}, rest...), env)
		},
	}
	c.Flags().StringVar(&small, "small", "", "model for opencode's small_model (titles, summaries)")
	return c
}

// opencodeConfig builds the opencode configuration for the given models: one
// provider per endpoint, holding only the named models, with model and
// small_model set. The returned keys map environment variable names to the
// keys the config references with {env:...}.
func opencodeConfig(s *registry.Store, refs []*registry.Ref, small *registry.Ref) (map[string]any, map[string]string, error) {
	providers := map[string]map[string]any{}
	keys := map[string]string{}
	// keyOwner records which model fixed an endpoint's key, for the error
	// when a model-scoped secret would give the same provider a second key.
	keyOwner := map[string]string{}

	all := refs
	if small != nil {
		all = append(append([]*registry.Ref{}, refs...), small)
	}
	for _, ref := range all {
		ep := ref.Endpoint
		if d := ep.Dialect(); d != registry.APIOpenAI {
			return nil, nil, fmt.Errorf("endpoint %s uses api %q; opencode is only configured for openai endpoints so far", ep.Name, d)
		}
		p, ok := providers[ep.Name]
		if !ok {
			options := map[string]any{"baseURL": ep.URL}
			p = map[string]any{
				"npm":     "@ai-sdk/openai-compatible",
				"name":    ep.Name,
				"options": options,
				"models":  map[string]any{},
			}
			providers[ep.Name] = p
		}

		if key, hasKey := ref.Key(s); hasKey {
			envName := registry.KeyEnvVar(ep.Name)
			if prev, seen := keys[envName]; seen && prev != key {
				return nil, nil, fmt.Errorf("%s and %s use different keys on endpoint %s; opencode has one key per provider",
					keyOwner[envName], ref.Name, ep.Name)
			}
			keys[envName] = key
			keyOwner[envName] = ref.Name
			p["options"].(map[string]any)["apiKey"] = "{env:" + envName + "}"
		}

		models := p["models"].(map[string]any)
		if _, dup := models[ref.ModelID]; !dup {
			model := map[string]any{"name": ref.Name}
			// opencode rejects a limit that lacks either value.
			switch {
			case ref.Context > 0 && ref.Output > 0:
				model["limit"] = map[string]any{"context": ref.Context, "output": ref.Output}
			case ref.Context > 0:
				return nil, nil, fmt.Errorf("%s has a context limit but no output limit, and opencode needs both; set it with `mf set %s output <tokens>`", ref.Name, ref.Name)
			case ref.Output > 0:
				return nil, nil, fmt.Errorf("%s has an output limit but no context limit, and opencode needs both; set it with `mf set %s context <tokens>`", ref.Name, ref.Name)
			}
			models[ref.ModelID] = model
		}
	}

	cfg := map[string]any{
		"$schema":  "https://opencode.ai/config.json",
		"provider": providers,
		"model":    refs[0].Endpoint.Name + "/" + refs[0].ModelID,
	}
	if small != nil {
		cfg["small_model"] = small.Endpoint.Name + "/" + small.ModelID
	}
	return cfg, keys, nil
}
