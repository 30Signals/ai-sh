package llm

import (
	"fmt"
	"os"
	"strings"

	"github.com/user/ai-sh/internal/config"
	"github.com/user/ai-sh/internal/memory"
)

// Provider turns a natural language instruction into a shell command.
type Provider interface {
	// Generate returns a single command for the given instruction.
	Generate(prompt string) (string, error)
	// Describe names the backend for error messages and status output.
	Describe() string
}

// New builds the provider selected by cfg.
func New(cfg config.Config) (Provider, error) {
	resolved, err := cfg.Resolve()
	if err != nil {
		return nil, err
	}

	if resolved.Provider == "local" {
		return newLocal()
	}
	return newCloud(resolved)
}

// buildSystemPrompt constructs the system prompt with shell context and the
// user's remembered notes. It is read fresh on every call, so an edit to the
// memory file takes effect on the next inference, including a refinement.
func buildSystemPrompt() string {
	var sb strings.Builder
	sb.WriteString("Convert the instruction to a single POSIX sh command. Output ONLY the command, no explanation, no markdown, no backticks.\n")

	if cwd, err := os.Getwd(); err == nil {
		sb.WriteString("Current directory: " + cwd + "\n")
	}

	if notes := memory.Prompt(); notes != "" {
		sb.WriteString(notes)
	}

	return sb.String()
}

// stripMarkdown extracts a bare command from the output.
// If a fenced code block exists anywhere, returns its first line.
// Falls back to the first non-empty line.
func stripMarkdown(s string) string {
	if idx := strings.Index(s, "```"); idx != -1 {
		rest := s[idx+3:]
		if nl := strings.Index(rest, "\n"); nl != -1 {
			rest = rest[nl+1:]
		}
		if end := strings.Index(rest, "```"); end != -1 {
			rest = rest[:end]
		}
		for _, line := range strings.Split(rest, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				return line
			}
		}
	}

	s = strings.Trim(s, "`")
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "$ ")
		if line != "" {
			return line
		}
	}

	return strings.TrimSpace(s)
}

// missingLocalRuntime reports the install hint shown when llama.cpp or a model
// is absent, pointing at cloud setup as the lighter alternative.
func missingLocalRuntime(what string) error {
	return fmt.Errorf("%s\n\nInstall the local runtime:\n  curl -fsSL https://raw.githubusercontent.com/30Signals/ai-sh/main/install.sh | bash\nOr use a cloud model instead:\n  ai --setup", what)
}
