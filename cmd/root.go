package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/ai-sh/internal/config"
	"github.com/user/ai-sh/internal/history"
	"github.com/user/ai-sh/internal/llm"
	"github.com/user/ai-sh/internal/runner"
)

var (
	setupFlag    bool
	statusFlag   bool
	newFlag      bool
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

		if newFlag {
			if err := history.Clear(); err != nil {
				return err
			}
			if len(args) == 0 {
				fmt.Println("Session history cleared.")
				return nil
			}
		}

		if len(args) == 0 {
			return cmd.Help()
		}
		prompt := strings.Join(args, " ")

		provider, err := llm.New(cfg)
		if err != nil {
			return err
		}

		useHistory := cfg.HistoryEnabled()
		messages, past := buildMessages(prompt, useHistory, cfg.HistoryTurns)

		if past > 0 {
			// History is implicit, so say when it is in play and how to escape.
			fmt.Fprintf(os.Stderr, "\033[2m↳ %s from this terminal (ai --new to reset)\033[0m\n", pluralTurns(past))
		}

		command, err := provider.Generate(messages)
		if err != nil {
			return err
		}
		if command == "" {
			return fmt.Errorf("model returned empty output. Try rephrasing")
		}

		outcome, runErr := runner.ConfirmAndRun(command, messages, provider.Generate)
		if useHistory {
			record(prompt, outcome)
		}
		return runErr
	},
}

// buildMessages assembles the conversation, returning it with the number of
// past turns folded in.
//
// Past commands become assistant turns verbatim, with no note of their exit
// status: keeping assistant content to a bare command is what stops the model
// learning to emit prose or comments. The exit code is still recorded, for the
// failure-explanation work that will need it.
func buildMessages(prompt string, useHistory bool, turns int) ([]llm.Message, int) {
	var past []history.Record
	if useHistory {
		past = history.Load(turns)
	}

	messages := make([]llm.Message, 0, len(past)*2+1)
	for _, rec := range past {
		messages = append(messages, llm.User(rec.Prompt), llm.Assistant(rec.Command))
	}
	return append(messages, llm.User(prompt)), len(past)
}

// record appends the turn, ignoring failures: a transcript that cannot be
// written is not worth interrupting the user over.
func record(prompt string, outcome runner.Outcome) {
	if outcome.Command == "" {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	_ = history.Append(history.Record{
		Cwd:     cwd,
		Prompt:  prompt,
		Command: outcome.Command,
		Ran:     outcome.Ran,
		Exit:    outcome.ExitCode,
	})
}

func pluralTurns(n int) string {
	if n == 1 {
		return "1 earlier turn"
	}
	return fmt.Sprintf("%d earlier turns", n)
}

func init() {
	rootCmd.Flags().BoolVar(&setupFlag, "setup", false, "choose a local or cloud model and save it to ~/.ai-sh/config.json")
	rootCmd.Flags().BoolVar(&statusFlag, "status", false, "show which backend is configured")
	rootCmd.Flags().BoolVar(&newFlag, "new", false, "forget this terminal's session history before running")
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
	fmt.Printf("history:  %s\n", historyStatus(cfg))

	provider, err := llm.New(cfg)
	if err != nil {
		fmt.Printf("provider: %s (not ready)\n", cfg.Provider)
		return err
	}
	fmt.Printf("provider: %s\n", provider.Describe())
	return nil
}

// historyStatus explains the effective setting, including the case where it is
// switched on in the config but clamped off by the backend.
func historyStatus(cfg config.Config) string {
	switch {
	case cfg.HistoryEnabled():
		return fmt.Sprintf("on (%s in this terminal)", pluralTurns(len(history.Load(cfg.HistoryTurns))))
	case cfg.History:
		return "off (not supported by the local backend)"
	default:
		return "off"
	}
}

func Execute() error {
	return rootCmd.Execute()
}
