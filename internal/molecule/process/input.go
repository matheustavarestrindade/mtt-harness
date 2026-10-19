package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

const MaximumInputBytes = 4096

func (processHandle *Handle) HasTerminal() bool { return processHandle.terminal != nil }

// WriteContext serializes input frames and cancels both admission and a blocked
// write. The terminal/pipe stays open when an individual client disconnects.
func (processHandle *Handle) WriteContext(operationContext context.Context, data []byte) error {
	if len(data) > MaximumInputBytes {
		return fmt.Errorf("process input exceeds %d bytes", MaximumInputBytes)
	}
	if len(data) == 0 {
		return nil
	}
	select {
	case processHandle.inputGate <- struct{}{}:
	case <-operationContext.Done():
		return operationContext.Err()
	case <-processHandle.done:
		return fmt.Errorf("process has exited")
	}
	defer func() { <-processHandle.inputGate }()
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	if !processHandle.Running() {
		return fmt.Errorf("process has exited")
	}
	input, ok := processHandle.stdin.(*os.File)
	if !ok {
		return errors.New("process input is not available")
	}
	if deadline, found := operationContext.Deadline(); found {
		if operationError := input.SetWriteDeadline(deadline); operationError != nil {
			return operationError
		}
	}
	callbackDone := make(chan struct{})
	stop := context.AfterFunc(operationContext, func() { _ = input.SetWriteDeadline(time.Now()); close(callbackDone) })
	defer func() {
		if !stop() {
			<-callbackDone
		}
		_ = input.SetWriteDeadline(time.Time{})
	}()
	for len(data) > 0 {
		written, operationError := input.Write(data)
		if operationContext.Err() != nil {
			return operationContext.Err()
		}
		if errors.Is(operationError, os.ErrDeadlineExceeded) {
			if deadline, found := operationContext.Deadline(); found && !time.Now().Before(deadline) {
				return context.DeadlineExceeded
			}
		}
		if operationError != nil {
			return operationError
		}
		data = data[written:]
	}
	return nil
}

func (processHandle *Handle) ResizeTerminal(columns, rows int) error {
	if columns < 2 || columns > 500 || rows < 1 || rows > 200 {
		return fmt.Errorf("terminal size must be 2–500 columns and 1–200 rows")
	}
	if processHandle.terminal == nil {
		return fmt.Errorf("process has no terminal")
	}
	if !processHandle.Running() {
		return fmt.Errorf("process has exited")
	}
	return resizeTerminal(processHandle.terminal, columns, rows)
}
