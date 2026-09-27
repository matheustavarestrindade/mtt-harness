package process

import (
	"context"
	"errors"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Supervisor struct {
	mu    sync.RWMutex
	procs map[string]harness.Process
	limit int
}

func New(limit int) *Supervisor {
	return &Supervisor{
		procs: map[string]harness.Process{},
		limit: limit,
	}
}

func (s *Supervisor) Start(ctx context.Context, spec atom.ProcessSpec) (harness.Process, error) {
	return nil, errors.New("process: start is not implemented")
}

func (s *Supervisor) Get(id string) (harness.Process, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	proc, ok := s.procs[id]
	return proc, ok
}

func (s *Supervisor) All() []harness.Process {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]harness.Process, 0, len(s.procs))
	for _, proc := range s.procs {
		list = append(list, proc)
	}
	return list
}
