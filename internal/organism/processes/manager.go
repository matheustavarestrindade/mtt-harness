package processes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type outputter interface {
	Output() ([]byte, []byte)
}

type InstanceLimits interface {
	ProcessLimit(ctx context.Context, instanceID string) int
}

type Manager struct {
	mu         sync.RWMutex
	supervisor *process.Supervisor
	store      store.Store
	h          *harness.Harness
	bus        *eventbus.Bus
	limits     InstanceLimits
	instances  map[string]string
}

func New(supervisor *process.Supervisor, database store.Store, h *harness.Harness, bus *eventbus.Bus, limits InstanceLimits) *Manager {
	return &Manager{
		supervisor: supervisor,
		store:      database,
		h:          h,
		bus:        bus,
		limits:     limits,
		instances:  map[string]string{},
	}
}

func (m *Manager) Start(ctx context.Context, spec atom.ProcessSpec) (harness.Process, error) {
	session, _ := harness.SessionFrom(ctx)
	if session.InstanceID != "" && m.limits != nil {
		if limit := m.limits.ProcessLimit(ctx, session.InstanceID); limit > 0 {
			if count, err := m.store.Processes().CountRunning(ctx, session.InstanceID); err == nil && count >= limit {
				return nil, errors.New("process: the process limit of the instance is reached")
			}
		}
	}
	proc, err := m.supervisor.Start(ctx, spec)
	if err != nil {
		return nil, err
	}
	record := atom.ProcessRecord{
		ID:         proc.ID(),
		InstanceID: session.InstanceID,
		SessionID:  session.ID,
		Spec:       spec,
		PID:        proc.PID(),
		Status:     "running",
		StartedAt:  time.Now(),
	}
	_ = m.store.Processes().Save(ctx, record)
	m.mu.Lock()
	m.instances[proc.ID()] = session.InstanceID
	m.mu.Unlock()
	go m.observe(ctx, session, proc, spec)
	return proc, nil
}

func (m *Manager) StopInstance(ctx context.Context, instanceID string) int {
	m.mu.Lock()
	var ids []string
	for id, owner := range m.instances {
		if owner == instanceID {
			ids = append(ids, id)
			delete(m.instances, id)
		}
	}
	m.mu.Unlock()
	count := 0
	for _, id := range ids {
		if proc, ok := m.supervisor.Get(id); ok {
			if err := proc.Kill(atom.Signal("")); err == nil {
				count++
			}
		}
	}
	return count
}

func (m *Manager) Get(id string) (harness.Process, bool) {
	return m.supervisor.Get(id)
}

func (m *Manager) All() []harness.Process {
	return m.supervisor.All()
}

type subscriber interface {
	Subscribe() <-chan atom.ProcessEvent
}

func (m *Manager) Subscribe(id string) (<-chan atom.ProcessEvent, bool) {
	proc, ok := m.supervisor.Get(id)
	if !ok {
		return nil, false
	}
	handle, ok := proc.(subscriber)
	if !ok {
		return nil, false
	}
	return handle.Subscribe(), true
}

func (m *Manager) Output(id string) ([]byte, []byte, bool) {
	proc, ok := m.supervisor.Get(id)
	if !ok {
		return nil, nil, false
	}
	holder, ok := proc.(outputter)
	if !ok {
		return nil, nil, false
	}
	stdout, stderr := holder.Output()
	return stdout, stderr, true
}

func (m *Manager) observe(ctx context.Context, session atom.Session, proc harness.Process, spec atom.ProcessSpec) {
	events := proc.Events()
	var ticker *time.Ticker
	var ticks <-chan time.Time
	if spec.Notify.Mode == atom.NotifyInterval && spec.Notify.Interval > 0 {
		ticker = time.NewTicker(spec.Notify.Interval)
		ticks = ticker.C
		defer ticker.Stop()
	}
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			event, err := harness.Run(ctx, m.h, atom.StageProcessOutput, event)
			if err != nil {
				continue
			}
			for _, watcher := range m.h.Watchers() {
				if watcher.Match(event) {
					current := watcher
					go current.OnMatch(ctx, event)
				}
			}
			if event.Stream == atom.StreamExit {
				m.mu.Lock()
				delete(m.instances, proc.ID())
				m.mu.Unlock()
				m.finish(ctx, session, proc, spec, event)
				return
			}
		case <-ticks:
			m.send(ctx, session, fmt.Sprintf("process %s: %s", proc.ID(), m.tail(proc)))
		}
	}
}

func (m *Manager) finish(ctx context.Context, session atom.Session, proc harness.Process, spec atom.ProcessSpec, event atom.ProcessEvent) {
	record := atom.ProcessRecord{
		ID:         proc.ID(),
		InstanceID: session.InstanceID,
		SessionID:  session.ID,
		Spec:       spec,
		PID:        proc.PID(),
		Status:     "stopped",
		StartedAt:  time.Now(),
		EndedAt:    time.Now(),
	}
	if exit, err := proc.Wait(); err == nil {
		record.Exit = &exit
	}
	_ = m.store.Processes().Save(ctx, record)
	switch spec.Notify.Mode {
	case atom.NotifyError:
		if event.Error != "" {
			m.send(ctx, session, fmt.Sprintf("process %s stopped with an error: %s", proc.ID(), event.Error))
		}
	case atom.NotifyInterval:
		m.send(ctx, session, fmt.Sprintf("process %s stopped", proc.ID()))
	default:
		m.send(ctx, session, fmt.Sprintf("process %s stopped", proc.ID()))
	}
}

func (m *Manager) tail(proc harness.Process) string {
	holder, ok := proc.(outputter)
	if !ok {
		return ""
	}
	stdout, stderr := holder.Output()
	if len(stderr) > 0 {
		return string(stderr)
	}
	return string(stdout)
}

func (m *Manager) send(ctx context.Context, session atom.Session, text string) {
	if session.ID == "" {
		return
	}
	message := atom.Message{
		ID:        newID(),
		SessionID: session.ID,
		Role:      atom.RoleUser,
		Content:   []atom.Content{{Type: atom.Text, Text: "[process] " + text}},
		CreatedAt: time.Now(),
	}
	_ = m.store.Sessions().Append(ctx, message)
	if m.bus != nil {
		m.bus.Send(ctx, atom.Event{
			InstanceID: session.InstanceID,
			SessionID:  session.ID,
			Name:       atom.EventProcessNotify,
			Time:       time.Now(),
		})
	}
}

func newID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(data[:])
}
