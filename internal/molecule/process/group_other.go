//go:build !unix

package process

import (
	"os"
	"os/exec"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func prepareCommand(command *exec.Cmd) {}
func signalCommand(command *exec.Cmd, signal atom.Signal) error {
	if signal == "interrupt" {
		return command.Process.Signal(os.Interrupt)
	}
	return command.Process.Kill()
}
