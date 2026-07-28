package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/ai-sh/internal/config"
	"github.com/user/ai-sh/internal/llm"
	"github.com/user/ai-sh/internal/memory"
	"github.com/user/ai-sh/internal/runner"
)

var (
	setupFlag    bool
	statusFlag   bool
	providerFlag string
	modelFlag    string
	rememberFlag string
	memoryFlag   bool
	forgetFlag   string
)

var rootCmd = &cobra.Command{
	Use:   "ai <prompt>",
	Short: "Convert natural language to shell commands",
	Long: "Convert natural language to shell commands using a local model or a cloud provider.\n\n" +
		"Backend selection lives in ~/.ai-sh/config.json (run 'ai --setup') and can be\n" +
		"overridden per call with --provider/--model or the AI_SH_* environment variables.\n\n" +
		"Notes in ~/.ai-sh/memory.md are sent with every prompt; manage them with\n" +
		"--remember/--memory/--forget, or edit the file directly.",
	Args:          cobra.ArbitraryArgs,
	SilenceUsage:  true,
	SilenceErrors: false,
	RunE: func(cmd *cobra.Command, args []string) error {
		if setupFlag {
			return runSetup()
		}
		if rememberFlag != "" {
			return runRemember(rememberFlag)
		}
		if forgetFlag != "" {
			return runForget(forgetFlag)
		}
		if memoryFlag {
			return runMemory()
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if providerFlag != "" {
			cfg.Provider = providerFlag
			// A different provider invalidates the stored model, unless the
			// caller named one explicitly.
			if modelFlag == "" {
				cfg.Model = ""
				cfg.BaseURL = ""
			}
		}
		if modelFlag != "" {
			cfg.Model = modelFlag
		}

		if statusFlag {
			return runStatus(cfg)
		}

		if len(args) == 0 {
			return cmd.Help()
		}
		prompt := strings.Join(args, " ")

		provider, err := llm.New(cfg)
		if err != nil {
			return err
		}

		command, err := provider.Generate(prompt)
		if err != nil {
			return err
		}
		if command == "" {
			return fmt.Errorf("model returned empty output. Try rephrasing")
		}

		return runner.ConfirmAndRun(command, prompt, provider.Generate)
	},
}

func init() {
	rootCmd.Flags().BoolVar(&setupFlag, "setup", false, "choose a local or cloud model and save it to ~/.ai-sh/config.json")
	rootCmd.Flags().BoolVar(&statusFlag, "status", false, "show which backend is configured")
	rootCmd.Flags().StringVar(&providerFlag, "provider", "", "backend for this call: local, "+config.KnownCloudProviders())
	rootCmd.Flags().StringVar(&modelFlag, "model", "", "cloud model id for this call")
	rootCmd.Flags().StringVar(&rememberFlag, "remember", "", "store a note sent with every prompt (e.g. \"prefer rg over grep\")")
	rootCmd.Flags().BoolVar(&memoryFlag, "memory", false, "list remembered notes")
	rootCmd.Flags().StringVar(&forgetFlag, "forget", "", "drop note <n> from memory, or 'all'")
}

// runRemember stores a note and echoes it back so the user sees the normalized
// form that will reach the model.
func runRemember(note string) error {
	added, err := memory.Add(note)
	if err != nil {
		return err
	}
	if added {
		fmt.Println("Remembered.")
	} else {
		fmt.Println("Already remembered.")
	}
	return runMemory()
}

// runForget drops one note by its listed position, or the whole file.
func runForget(arg string) error {
	if strings.EqualFold(arg, "all") {
		if err := memory.Clear(); err != nil {
			return err
		}
		fmt.Println("Memory cleared.")
		return nil
	}

	n, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil || n < 1 {
		return fmt.Errorf("--forget takes a note number from 'ai --memory', or 'all'")
	}

	removed, err := memory.Forget(n)
	if err != nil {
		return err
	}
	fmt.Printf("Forgot: %s\n", removed)
	return nil
}

// runMemory lists the notes with the numbers --forget accepts.
func runMemory() error {
	path, err := memory.Path()
	if err != nil {
		return err
	}
	entries, err := memory.Load()
	if err != nil {
		return err
	}

	fmt.Printf("memory:   %s\n", path)
	if len(entries) == 0 {
		fmt.Println("(empty)   add one with: ai --remember \"...\"")
		return nil
	}
	for i, entry := range entries {
		fmt.Printf("%3d. %s\n", i+1, entry)
	}
	return nil
}

// runStatus reports the resolved backend without running inference.
func runStatus(cfg config.Config) error {
	path, err := config.Path()
	if err != nil {
		return err
	}
	fmt.Printf("config:   %s\n", path)

	if memPath, err := memory.Path(); err == nil {
		notes, _ := memory.Load()
		fmt.Printf("memory:   %s (%d notes)\n", memPath, len(notes))
	}

	provider, err := llm.New(cfg)
	if err != nil {
		fmt.Printf("provider: %s (not ready)\n", cfg.Provider)
		return err
	}
	fmt.Printf("provider: %s\n", provider.Describe())
	return nil
}

func Execute() error {
	return rootCmd.Execute()
}
