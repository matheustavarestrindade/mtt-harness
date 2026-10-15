package processes

import (
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

// FenceInstanceUnlink rejects active processes and blocks new process starts.
// The caller retains the fence after commit, or releases it after a failed unlink.
// It performs no filesystem operations and never stops an existing process.
func (processManager *Manager) FenceInstanceUnlink(instanceID string) (func(bool), error) {
	processManager.mutex.Lock()
	defer processManager.mutex.Unlock()
	if processManager.closing || processManager.starting[instanceID] > 0 {
		return nil, store.ErrInstanceBusy
	}
	if blocked := processManager.instanceBlocks[instanceID]; blocked != nil {
		return nil, blocked
	}
	for identifier, owner := range processManager.instances {
		if owner != instanceID {
			continue
		}
		if running, found := processManager.supervisor.Get(identifier); found && running.(*process.Handle).Running() {
			return nil, store.ErrInstanceBusy
		}
	}
	processManager.instanceBlocks[instanceID] = store.ErrInstanceBusy
	return func(committed bool) {
		processManager.mutex.Lock()
		defer processManager.mutex.Unlock()
		if committed {
			processManager.instanceBlocks[instanceID] = store.ErrInstanceUnlinked
			return
		}
		delete(processManager.instanceBlocks, instanceID)
	}, nil
}
