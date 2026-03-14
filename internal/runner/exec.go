package runner

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

// InferFunc re-runs inference with a new prompt and returns the command.
type InferFunc func(prompt string) (string, error)

// ConfirmAndRun shows the command, lets the user run, refine, or cancel.
func ConfirmAndRun(command, originalPrompt string, infer InferFunc) error {
	for {
		fmt.Printf("\nai:\n\033[1m%s\033[0m\n\n", command)
		fmt.Print("\033[1m↵\033[0m run   \033[1me\033[0m refine   \033[1mn\033[0m cancel  ")

		key, err := readKey()
		fmt.Println()
		if err != nil {
			return err
		}

		switch key {
		case '\r', '\n':
			return runCommand(command)

		case 'e', 'E':
			fmt.Print("Refine: ")
			line, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil {
				return err
			}
			feedback := strings.TrimSpace(line)
			if feedback == "" {
				continue
			}
			refined := originalPrompt + " — " + feedback
			fmt.Println("Thinking...")
			command, err = infer(refined)
			if err != nil {
				return err
			}
			if command == "" {
				fmt.Println("Model returned nothing, try again.")
				continue
			}

		default:
			fmt.Println("Cancelled.")
			return nil
		}
	}
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
