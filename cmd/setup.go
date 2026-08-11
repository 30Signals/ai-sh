package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/user/ai-sh/internal/config"
	"github.com/user/ai-sh/internal/llm"
)

// cloudOrder fixes the menu order; config.Presets is a map.
var cloudOrder = []string{"mistral", "groq", "openrouter", "cerebras", "custom"}

// runSetup asks which backend to use and writes ~/.ai-sh/config.json.
func runSetup() error {
	in := bufio.NewReader(os.Stdin)

	current, err := config.Load()
	if err != nil {
		return err
	}

	fmt.Println("Choose a backend:")
	fmt.Println("  1) local — llama.cpp model in ~/.ai-sh/models/ (offline, no API key)")
	for i, name := range cloudOrder {
		preset := config.Presets[name]
		if preset.DefaultModel != "" {
			fmt.Printf("  %d) %s — %s (%s)\n", i+2, name, preset.Label, preset.DefaultModel)
		} else {
			fmt.Printf("  %d) %s — %s\n", i+2, name, preset.Label)
		}
	}
	fmt.Println()

	choice, err := prompt(in, fmt.Sprintf("Enter choice [1-%d]", len(cloudOrder)+1), currentChoice(current.Provider))
	if err != nil {
		return err
	}

	cfg := config.Config{Provider: "local"}
	if choice != "1" {
		name, ok := providerForChoice(choice)
		if !ok {
			return fmt.Errorf("invalid choice %q", choice)
		}
		preset := config.Presets[name]
		cfg.Provider = name

		if preset.BaseURL == "" {
			base, err := prompt(in, "Base URL (OpenAI-compatible, e.g. https://host/v1)", current.BaseURL)
			if err != nil {
				return err
			}
			if base == "" {
				return fmt.Errorf("a base URL is required for provider %q", name)
			}
			cfg.BaseURL = base
		}

		defaultModel := preset.DefaultModel
		if current.Provider == name && current.Model != "" {
			defaultModel = current.Model
		}
		model, err := prompt(in, "Model", defaultModel)
		if err != nil {
			return err
		}
		if model == "" {
			return fmt.Errorf("a model is required for provider %q", name)
		}
		cfg.Model = model

		if preset.KeysURL != "" {
			fmt.Printf("Get an API key: %s\n", preset.KeysURL)
		}
		keyPrompt := "API key"
		if preset.EnvKey != "" {
			keyPrompt = fmt.Sprintf("API key (blank to read %s from the environment)", preset.EnvKey)
		}
		key, err := prompt(in, keyPrompt, "")
		if err != nil {
			return err
		}
		cfg.APIKey = key

		// Only offered on the cloud path: the local backend clamps history off,
		// so asking here would save a setting that visibly does nothing.
		fmt.Println("\nSession history lets a follow-up refer back to earlier turns in the")
		fmt.Println("same terminal (\"now only the pdfs\"). Prompts get larger, and past")
		fmt.Println("instructions and commands are stored under ~/.ai-sh/sessions/.")
		enableHistory, err := confirm(in, "Enable session history?", current.History)
		if err != nil {
			return err
		}
		cfg.History = enableHistory
		cfg.HistoryTurns = current.HistoryTurns
	}

	if err := cfg.Save(); err != nil {
		return err
	}

	path, err := config.Path()
	if err != nil {
		return err
	}
	fmt.Printf("\nSaved %s\n", path)

	// Report whether the choice is actually usable now, so a missing key or
	// model surfaces here instead of on the next real prompt.
	provider, err := llm.New(cfg)
	if err != nil {
		fmt.Printf("Warning: %v\n", err)
		return nil
	}
	fmt.Printf("Using %s\n", provider.Describe())
	return nil
}

// currentChoice maps the configured provider back to its menu number so the
// existing setting is the default.
func currentChoice(provider string) string {
	for i, name := range cloudOrder {
		if name == provider {
			return fmt.Sprint(i + 2)
		}
	}
	return "1"
}

func providerForChoice(choice string) (string, bool) {
	for i, name := range cloudOrder {
		if choice == fmt.Sprint(i+2) {
			return name, true
		}
	}
	return "", false
}

// confirm asks a yes/no question, returning def when the input is empty.
func confirm(in *bufio.Reader, label string, def bool) (bool, error) {
	choices := "y/N"
	if def {
		choices = "Y/n"
	}
	fmt.Printf("%s [%s]: ", label, choices)

	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return def, err
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return def, nil
	}
}

// prompt reads one line, returning def when the input is empty.
func prompt(in *bufio.Reader, label, def string) (string, error) {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}

	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}

	value := strings.TrimSpace(line)
	if value == "" {
		return def, nil
	}
	return value, nil
}
