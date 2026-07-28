package llm

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// local runs inference through a llama.cpp binary on this machine.
type local struct {
	llamaPath string
	modelPath string
}

func newLocal() (Provider, error) {
	llamaPath, err := FindLlamaCLI()
	if err != nil {
		return nil, missingLocalRuntime("llama-cli not found in ~/.ai-sh/bin/.")
	}

	modelPath, err := FindModel()
	if err != nil {
		return nil, missingLocalRuntime("no .gguf model found in ~/.ai-sh/models/.")
	}

	return &local{llamaPath: llamaPath, modelPath: modelPath}, nil
}

func (l *local) Describe() string {
	return "local: " + filepath.Base(l.modelPath)
}

// FindLlamaCLI searches for the llama-cli binary in known locations.
func FindLlamaCLI() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	candidates := []string{
		filepath.Join(home, ".ai-sh", "bin", "llama-cli"),
	}

	if path, err := exec.LookPath("llama-cli"); err == nil {
		candidates = append(candidates, path)
	}

	candidates = append(candidates,
		"/opt/homebrew/bin/llama-cli",
		"/usr/local/bin/llama-cli",
	)

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("llama-cli not found")
}

// FindModel returns the path to the first .gguf file in ~/.ai-sh/models/.
func FindModel() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	modelsDir := filepath.Join(home, ".ai-sh", "models")
	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		return "", fmt.Errorf("no model found in ~/.ai-sh/models/")
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".gguf") {
			return filepath.Join(modelsDir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("no model found in ~/.ai-sh/models/")
}

// Generate runs llama-cli with the given prompt and returns the reply: a bare
// command, or prose carrying AnswerPrefix when the request was not a shell task.
func (l *local) Generate(userPrompt string) (string, error) {
	systemPrompt := buildSystemPrompt()

	args := []string{
		"-m", l.modelPath,
		"-sys", systemPrompt,
		"-p", userPrompt,
		"-n", "220",
		"--temp", "0.1",
		"-ngl", "0",
		"--no-display-prompt",
		"--log-disable",
		"-cnv",
		"-st",
	}

	cmd := exec.Command(l.llamaPath, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard
	// Run in a new session so llama-cli has no controlling terminal.
	// Without /dev/tty, its UI output falls through to stdout/stderr
	// where we can capture it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("running inference: llama-cli failed: %w", err)
	}

	return formatReply(cleanOutput(stdout.String(), userPrompt)), nil
}

// cleanOutput extracts the model reply from llama-cli conversation output.
// Format: ...preamble... \n> <userPrompt>\n\n<reply>\n\n[ Prompt: ... ]
func cleanOutput(raw, userPrompt string) string {
	if marker := "> " + userPrompt; strings.Contains(raw, marker) {
		raw = raw[strings.Index(raw, marker)+len(marker):]
	}
	if i := strings.Index(raw, "[ Prompt:"); i != -1 {
		raw = raw[:i]
	}
	raw = stripBackspaces(raw)
	return strings.TrimSpace(raw)
}

// stripBackspaces processes backspace (\x08) control characters, removing
// each backspace and the character before it (terminal spinner artifacts).
func stripBackspaces(s string) string {
	b := []byte(s)
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c == '\x08' {
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		} else {
			out = append(out, c)
		}
	}
	return string(out)
}
