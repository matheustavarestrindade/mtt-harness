package loop

import (
	"context"
	"errors"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

var ErrInstanceResume = errors.New("workspace could not start")
var ErrInstanceLifecycleUnavailable = errors.New("workspace lifecycle is unavailable")

// StopStoredInstance serializes persistence, runtime removal, and queue/process
// shutdown with resume and unlink operations for the same workspace.
func (messageQueue *Queue) StopStoredInstance(operationContext context.Context, instanceID string) error {
	_, operationError := messageQueue.requestDirectoryCommand(operationContext, queueCommand{kind: stopStoredInstance, instanceID: instanceID})
	return operationError
}

// ResumeStoredInstance restores a linked workspace under the directory's gate.
func (messageQueue *Queue) ResumeStoredInstance(operationContext context.Context, instanceID string) (atom.InstanceSpec, error) {
	response, operationError := messageQueue.requestDirectoryCommand(operationContext, queueCommand{kind: resumeStoredInstance, instanceID: instanceID})
	return response.instance, operationError
}

// UnlinkInstance retires an idle workspace reference. Files and saved workspace
// data are not deleted. An accepted operation outlives the caller's wait and is
// joined by Queue.Close before storage is released.
func (messageQueue *Queue) UnlinkInstance(operationContext context.Context, instanceID string) error {
	_, operationError := messageQueue.requestDirectoryCommand(operationContext, queueCommand{kind: unlinkInstance, instanceID: instanceID})
	return operationError
}

func (messageQueue *Queue) controlStoredInstance(command queueCommand, coordinators []*sessionCoordinator) {
	operationContext := context.WithoutCancel(command.operationContext)
	completed := directoryCompletion{command: command}
	completed.operationError = func() error {
		configuration := messageQueue.loop.configuration
		if configuration.Instances == nil {
			return ErrInstanceLifecycleUnavailable
		}
		instanceSpec, operationError := configuration.Store.Instances().Get(operationContext, command.instanceID)
		if operationError != nil {
			return operationError
		}
		switch command.kind {
		case unlinkInstance:
			return messageQueue.unlinkIdleInstance(operationContext, command.instanceID, coordinators, &completed)
		case stopStoredInstance:
			instanceSpec.Stopped = true
			if operationError = configuration.Store.Instances().Save(operationContext, instanceSpec); operationError != nil {
				return operationError
			}
			completed.instanceStopped = true
			if configuration.Instances.IsRunning(instanceSpec.ID) {
				if operationError = configuration.Instances.Stop(operationContext, instanceSpec.ID); operationError != nil {
					return operationError
				}
			}
			operationError = messageQueue.controlInstanceCoordinators(operationContext, instanceSpec.ID, stopSession, coordinators)
			if configuration.Processes != nil {
				_, processError := configuration.Processes.StopInstance(operationContext, instanceSpec.ID)
				operationError = errors.Join(operationError, processError)
			}
			return operationError
		case resumeStoredInstance:
			instanceSpec.Stopped = false
			instance, operationError := configuration.Instances.Start(operationContext, instanceSpec)
			if operationError != nil {
				return fmt.Errorf("%w: %w", ErrInstanceResume, operationError)
			}
			if operationError = configuration.Store.Instances().Save(operationContext, instance.Spec()); operationError != nil {
				return errors.Join(operationError, configuration.Instances.Stop(operationContext, instanceSpec.ID))
			}
			if operationError = messageQueue.controlInstanceCoordinators(operationContext, instanceSpec.ID, resumeSession, coordinators); operationError != nil {
				return operationError
			}
			completed.instance = instance.Spec()
			return nil
		}
		panic("unknown stored workspace operation")
	}()
	messageQueue.completed <- completed
}

func (messageQueue *Queue) unlinkIdleInstance(operationContext context.Context, instanceID string, coordinators []*sessionCoordinator, completed *directoryCompletion) (operationError error) {
	var frozen []*sessionCoordinator
	defer func() {
		for _, coordinator := range frozen {
			_, releaseError := coordinator.requestSessionCommand(operationContext, sessionCommand{kind: releaseSessionDeletion})
			if releaseError != nil && !errors.Is(releaseError, ErrQueueClosed) {
				operationError = errors.Join(operationError, releaseError)
			}
		}
	}()
	for _, coordinator := range coordinators {
		if _, operationError = coordinator.requestSessionCommand(operationContext, sessionCommand{kind: prepareSessionDeletion}); operationError != nil {
			if errors.Is(operationError, ErrQueueClosed) {
				<-coordinator.done
				continue
			}
			if errors.Is(operationError, store.ErrConversationBusy) {
				return store.ErrInstanceBusy
			}
			return operationError
		}
		frozen = append(frozen, coordinator)
	}
	finishRuns, operationError := messageQueue.loop.fenceInstanceUnlink(instanceID)
	if operationError != nil {
		return operationError
	}
	defer func() { finishRuns(completed.unlinked) }()
	configuration := messageQueue.loop.configuration
	if configuration.Processes != nil {
		finishProcesses, processError := configuration.Processes.FenceInstanceUnlink(instanceID)
		if processError != nil {
			return processError
		}
		defer func() { finishProcesses(completed.unlinked) }()
	}
	if operationError = configuration.Store.Instances().Unlink(operationContext, instanceID); operationError != nil {
		return operationError
	}
	completed.unlinked = true
	if configuration.Instances.IsRunning(instanceID) {
		if operationError = configuration.Instances.Stop(operationContext, instanceID); operationError != nil {
			return operationError
		}
	}
	for _, coordinator := range frozen {
		_, closeError := coordinator.requestSessionCommand(operationContext, sessionCommand{kind: closeSession})
		if closeError != nil && !errors.Is(closeError, ErrQueueClosed) {
			return closeError
		}
		<-coordinator.done
	}
	return nil
}

func (agentLoop *Loop) fenceInstanceUnlink(instanceID string) (func(bool), error) {
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	if agentLoop.instanceBlocks[instanceID] != nil {
		return nil, store.ErrInstanceBusy
	}
	for _, active := range agentLoop.activeRuns {
		if active.session.InstanceID == instanceID {
			return nil, store.ErrInstanceBusy
		}
	}
	agentLoop.instanceBlocks[instanceID] = store.ErrInstanceBusy
	return func(committed bool) {
		agentLoop.mutex.Lock()
		defer agentLoop.mutex.Unlock()
		if committed {
			agentLoop.instanceBlocks[instanceID] = store.ErrInstanceUnlinked
			return
		}
		delete(agentLoop.instanceBlocks, instanceID)
	}, nil
}
