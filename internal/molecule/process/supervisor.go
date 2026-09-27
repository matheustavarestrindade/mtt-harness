package process

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

const outputLimit = 256 * 1024

type Handle struct {
	id     string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	mu     sync.Mutex
	subs   []chan atom.ProcessEvent
	stdout []byte
	stderr []byte
	done   chan struct{}
	exit   atom.ExitStatus
	err    error
}

func (h *Handle) ID() string { return h.id }

func (h *Handle) PID() int {
	if h.cmd.Process == nil {
		return 0
	}
	return h.cmd.Process.Pid
}

func (h *Handle) Write(data []byte) error {
	if h.stdin == nil {
		return errors.New("process: the input of the process is not open")
	}
	_, err := h.stdin.Write(data)
	return err
}

func (h *Handle) Kill(signal atom.Signal) error {
	if h.cmd.Process == nil {
		return errors.New("process: the process is not started")
	}
	if signal == atom.Signal("interrupt") {
		return h.cmd.Process.Signal(os.Interrupt)
	}
	return h.cmd.Process.Kill()
}

func (h *Handle) Wait() (atom.ExitStatus, error) {
	<-h.done
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.exit, h.err
}

func (h *Handle) Events() <-chan atom.ProcessEvent {
	return h.Subscribe()
}

func (h *Handle) Subscribe() <-chan atom.ProcessEvent {
	channel := make(chan atom.ProcessEvent, 256)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subs = append(h.subs, channel)
	return channel
}

func (h *Handle) Output() ([]byte, []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]byte(nil), h.stdout...), append([]byte(nil), h.stderr...)
}

func (h *Handle) publish(event atom.ProcessEvent) {
	h.mu.Lock()
	subs := append([]chan atom.ProcessEvent(nil), h.subs...)
	h.mu.Unlock()
	for _, channel := range subs {
		select {
		case channel <- event:
		default:
		}
	}
}

func (h *Handle) read(stream atom.StreamName, reader io.Reader) {
	buffer := make([]byte, 4096)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			data := append([]byte(nil), buffer[:n]...)
			h.append(stream, data)
			h.publish(atom.ProcessEvent{
				ProcessID: h.id,
				Stream:    stream,
				Data:      data,
				At:        time.Now(),
			})
		}
		if err != nil {
			return
		}
	}
}

func (h *Handle) append(stream atom.StreamName, data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if stream == atom.StreamStderr {
		h.stderr = appendCapped(h.stderr, data)
		return
	}
	h.stdout = appendCapped(h.stdout, data)
}

func appendCapped(buffer []byte, data []byte) []byte {
	buffer = append(buffer, data...)
	if len(buffer) > outputLimit {
		buffer = buffer[len(buffer)-outputLimit:]
	}
	return buffer
}

func (h *Handle) wait() {
	err := h.cmd.Wait()
	status := atom.ExitStatus{}
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			status.Code = exitError.ExitCode()
		}
		status.Error = err.Error()
	}
	h.mu.Lock()
	h.exit = status
	h.err = err
	h.mu.Unlock()
	close(h.done)
	h.publish(atom.ProcessEvent{
		ProcessID: h.id,
		Stream:    atom.StreamExit,
		Error:     status.Error,
		At:        time.Now(),
	})
	h.mu.Lock()
	for _, channel := range h.subs {
		close(channel)
	}
	h.subs = nil
	h.mu.Unlock()
	if h.stdin != nil {
		_ = h.stdin.Close()
	}
}

type Supervisor struct {
	mu    sync.RWMutex
	procs map[string]*Handle
	limit int
}

func New(limit int) *Supervisor {
	return &Supervisor{
		procs: map[string]*Handle{},
		limit: limit,
	}
}

func (s *Supervisor) Start(ctx context.Context, spec atom.ProcessSpec) (harness.Process, error) {
	if spec.Command == "" {
		return nil, errors.New("process: the command is necessary")
	}
	s.mu.Lock()
	if s.limit > 0 && len(s.procs) >= s.limit {
		s.mu.Unlock()
		return nil, errors.New("process: the process limit is reached")
	}
	s.mu.Unlock()
	command := exec.Command(spec.Command, spec.Args...)
	command.Dir = spec.Cwd
	command.Env = append(os.Environ(), spec.Env...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, err
	}
	handle := &Handle{
		id:    newID(),
		cmd:   command,
		stdin: stdin,
		done:  make(chan struct{}),
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	go handle.read(atom.StreamStdout, stdout)
	go handle.read(atom.StreamStderr, stderr)
	go handle.wait()
	s.mu.Lock()
	s.procs[handle.id] = handle
	s.mu.Unlock()
	handle.publish(atom.ProcessEvent{
		ProcessID: handle.id,
		Stream:    atom.StreamStart,
		At:        time.Now(),
	})
	if spec.Timeout > 0 {
		go func() {
			select {
			case <-handle.done:
			case <-time.After(spec.Timeout):
				_ = command.Process.Kill()
			}
		}()
	}
	go func() {
		<-handle.done
		s.mu.Lock()
		delete(s.procs, handle.id)
		s.mu.Unlock()
	}()
	return handle, nil
}

func (s *Supervisor) Get(id string) (harness.Process, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	handle, ok := s.procs[id]
	return handle, ok
}

func (s *Supervisor) All() []harness.Process {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]harness.Process, 0, len(s.procs))
	for _, handle := range s.procs {
		list = append(list, handle)
	}
	return list
}

func newID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(data[:])
}
