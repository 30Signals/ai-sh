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

// RunInference runs llama-cli with the given prompt and returns the generated command.
func RunInference(llamaPath, modelPath, userPrompt string) (string, error) {
	systemPrompt := "Convert the instruction to a single bash command. Output ONLY the command, no explanation, no markdown, no backticks."

	args := []string{
		"-m", modelPath,
		"-sys", systemPrompt,
		"-p", userPrompt,
		"-n", "100",
		"--temp", "0.1",
		"-ngl", "0",
		"--no-display-prompt",
		"--log-disable",
		"-cnv",
		"-st",
	}

	cmd := exec.Command(llamaPath, args...)
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

	output := cleanOutput(stdout.String(), userPrompt)
	output = stripMarkdown(output)

	return output, nil
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
