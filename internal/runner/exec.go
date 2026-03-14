package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

// ConfirmAndRun shows the command in a box, prompts the user, and runs it if confirmed.
func ConfirmAndRun(command string) error {
	fmt.Printf("\nai:\n\033[1m%s\033[0m\n\n", command)
	fmt.Print("\033[1m↵\033[0m run   \033[1me\033[0m edit   \033[1mn\033[0m cancel  ")

	key, err := readKey()
	fmt.Println()
	if err != nil {
		return err
	}

	switch key {
	case '\r', '\n': // Enter — run as-is
		return runCommand(command)
	case 'e', 'E': // Edit in $EDITOR, then run
		edited, err := editCommand(command)
		if err != nil {
			return err
		}
		edited = strings.TrimSpace(edited)
		if edited == "" {
			fmt.Println("Cancelled.")
			return nil
		}
		return runCommand(edited)
	default: // anything else — cancel
		fmt.Println("Cancelled.")
		return nil
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

// editCommand opens $EDITOR with the command in a temp file and returns the edited result.
func editCommand(command string) (string, error) {
	tmp, err := os.CreateTemp("", "ai-*.sh")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(command + "\n"); err != nil {
		return "", err
	}
	tmp.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.Command(editor, tmp.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		return "", err
	}
	// Return first non-empty line
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line, nil
		}
	}
	return "", nil
}

func runCommand(command string) error {
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
