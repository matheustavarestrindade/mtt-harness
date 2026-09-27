package memory

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type Store struct {
	mu          sync.RWMutex
	instances   map[string]atom.InstanceSpec
	sessions    map[atom.SessionID]atom.Session
	messages    map[atom.SessionID][]atom.Message
	events      []atom.Event
	seq         uint64
	processes   map[string]atom.ProcessRecord
	permissions map[string]atom.PermissionDecision
	usage       []atom.UsageRecord
	providers   map[string]atom.ProviderSpec
	models      map[string][]atom.ModelInfo
}

func New() *Store {
	return &Store{
		instances:   map[string]atom.InstanceSpec{},
		sessions:    map[atom.SessionID]atom.Session{},
		messages:    map[atom.SessionID][]atom.Message{},
		processes:   map[string]atom.ProcessRecord{},
		permissions: map[string]atom.PermissionDecision{},
		providers:   map[string]atom.ProviderSpec{},
		models:      map[string][]atom.ModelInfo{},
	}
}

func (s *Store) Instances() store.InstanceStore     { return &instances{s} }
func (s *Store) Sessions() store.SessionStore       { return &sessions{s} }
func (s *Store) Events() store.EventStore           { return &events{s} }
func (s *Store) Processes() store.ProcessStore      { return &processes{s} }
func (s *Store) Permissions() store.PermissionStore { return &permissions{s} }
func (s *Store) Usage() store.UsageStore            { return &usage{s} }
func (s *Store) Providers() store.ProviderStore     { return &providers{s} }
func (s *Store) Close() error                       { return nil }

type instances struct{ s *Store }

func (i *instances) Save(ctx context.Context, spec atom.InstanceSpec) error {
	i.s.mu.Lock()
	defer i.s.mu.Unlock()
	i.s.instances[spec.ID] = spec
	return nil
}

func (i *instances) Get(ctx context.Context, id string) (atom.InstanceSpec, error) {
	i.s.mu.RLock()
	defer i.s.mu.RUnlock()
	spec, ok := i.s.instances[id]
	if !ok {
		return atom.InstanceSpec{}, errors.New("memory: the instance is not in the store")
	}
	return spec, nil
}

func (i *instances) All(ctx context.Context) ([]atom.InstanceSpec, error) {
	i.s.mu.RLock()
	defer i.s.mu.RUnlock()
	list := make([]atom.InstanceSpec, 0, len(i.s.instances))
	for _, spec := range i.s.instances {
		list = append(list, spec)
	}
	sort.Slice(list, func(a, b int) bool { return list[a].ID < list[b].ID })
	return list, nil
}

func (i *instances) Delete(ctx context.Context, id string) error {
	i.s.mu.Lock()
	defer i.s.mu.Unlock()
	delete(i.s.instances, id)
	return nil
}

type sessions struct{ s *Store }

func (s *sessions) Save(ctx context.Context, session atom.Session) error {
	s.s.mu.Lock()
	defer s.s.mu.Unlock()
	s.s.sessions[session.ID] = session
	return nil
}

func (s *sessions) Get(ctx context.Context, id atom.SessionID) (atom.Session, error) {
	s.s.mu.RLock()
	defer s.s.mu.RUnlock()
	session, ok := s.s.sessions[id]
	if !ok {
		return atom.Session{}, errors.New("memory: the session is not in the store")
	}
	return session, nil
}

func (s *sessions) Agents(ctx context.Context, parent atom.SessionID) ([]atom.SessionID, error) {
	s.s.mu.RLock()
	defer s.s.mu.RUnlock()
	var list []atom.SessionID
	for id, session := range s.s.sessions {
		if session.Parent == parent {
			list = append(list, id)
		}
	}
	sort.Slice(list, func(a, b int) bool { return list[a] < list[b] })
	return list, nil
}

func (s *sessions) Append(ctx context.Context, message atom.Message) error {
	s.s.mu.Lock()
	defer s.s.mu.Unlock()
	s.s.messages[message.SessionID] = append(s.s.messages[message.SessionID], message)
	return nil
}

func (s *sessions) Messages(ctx context.Context, id atom.SessionID) ([]atom.Message, error) {
	s.s.mu.RLock()
	defer s.s.mu.RUnlock()
	return append([]atom.Message(nil), s.s.messages[id]...), nil
}

type events struct{ s *Store }

func (e *events) Append(ctx context.Context, event atom.Event) error {
	e.s.mu.Lock()
	defer e.s.mu.Unlock()
	e.s.seq++
	event.Seq = e.s.seq
	e.s.events = append(e.s.events, event)
	return nil
}

func (e *events) Since(ctx context.Context, instanceID string, seq uint64) ([]atom.Event, error) {
	e.s.mu.RLock()
	defer e.s.mu.RUnlock()
	var list []atom.Event
	for _, event := range e.s.events {
		if event.Seq > seq && (instanceID == "" || event.InstanceID == instanceID) {
			list = append(list, event)
		}
	}
	return list, nil
}

type processes struct{ s *Store }

func (p *processes) Save(ctx context.Context, process atom.ProcessRecord) error {
	p.s.mu.Lock()
	defer p.s.mu.Unlock()
	p.s.processes[process.ID] = process
	return nil
}

func (p *processes) Get(ctx context.Context, id string) (atom.ProcessRecord, error) {
	p.s.mu.RLock()
	defer p.s.mu.RUnlock()
	record, ok := p.s.processes[id]
	if !ok {
		return atom.ProcessRecord{}, errors.New("memory: the process is not in the store")
	}
	return record, nil
}

func (p *processes) List(ctx context.Context, session atom.SessionID) ([]atom.ProcessRecord, error) {
	p.s.mu.RLock()
	defer p.s.mu.RUnlock()
	var list []atom.ProcessRecord
	for _, record := range p.s.processes {
		if record.SessionID == session {
			list = append(list, record)
		}
	}
	sort.Slice(list, func(a, b int) bool { return list[a].StartedAt.Before(list[b].StartedAt) })
	return list, nil
}

type permissions struct{ s *Store }

func (p *permissions) Save(ctx context.Context, decision atom.PermissionDecision) error {
	p.s.mu.Lock()
	defer p.s.mu.Unlock()
	p.s.permissions[decision.RequestID] = decision
	return nil
}

func (p *permissions) Get(ctx context.Context, id string) (atom.PermissionDecision, error) {
	p.s.mu.RLock()
	defer p.s.mu.RUnlock()
	decision, ok := p.s.permissions[id]
	if !ok {
		return atom.PermissionDecision{}, errors.New("memory: the decision is not in the store")
	}
	return decision, nil
}

type usage struct{ s *Store }

func (u *usage) Save(ctx context.Context, record atom.UsageRecord) error {
	u.s.mu.Lock()
	defer u.s.mu.Unlock()
	u.s.usage = append(u.s.usage, record)
	return nil
}

func (u *usage) Session(ctx context.Context, id atom.SessionID) (atom.Statistics, error) {
	u.s.mu.RLock()
	defer u.s.mu.RUnlock()
	tree := u.s.sessionTree(id)
	var records []atom.UsageRecord
	for _, record := range u.s.usage {
		if tree[record.SessionID] {
			records = append(records, record)
		}
	}
	return statistics(records), nil
}

func (u *usage) Instance(ctx context.Context, id string) (atom.Statistics, error) {
	u.s.mu.RLock()
	defer u.s.mu.RUnlock()
	var records []atom.UsageRecord
	for _, record := range u.s.usage {
		if record.InstanceID == id {
			records = append(records, record)
		}
	}
	return statistics(records), nil
}

func (u *usage) All(ctx context.Context) (atom.Statistics, error) {
	u.s.mu.RLock()
	defer u.s.mu.RUnlock()
	return statistics(append([]atom.UsageRecord(nil), u.s.usage...)), nil
}

func (s *Store) sessionTree(root atom.SessionID) map[atom.SessionID]bool {
	tree := map[atom.SessionID]bool{root: true}
	for changed := true; changed; {
		changed = false
		for id, session := range s.sessions {
			if tree[session.Parent] && !tree[id] {
				tree[id] = true
				changed = true
			}
		}
	}
	return tree
}

func statistics(records []atom.UsageRecord) atom.Statistics {
	stats := atom.Statistics{Calls: len(records)}
	totals := map[string]float64{}
	for _, record := range records {
		stats.Input += record.Usage.Input
		stats.CacheRead += record.Usage.CacheRead
		stats.CacheWrite += record.Usage.CacheWrite
		stats.Output += record.Usage.Output
		stats.Reasoning += record.Usage.Reasoning
		if record.Usage.Cost != nil {
			totals[record.Usage.Cost.Currency] += record.Usage.Cost.Value
		}
	}
	currencies := make([]string, 0, len(totals))
	for currency := range totals {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)
	for _, currency := range currencies {
		stats.Costs = append(stats.Costs, atom.Cost{Currency: currency, Value: totals[currency]})
	}
	if len(stats.Costs) == 1 {
		cost := stats.Costs[0]
		stats.Cost = &cost
	}
	return stats
}

type providers struct{ s *Store }

func (p *providers) Save(ctx context.Context, spec atom.ProviderSpec) error {
	p.s.mu.Lock()
	defer p.s.mu.Unlock()
	p.s.providers[spec.Name] = spec
	return nil
}

func (p *providers) Get(ctx context.Context, id string) (atom.ProviderSpec, error) {
	p.s.mu.RLock()
	defer p.s.mu.RUnlock()
	spec, ok := p.s.providers[id]
	if !ok {
		return atom.ProviderSpec{}, errors.New("memory: the provider is not in the store")
	}
	return spec, nil
}

func (p *providers) All(ctx context.Context) ([]atom.ProviderSpec, error) {
	p.s.mu.RLock()
	defer p.s.mu.RUnlock()
	list := make([]atom.ProviderSpec, 0, len(p.s.providers))
	for _, spec := range p.s.providers {
		list = append(list, spec)
	}
	sort.Slice(list, func(a, b int) bool { return list[a].Name < list[b].Name })
	return list, nil
}

func (p *providers) Delete(ctx context.Context, id string) error {
	p.s.mu.Lock()
	defer p.s.mu.Unlock()
	delete(p.s.providers, id)
	return nil
}

func (p *providers) SaveModels(ctx context.Context, provider string, models []atom.ModelInfo) error {
	p.s.mu.Lock()
	defer p.s.mu.Unlock()
	p.s.models[provider] = append([]atom.ModelInfo(nil), models...)
	return nil
}

func (p *providers) Models(ctx context.Context, provider string) ([]atom.ModelInfo, error) {
	p.s.mu.RLock()
	defer p.s.mu.RUnlock()
	return append([]atom.ModelInfo(nil), p.s.models[provider]...), nil
}
