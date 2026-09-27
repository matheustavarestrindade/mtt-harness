package plugins

import (
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Host struct {
	mu     sync.RWMutex
	h      *harness.Harness
	loaded map[string]harness.Plugin
}

func New(h *harness.Harness) *Host {
	return &Host{h: h, loaded: map[string]harness.Plugin{}}
}

func (p *Host) Attach(plugin harness.Plugin) error {
	if err := plugin.Setup(p.h); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.loaded[plugin.Name()] = plugin
	return nil
}

func (p *Host) All() []harness.Plugin {
	p.mu.RLock()
	defer p.mu.RUnlock()
	list := make([]harness.Plugin, 0, len(p.loaded))
	for _, plugin := range p.loaded {
		list = append(list, plugin)
	}
	return list
}
