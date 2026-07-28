package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/ai-sh/internal/config"
	"github.com/user/ai-sh/internal/llm"
	"github.com/user/ai-sh/internal/runner"
)

var (
	setupFlag    bool
	statusFlag   bool
	providerFlag string
	modelFlag    string
)

var rootCmd = &cobra.Command{
	Use:   "ai <prompt>",
	Short: "Convert natural language to shell commands",
	Long: "Convert natural language to shell commands using a local model or a cloud provider.\n\n" +
		"Backend selection lives in ~/.ai-sh/config.json (run 'ai --setup') and can be\n" +
		"overridden per call with --provider/--model or the AI_SH_* environment variables.",
	Args:          cobra.ArbitraryArgs,
	SilenceUsage:  true,
	SilenceErrors: false,
	RunE: func(cmd *cobra.Command, args []string) error {
		if setupFlag {
			return runSetup()
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
}

// runStatus reports the resolved backend without running inference.
func runStatus(cfg config.Config) error {
	path, err := config.Path()
	if err != nil {
		return err
	}
	fmt.Printf("config:   %s\n", path)

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
