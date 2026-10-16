//go:build !unix

package process

import (
	"fmt"
	"os"
	"os/exec"
)

func startTerminal(*exec.Cmd, int, int, []string) (*os.File, error) {
	return nil, fmt.Errorf("terminal processes are not supported on this platform")
}
func resizeTerminal(*os.File, int, int) error {
	return fmt.Errorf("terminal resize is not supported on this platform")
}
func terminalReadError(operationError error) error { return operationError }
