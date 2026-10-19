//go:build unix

package process

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/creack/pty"
)

func startTerminal(command *exec.Cmd, columns, rows int, explicitEnvironment []string) (*os.File, error) {
	// The PTY owns a new session/controlling terminal. Setpgid and Setsid cannot
	// both be requested; the new session already gives the shell its own group.
	command.SysProcAttr.Setpgid = false
	if !hasEnvironment(explicitEnvironment, "TERM") {
		command.Env = append(command.Env, "TERM=xterm-256color")
	}
	if !hasEnvironment(explicitEnvironment, "COLORTERM") {
		command.Env = append(command.Env, "COLORTERM=truecolor")
	}
	terminal, operationError := pty.StartWithSize(command, &pty.Winsize{Cols: uint16(columns), Rows: uint16(rows)})
	if operationError != nil {
		return nil, operationError
	}
	// Pollable I/O permits cancellation/deadlines for a full terminal input buffer.
	if operationError = syscall.SetNonblock(int(terminal.Fd()), true); operationError != nil {
		_ = signalCommand(command, "")
		_ = terminal.Close()
		_ = command.Wait()
		return nil, operationError
	}
	return terminal, nil
}

func hasEnvironment(environment []string, name string) bool {
	for _, entry := range environment {
		if strings.HasPrefix(entry, name+"=") {
			return true
		}
	}
	return false
}

func resizeTerminal(terminal *os.File, columns, rows int) error {
	return pty.Setsize(terminal, &pty.Winsize{Cols: uint16(columns), Rows: uint16(rows)})
}

func terminalReadError(operationError error) error {
	// Linux reports EIO when the last slave closes; other PTYs report EOF.
	if errors.Is(operationError, syscall.EIO) || errors.Is(operationError, io.EOF) {
		return nil
	}
	return operationError
}
