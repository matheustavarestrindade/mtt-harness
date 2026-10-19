//go:build unix

package process

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func awaitTerminalText(test *testing.T, handle *Handle, text string) {
	test.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		output, _ := handle.Output()
		if strings.Contains(string(output), text) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	output, _ := handle.Output()
	test.Fatalf("terminal did not produce %q: %q", text, output)
}

func TestTerminalProvidesTTYColorsInputResizeAndControlC(test *testing.T) {
	supervisor := New(4)
	defer supervisor.Close(context.Background())
	running, operationError := supervisor.Start(context.Background(), atom.ProcessSpec{Command: "sh", Args: []string{"-c", `test -t 0 && test -t 1 && test -t 2 || exit 99; printf '\033[32mTTY_READY\033[0m\n'; IFS= read -r answer; printf 'ANSWER=%s\n' "$answer"; stty size; sleep 30`}, Terminal: true, Columns: 120, Rows: 30})
	testutil.RequireNoError(test, operationError)
	handle := running.(*Handle)
	awaitTerminalText(test, handle, "TTY_READY")
	testutil.RequireNoError(test, handle.ResizeTerminal(91, 27))
	inputContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	testutil.RequireNoError(test, handle.WriteContext(inputContext, []byte("hello 🌍\n")))
	awaitTerminalText(test, handle, "ANSWER=hello 🌍")
	awaitTerminalText(test, handle, "27 91")
	testutil.RequireNoError(test, handle.WriteContext(inputContext, []byte{3}))
	select {
	case <-handle.done:
	case <-time.After(3 * time.Second):
		test.Fatal("Ctrl-C did not end the foreground process group")
	}
	stdout, stderr := handle.Output()
	if len(stderr) != 0 || !bytes.Contains(stdout, []byte("\x1b[32mTTY_READY\x1b[0m\r\n")) {
		test.Fatalf("PTY output was rewritten or split: %q %q", stdout, stderr)
	}
	if !handle.HasTerminal() {
		test.Fatal("terminal capability is missing")
	}
	if operationError := handle.WriteContext(context.Background(), []byte("late")); operationError == nil {
		test.Fatal("input after exit was accepted")
	}
}

func TestTerminalInputCancellationDoesNotStopTheProcess(test *testing.T) {
	supervisor := New(2)
	defer supervisor.Close(context.Background())
	running, operationError := supervisor.Start(context.Background(), atom.ProcessSpec{Command: "sh", Args: []string{"-c", `stty raw -echo; printf READY; sleep 30`}, Terminal: true})
	testutil.RequireNoError(test, operationError)
	handle := running.(*Handle)
	awaitTerminalText(test, handle, "READY")
	operationContext, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	var writeError error
	for index := 0; index < 128; index++ {
		writeError = handle.WriteContext(operationContext, bytes.Repeat([]byte("x"), 4096))
		if writeError != nil {
			break
		}
	}
	if !errors.Is(writeError, context.DeadlineExceeded) {
		test.Fatalf("blocked terminal input did not honor cancellation: %v", writeError)
	}
	if !handle.Running() {
		test.Fatal("input cancellation stopped the command")
	}
}

func TestTerminalExitDrainsFinalOutputBeforeExitEvent(test *testing.T) {
	supervisor := New(2)
	defer supervisor.Close(context.Background())
	running, operationError := supervisor.Start(context.Background(), atom.ProcessSpec{Command: "sh", Args: []string{"-c", `printf '\033[38;2;18;171;99mfinal 🌍\033[0m\n'`}, Terminal: true})
	testutil.RequireNoError(test, operationError)
	status, operationError := running.Wait()
	testutil.RequireNoError(test, operationError)
	if status.Code != 0 {
		test.Fatal(status)
	}
	handle := running.(*Handle)
	stdout, _ := handle.Output()
	if string(stdout) != "\x1b[38;2;18;171;99mfinal 🌍\x1b[0m\r\n" {
		test.Fatalf("missing final bytes: %q", stdout)
	}
	var sawOutput bool
	for event := range handle.Subscribe() {
		if event.Stream == atom.StreamStdout {
			sawOutput = true
		}
		if event.Stream == atom.StreamExit && !sawOutput {
			test.Fatal("exit preceded retained bytes")
		}
	}
}
