package plugins

import (
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Host struct {
	mutex          sync.RWMutex
	harnessRuntime *harness.Harness
	loaded         map[string]harness.Plugin
}

func New(harnessRuntime *harness.Harness) *Host {
	return &Host{harnessRuntime: harnessRuntime, loaded: map[string]harness.Plugin{}}
}

func (pluginHost *Host) Attach(plugin harness.Plugin) error {
	if operationError := plugin.Setup(pluginHost.harnessRuntime); operationError != nil {
		return operationError
	}
	pluginHost.mutex.Lock()
	defer pluginHost.mutex.Unlock()
	pluginHost.loaded[plugin.Name()] = plugin
	return nil
}

func (pluginHost *Host) All() []harness.Plugin {
	pluginHost.mutex.RLock()
	defer pluginHost.mutex.RUnlock()
	list := make([]harness.Plugin, 0, len(pluginHost.loaded))
	for _, plugin := range pluginHost.loaded {
		list = append(list, plugin)
	}
	return list
}
