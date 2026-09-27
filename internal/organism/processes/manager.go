package processes

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
)

type Manager struct {
	supervisor *process.Supervisor
}

func New(supervisor *process.Supervisor) *Manager {
	return &Manager{supervisor: supervisor}
}

func (m *Manager) Start(ctx context.Context, spec atom.ProcessSpec) (harness.Process, error) {
	return m.supervisor.Start(ctx, spec)
}

func (m *Manager) Get(id string) (harness.Process, bool) {
	return m.supervisor.Get(id)
}

func (m *Manager) All() []harness.Process {
	return m.supervisor.All()
}
