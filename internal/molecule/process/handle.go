package process

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// Handle owns its output buffers and subscriptions. The same lock protects
// publication and closure, so a subscriber is never sent to after it closes.
type Handle struct {
	identifier     string
	command        *exec.Cmd
	stdin          io.WriteCloser
	mutex          sync.Mutex
	subscribers    map[uint64]chan atom.ProcessEvent
	nextSubscriber uint64
	stdout, stderr []byte
	done           chan struct{}
	completed      bool
	exit           atom.ExitStatus
	exitEvent      atom.ProcessEvent
	startEvent     atom.ProcessEvent
	operationError error
	transform      func(atom.ProcessEvent) (atom.ProcessEvent, error)
}

func (processHandle *Handle) ID() string {
	return processHandle.identifier
}
func (processHandle *Handle) Running() bool {
	select {
	case <-processHandle.done:
		return false
	default:
		return true
	}
}
func (processHandle *Handle) PID() int {
	return processHandle.command.Process.Pid
}
func (processHandle *Handle) Write(data []byte) error {
	if processHandle.stdin == nil {
		return errors.New("process input is not open")
	}
	_, operationError := processHandle.stdin.Write(data)
	return operationError
}

func (processHandle *Handle) Kill(signal atom.Signal) error {
	select {
	case <-processHandle.done:
		return nil
	default:
	}
	return signalCommand(processHandle.command, signal)
}

func (processHandle *Handle) Wait() (atom.ExitStatus, error) {
	<-processHandle.done
	processHandle.mutex.Lock()
	defer processHandle.mutex.Unlock()
	return processHandle.exit, processHandle.operationError
}

func (processHandle *Handle) Events() <-chan atom.ProcessEvent {
	return processHandle.Subscribe()
}
func (processHandle *Handle) Subscribe() <-chan atom.ProcessEvent {
	return processHandle.SubscribeContext(context.Background())
}

// A late subscriber receives the retained output and terminal event. A live
// subscriber gets a snapshot and subsequent events with no subscription gap.
func (processHandle *Handle) SubscribeContext(operationContext context.Context) <-chan atom.ProcessEvent {
	channel := make(chan atom.ProcessEvent, 256)
	processHandle.mutex.Lock()
	for _, stream := range []atom.StreamName{atom.StreamStdout, atom.StreamStderr} {
		if stream == atom.StreamStdout {
			channel <- processHandle.startEvent
		}
		data := processHandle.stdout
		if stream == atom.StreamStderr {
			data = processHandle.stderr
		}
		if len(data) > 0 {
			channel <- atom.ProcessEvent{ProcessID: processHandle.identifier, Stream: stream, Data: append([]byte(nil), data...), At: time.Now()}
		}
	}
	if processHandle.completed {
		channel <- processHandle.exitEvent
		close(channel)
		processHandle.mutex.Unlock()
		return channel
	}
	processHandle.nextSubscriber++
	identifier := processHandle.nextSubscriber
	processHandle.subscribers[identifier] = channel
	processHandle.mutex.Unlock()
	go func() {
		select {
		case <-operationContext.Done():
		case <-processHandle.done:
		}
		processHandle.mutex.Lock()
		defer processHandle.mutex.Unlock()
		if current, found := processHandle.subscribers[identifier]; found {
			delete(processHandle.subscribers, identifier)
			close(current)
		}
	}()
	return channel
}

func (processHandle *Handle) Output() ([]byte, []byte) {
	processHandle.mutex.Lock()
	defer processHandle.mutex.Unlock()
	return append([]byte(nil), processHandle.stdout...), append([]byte(nil), processHandle.stderr...)
}

type outputWriter struct {
	handle *Handle
	stream atom.StreamName
}

func (writer outputWriter) Write(data []byte) (int, error) {
	event := atom.ProcessEvent{ProcessID: writer.handle.identifier, Stream: writer.stream, Data: append([]byte(nil), data...), At: time.Now()}
	if writer.handle.transform != nil {
		transformed, operationError := writer.handle.transform(event)
		if operationError != nil {
			return 0, operationError
		}
		event = transformed
	}
	writer.handle.mutex.Lock()
	defer writer.handle.mutex.Unlock()
	if writer.stream == atom.StreamStderr {
		writer.handle.stderr = appendCapped(writer.handle.stderr, event.Data)
	}
	if writer.stream == atom.StreamStdout {
		writer.handle.stdout = appendCapped(writer.handle.stdout, event.Data)
	}
	for _, channel := range writer.handle.subscribers {
		select {
		case channel <- event:
		default:
		}
	}
	return len(data), nil
}

// exec.Cmd owns the stdout/stderr copying goroutines and Wait joins them before
// this method publishes completion. No trailing output can race the exit event.
func (processHandle *Handle) waitForCommandExit() {
	operationError := processHandle.command.Wait()
	status := atom.ExitStatus{}
	if operationError != nil {
		var exitError *exec.ExitError
		if errors.As(operationError, &exitError) {
			status.Code = exitError.ExitCode()
		}
		status.Error = operationError.Error()
	}
	if processHandle.stdin != nil {
		_ = processHandle.stdin.Close()
	}
	exitEvent := atom.ProcessEvent{ProcessID: processHandle.identifier, Stream: atom.StreamExit, Error: status.Error, At: time.Now()}
	if processHandle.transform != nil {
		transformed, transformError := processHandle.transform(exitEvent)
		if transformError == nil {
			exitEvent = transformed
		}
		if transformError != nil {
			operationError = errors.Join(operationError, transformError)
			status.Error = operationError.Error()
		}
	}
	processHandle.mutex.Lock()
	defer processHandle.mutex.Unlock()
	processHandle.exit, processHandle.operationError, processHandle.completed = status, operationError, true
	processHandle.exitEvent = exitEvent
	for identifier, channel := range processHandle.subscribers {
		select {
		case channel <- processHandle.exitEvent:
		default:
			<-channel
			channel <- processHandle.exitEvent
		}
		close(channel)
		delete(processHandle.subscribers, identifier)
	}
	close(processHandle.done)
}

func appendCapped(buffer, data []byte) []byte {
	if len(data) >= outputLimit {
		return append([]byte(nil), data[len(data)-outputLimit:]...)
	}
	if overflow := len(buffer) + len(data) - outputLimit; overflow > 0 {
		buffer = buffer[overflow:]
	}
	return append(buffer, data...)
}
