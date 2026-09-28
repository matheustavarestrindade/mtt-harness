//go:build unix

package process

import (
	"errors"
	"os/exec"
	"syscall"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func prepareCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalCommand(command *exec.Cmd, signal atom.Signal) error {
	systemSignal := syscall.SIGKILL
	if signal == "interrupt" {
		systemSignal = syscall.SIGINT
	}
	operationError := syscall.Kill(-command.Process.Pid, systemSignal)
	if errors.Is(operationError, syscall.ESRCH) {
		return nil
	}
	return operationError
}
