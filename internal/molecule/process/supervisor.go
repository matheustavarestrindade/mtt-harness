package process

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

const outputLimit = 256 * 1024
const retainedProcessLimit = 128

type Supervisor struct {
	mutex              sync.RWMutex
	runningProcesses   map[string]*Handle
	completedProcesses map[string]*Handle
	completionOrder    []string
	limit              int
	closed             bool
}

func New(limit int) *Supervisor {
	return &Supervisor{runningProcesses: map[string]*Handle{}, completedProcesses: map[string]*Handle{}, limit: limit}
}

func (processSupervisor *Supervisor) Start(operationContext context.Context, processSpec atom.ProcessSpec) (harness.Process, error) {
	return processSupervisor.StartWithTransform(operationContext, processSpec, nil)
}

func (processSupervisor *Supervisor) StartWithTransform(operationContext context.Context, processSpec atom.ProcessSpec, transform func(atom.ProcessEvent) (atom.ProcessEvent, error)) (harness.Process, error) {
	if processSpec.Command == "" {
		return nil, fmt.Errorf("process command is required")
	}
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	command := exec.Command(processSpec.Command, processSpec.Args...)
	command.Dir, command.Env = processSpec.Cwd, append(os.Environ(), processSpec.Env...)
	prepareCommand(command)
	stdin, operationError := command.StdinPipe()
	if operationError != nil {
		return nil, operationError
	}
	var identifierBytes [16]byte
	if _, operationError := rand.Read(identifierBytes[:]); operationError != nil {
		stdin.Close()
		return nil, operationError
	}
	handle := &Handle{identifier: hex.EncodeToString(identifierBytes[:]), command: command, stdin: stdin, done: make(chan struct{}), subscribers: map[uint64]chan atom.ProcessEvent{}}
	handle.transform = transform
	handle.startEvent = atom.ProcessEvent{ProcessID: handle.identifier, Stream: atom.StreamStart, At: time.Now()}
	if transform != nil {
		handle.startEvent, operationError = transform(handle.startEvent)
		if operationError != nil {
			stdin.Close()
			return nil, operationError
		}
	}
	command.Stdout, command.Stderr = outputWriter{handle, atom.StreamStdout}, outputWriter{handle, atom.StreamStderr}
	processSupervisor.mutex.Lock()
	defer processSupervisor.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		stdin.Close()
		return nil, operationError
	}
	if processSupervisor.closed {
		stdin.Close()
		return nil, fmt.Errorf("process supervisor is closed")
	}
	active := 0
	for _, process := range processSupervisor.runningProcesses {
		if process.Running() {
			active++
		}
	}
	if processSupervisor.limit > 0 && active >= processSupervisor.limit {
		stdin.Close()
		return nil, fmt.Errorf("process limit reached")
	}
	if operationError := command.Start(); operationError != nil {
		stdin.Close()
		return nil, operationError
	}
	processSupervisor.runningProcesses[handle.identifier] = handle
	go func() {
		handle.wait()
		processSupervisor.retain(handle)
	}()
	if processSpec.Timeout > 0 {
		go func() {
			timer := time.NewTimer(processSpec.Timeout)
			defer timer.Stop()
			select {
			case <-handle.done:
			case <-timer.C:
				_ = handle.Kill("")
			}
		}()
	}
	return handle, nil
}

func (processSupervisor *Supervisor) retain(handle *Handle) {
	processSupervisor.mutex.Lock()
	defer processSupervisor.mutex.Unlock()
	delete(processSupervisor.runningProcesses, handle.identifier)
	processSupervisor.completedProcesses[handle.identifier] = handle
	processSupervisor.completionOrder = append(processSupervisor.completionOrder, handle.identifier)
	if len(processSupervisor.completionOrder) > retainedProcessLimit {
		delete(processSupervisor.completedProcesses, processSupervisor.completionOrder[0])
		processSupervisor.completionOrder = processSupervisor.completionOrder[1:]
	}
}

func (processSupervisor *Supervisor) Get(identifier string) (harness.Process, bool) {
	processSupervisor.mutex.RLock()
	defer processSupervisor.mutex.RUnlock()
	if handle, found := processSupervisor.runningProcesses[identifier]; found {
		return handle, true
	}
	handle, found := processSupervisor.completedProcesses[identifier]
	return handle, found
}

func (processSupervisor *Supervisor) All() []harness.Process {
	processSupervisor.mutex.RLock()
	defer processSupervisor.mutex.RUnlock()
	var processes []harness.Process
	for _, handle := range processSupervisor.runningProcesses {
		processes = append(processes, handle)
	}
	return processes
}

func (processSupervisor *Supervisor) Close(operationContext context.Context) error {
	processSupervisor.mutex.Lock()
	processSupervisor.closed = true
	processSupervisor.mutex.Unlock()
	var failures []error
	for _, runningProcess := range processSupervisor.All() {
		failures = append(failures, runningProcess.Kill(""))
		handle := runningProcess.(*Handle)
		select {
		case <-handle.done:
		case <-operationContext.Done():
			return errors.Join(append(failures, operationContext.Err())...)
		}
	}
	return errors.Join(failures...)
}
