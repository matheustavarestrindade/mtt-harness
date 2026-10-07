package plugins

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Host struct {
	mutex           sync.RWMutex
	harnessRuntime  *harness.Harness
	loaded          map[string]harness.Plugin
	order           []string
	attachmentMutex sync.Mutex
	closed          bool
}

func New(harnessRuntime *harness.Harness) *Host {
	return &Host{harnessRuntime: harnessRuntime, loaded: map[string]harness.Plugin{}}
}

func (pluginHost *Host) Attach(plugin harness.Plugin) error {
	pluginHost.attachmentMutex.Lock()
	defer pluginHost.attachmentMutex.Unlock()
	pluginHost.mutex.RLock()
	_, exists := pluginHost.loaded[plugin.Name()]
	closed := pluginHost.closed
	pluginHost.mutex.RUnlock()
	if closed || exists || plugin.Name() == "" {
		return fmt.Errorf("plugin %q cannot attach: closed=%t registered=%t", plugin.Name(), closed, exists)
	}
	if operationError := plugin.Setup(pluginHost.harnessRuntime); operationError != nil {
		return operationError
	}
	pluginHost.mutex.Lock()
	defer pluginHost.mutex.Unlock()
	pluginHost.loaded[plugin.Name()] = plugin
	pluginHost.order = append(pluginHost.order, plugin.Name())
	return nil
}

func (pluginHost *Host) All() []harness.Plugin {
	pluginHost.mutex.RLock()
	defer pluginHost.mutex.RUnlock()
	list := make([]harness.Plugin, 0, len(pluginHost.loaded))
	for _, name := range pluginHost.order {
		list = append(list, pluginHost.loaded[name])
	}
	return list
}

func (pluginHost *Host) Get(name string) (harness.Plugin, bool) {
	pluginHost.mutex.RLock()
	defer pluginHost.mutex.RUnlock()
	plugin, found := pluginHost.loaded[name]
	return plugin, found
}

// Close prevents later attachment and joins every optional resource owner.
func (pluginHost *Host) Close(operationContext context.Context) error {
	pluginHost.attachmentMutex.Lock()
	defer pluginHost.attachmentMutex.Unlock()
	pluginHost.mutex.Lock()
	pluginHost.closed = true
	pluginHost.mutex.Unlock()
	var operationError error
	loaded := pluginHost.All()
	for index := len(loaded) - 1; index >= 0; index-- {
		if plugin, supported := loaded[index].(harness.ClosingPlugin); supported {
			operationError = errors.Join(operationError, plugin.Close(operationContext))
		}
	}
	return operationError
}
