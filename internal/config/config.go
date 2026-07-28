// Package config stores which LLM backend ai-sh talks to.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is persisted as ~/.ai-sh/config.json.
type Config struct {
	// Provider is "local" or the name of a cloud preset (see Presets).
	Provider string `json:"provider"`
	// Model is the cloud model id. Ignored by the local provider, which
	// always uses the .gguf file in ~/.ai-sh/models/.
	Model string `json:"model,omitempty"`
	// APIKey authenticates cloud requests. May be left empty and supplied
	// through the environment instead.
	APIKey string `json:"api_key,omitempty"`
	// BaseURL overrides the preset endpoint. Required for provider "custom".
	BaseURL string `json:"base_url,omitempty"`
	// History opts into carrying recent turns from the same terminal into the
	// next prompt. Off by default, and set once (here or via 'ai --setup')
	// rather than per call, which is what keeps a follow-up invocation bare.
	History bool `json:"history,omitempty"`
	// HistoryTurns overrides how many past turns are carried. Zero means the
	// package default.
	HistoryTurns int `json:"history_turns,omitempty"`
}

// HistoryEnabled reports whether to carry past turns for this backend.
//
// The local backend is excluded even when History is set, for two reasons: a
// 1-3B model at temperature 0.1 tends to answer worse with several turns of
// chatter ahead of the instruction, and every call spawns a fresh llama-cli, so
// history would mean re-prefilling the whole transcript on CPU each time. This
// is a clamp rather than a validation error on purpose -- switching to the local
// backend must not turn a saved config into a startup failure.
func (c Config) HistoryEnabled() bool {
	return c.History && c.Provider != "local"
}

// Preset describes a cloud endpoint that speaks the OpenAI chat-completions
// protocol, which every provider below does.
type Preset struct {
	Label        string
	BaseURL      string
	DefaultModel string
	// EnvKey is the provider's conventional API key variable, checked when
	// neither the config file nor AI_SH_API_KEY supplies one.
	EnvKey  string
	KeysURL string
}

// Presets are the known cloud providers, keyed by config Provider value.
var Presets = map[string]Preset{
	"mistral": {
		Label:        "Mistral",
		BaseURL:      "https://api.mistral.ai/v1",
		DefaultModel: "devstral-medium-latest",
		EnvKey:       "MISTRAL_API_KEY",
		KeysURL:      "https://console.mistral.ai/api-keys",
	},
	"groq": {
		Label:        "Groq",
		BaseURL:      "https://api.groq.com/openai/v1",
		DefaultModel: "llama-3.3-70b-versatile",
		EnvKey:       "GROQ_API_KEY",
		KeysURL:      "https://console.groq.com/keys",
	},
	"openrouter": {
		Label:        "OpenRouter",
		BaseURL:      "https://openrouter.ai/api/v1",
		DefaultModel: "mistralai/devstral-small-2505:free",
		EnvKey:       "OPENROUTER_API_KEY",
		KeysURL:      "https://openrouter.ai/keys",
	},
	"cerebras": {
		Label:        "Cerebras",
		BaseURL:      "https://api.cerebras.ai/v1",
		DefaultModel: "llama-3.3-70b",
		EnvKey:       "CEREBRAS_API_KEY",
		KeysURL:      "https://cloud.cerebras.ai/platform",
	},
	"custom": {
		Label:   "Custom OpenAI-compatible endpoint",
		EnvKey:  "AI_SH_API_KEY",
		KeysURL: "",
	},
}

// Path returns the config file location.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ai-sh", "config.json"), nil
}

// Load reads the config file, then applies environment overrides. A missing
// file is not an error: it yields the local provider, preserving the
// behaviour of installs that predate cloud support.
func Load() (Config, error) {
	cfg := Config{Provider: "local"}

	path, err := Path()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parsing %s: %w", path, err)
		}
	case !os.IsNotExist(err):
		return cfg, fmt.Errorf("reading %s: %w", path, err)
	}

	if v := os.Getenv("AI_SH_PROVIDER"); v != "" {
		cfg.Provider = v
	}
	if cfg.Provider == "" {
		cfg.Provider = "local"
	}
	if v := os.Getenv("AI_SH_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("AI_SH_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("AI_SH_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("AI_SH_HISTORY"); v != "" {
		if on, ok := parseBool(v); ok {
			cfg.History = on
		}
	}
	if v := os.Getenv("AI_SH_HISTORY_TURNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.HistoryTurns = n
		}
	}

	return cfg, nil
}

// parseBool accepts the spellings people actually type in a shell.
func parseBool(v string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	}
	return false, false
}

// Save writes the config file with owner-only permissions, since it may hold
// an API key.
func (c Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// Resolve fills in endpoint, model, and key defaults for cloud providers from
// the matching preset and environment.
func (c Config) Resolve() (Config, error) {
	if c.Provider == "local" {
		return c, nil
	}

	preset, ok := Presets[c.Provider]
	if !ok {
		return c, fmt.Errorf("unknown provider %q (known: local, %s)", c.Provider, KnownCloudProviders())
	}

	if c.BaseURL == "" {
		c.BaseURL = preset.BaseURL
	}
	if c.BaseURL == "" {
		return c, fmt.Errorf("provider %q needs a base URL: set base_url in the config file or AI_SH_BASE_URL", c.Provider)
	}
	if c.Model == "" {
		c.Model = preset.DefaultModel
	}
	if c.Model == "" {
		return c, fmt.Errorf("provider %q needs a model: set model in the config file or AI_SH_MODEL", c.Provider)
	}
	if c.APIKey == "" && preset.EnvKey != "" {
		c.APIKey = os.Getenv(preset.EnvKey)
	}
	if c.APIKey == "" {
		hint := "run: ai --setup"
		if preset.EnvKey != "" {
			hint = fmt.Sprintf("set %s or run: ai --setup", preset.EnvKey)
		}
		return c, fmt.Errorf("no API key for %s (%s)", preset.Label, hint)
	}

	return c, nil
}

// KnownCloudProviders lists cloud preset names in a stable order for messages.
func KnownCloudProviders() string {
	return "mistral, groq, openrouter, cerebras, custom"
}
