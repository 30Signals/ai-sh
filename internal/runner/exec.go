package runner

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"github.com/user/ai-sh/internal/llm"
)

// InferFunc re-runs inference over a conversation and returns the command.
type InferFunc func(messages []llm.Message) (string, error)

// Outcome reports what the user settled on, so the caller can record it.
type Outcome struct {
	// Command is the last command shown, after any refinement.
	Command string
	// Ran is false when the user cancelled.
	Ran bool
	// ExitCode is meaningful only when Ran.
	ExitCode int
}

// ConfirmAndRun shows the command, lets the user run, refine, or cancel.
//
// messages is the conversation that produced command, ending with the user's
// instruction. Refining appends the command as an assistant turn and the
// feedback as a user turn, so the model corrects its own output instead of
// re-answering a concatenated prompt.
func ConfirmAndRun(command string, messages []llm.Message, infer InferFunc) (Outcome, error) {
	// Copy: appending on refine must not write into the caller's array.
	convo := append([]llm.Message(nil), messages...)

	for {
		fmt.Printf("\nai:\n\033[1m%s\033[0m\n\n", command)
		fmt.Print("\033[1m↵\033[0m run   \033[1me\033[0m refine   \033[1mn\033[0m cancel  ")

		key, err := readKey()
		fmt.Println()
		if err != nil {
			return Outcome{Command: command}, err
		}

		switch key {
		case '\r', '\n':
			runErr := runCommand(command)
			return Outcome{Command: command, Ran: true, ExitCode: exitCode(runErr)}, runErr

		case 'e', 'E':
			fmt.Print("Refine: ")
			line, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil {
				return Outcome{Command: command}, err
			}
			feedback := strings.TrimSpace(line)
			if feedback == "" {
				continue
			}

			convo = append(convo, llm.Assistant(command), llm.User(feedback))
			fmt.Println("Thinking...")
			refined, err := infer(convo)
			if err != nil {
				return Outcome{Command: command}, err
			}
			if refined == "" {
				fmt.Println("Model returned nothing, try again.")
				// Drop the dead exchange so the next refinement is not
				// anchored to an empty answer.
				convo = convo[:len(convo)-2]
				continue
			}
			command = refined

		default:
			fmt.Println("Cancelled.")
			return Outcome{Command: command}, nil
		}
	}
}

// exitCode maps a command's error to a shell-style status.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

// readKey reads a single keypress without requiring Enter.
func readKey() (byte, error) {
	fd := int(os.Stdin.Fd())

	var oldState syscall.Termios
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), ioctlReadTermios, uintptr(unsafe.Pointer(&oldState))); errno != 0 {
		return 0, errno
	}

	raw := oldState
	raw.Lflag &^= syscall.ECHO | syscall.ICANON
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), ioctlWriteTermios, uintptr(unsafe.Pointer(&raw))); errno != 0 {
		return 0, errno
	}
	defer syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), ioctlWriteTermios, uintptr(unsafe.Pointer(&oldState)))

	buf := make([]byte, 1)
	os.Stdin.Read(buf)
	return buf[0], nil
}

func runCommand(command string) error {
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
