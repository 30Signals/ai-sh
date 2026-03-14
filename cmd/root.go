package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/ai-sh/internal/llm"
	"github.com/user/ai-sh/internal/runner"
)

var rootCmd = &cobra.Command{
	Use:          "ai <prompt>",
	Short:        "Convert natural language to bash commands",
	Args:         cobra.MinimumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		prompt := strings.Join(args, " ")

		llamaPath, err := llm.FindLlamaCLI()
		if err != nil {
			return fmt.Errorf("llama-cli not found in ~/.ai-sh/bin/. Run: curl -fsSL https://raw.githubusercontent.com/30Signals/ai-sh/main/install.sh | bash")
		}

		modelPath, err := llm.FindModel()
		if err != nil {
			return fmt.Errorf("no .gguf model found in ~/.ai-sh/models/. Download a model and place it there")
		}

		infer := func(p string) (string, error) {
			return llm.RunInference(llamaPath, modelPath, p)
		}

		command, err := infer(prompt)
		if err != nil {
			return err
		}
		if command == "" {
			return fmt.Errorf("model returned empty output. Try rephrasing")
		}

		return runner.ConfirmAndRun(command, prompt, infer)
	},
}

func Execute() error {
	return rootCmd.Execute()
}
