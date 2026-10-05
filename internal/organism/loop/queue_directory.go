package loop

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

// Only runDirectory accesses this map and lifecycle state. It delegates waits,
// persistence and session commands to workers instead of waiting on an owner.
type queueDirectory struct {
	queue         *Queue
	sessions      map[atom.SessionID]*sessionCoordinator
	stopped       map[string]bool
	controlling   map[string]bool
	deleted       map[atom.SessionID]bool
	deleting      map[string]bool
	blocked       map[atom.SessionID]bool
	restoring     bool
	closing       bool
	operations    int
	restoreCancel context.CancelFunc
	closeWaiters  []queueCommand
}

func (messageQueue *Queue) runDirectory() {
	directory := &queueDirectory{queue: messageQueue, sessions: map[atom.SessionID]*sessionCoordinator{}, stopped: map[string]bool{}, controlling: map[string]bool{}, deleted: map[atom.SessionID]bool{}, deleting: map[string]bool{}, blocked: map[atom.SessionID]bool{}}
	defer close(messageQueue.done)
	for {
		select {
		case command := <-messageQueue.commands:
			directory.handleDirectoryCommand(command)
		case completed := <-messageQueue.completed:
			directory.handleCompletion(completed)
		}
		if directory.closing && directory.operations == 0 {
			for _, command := range directory.closeWaiters {
				command.respond(queueReply{})
			}
			return
		}
	}
}

func (directory *queueDirectory) handleDirectoryCommand(command queueCommand) {
	if operationError := command.operationContext.Err(); operationError != nil && command.kind != closeQueue {
		command.respond(queueReply{operationError: operationError})
		return
	}
	if command.kind == closeQueue {
		directory.beginClose(command)
		return
	}
	// Accepted deletion workers still need their admission fence during shutdown.
	if directory.closing && command.kind != fenceConversationDeletion {
		command.respond(queueReply{operationError: ErrQueueClosed})
		return
	}
	if command.kind == findSession {
		if directory.deleted[command.session.ID] {
			command.respond(queueReply{operationError: store.ErrSessionDeleted})
			return
		}
		if command.create && directory.restoring {
			command.respond(queueReply{operationError: ErrSessionBusy})
			return
		}
		if command.create && (directory.blocked[command.session.ID] || directory.blocked[command.session.Parent] ||
			(directory.controlling[command.session.InstanceID] && !directory.deleting[command.session.InstanceID])) {
			command.respond(queueReply{operationError: ErrSessionBusy})
			return
		}
		coordinator := directory.sessions[command.session.ID]
		if coordinator == nil && command.create {
			coordinator = newSessionCoordinator(directory.queue.loop, command.session, nil, directory.stopped[command.session.InstanceID])
			directory.sessions[command.session.ID] = coordinator
		}
		command.respond(queueReply{coordinator: coordinator})
		return
	}
	if directory.restoring {
		command.respond(queueReply{operationError: ErrSessionBusy})
		return
	}
	switch command.kind {
	case fenceConversationDeletion:
		if !directory.deleting[command.instanceID] {
			command.respond(queueReply{operationError: ErrSessionBusy})
			return
		}
		var coordinators []*sessionCoordinator
		for identifier := range command.protected {
			directory.blocked[identifier] = true
			if coordinator := directory.sessions[identifier]; coordinator != nil {
				coordinators = append(coordinators, coordinator)
			}
		}
		command.respond(queueReply{coordinators: coordinators})
	case deleteConversation:
		if directory.controlling[command.instanceID] {
			command.respond(queueReply{operationError: ErrSessionBusy})
			return
		}
		directory.controlling[command.instanceID] = true
		directory.deleting[command.instanceID] = true
		directory.operations++
		go directory.queue.deleteStoredConversation(command)
	case restoreQueue:
		if len(directory.sessions) > 0 {
			command.respond(queueReply{operationError: ErrSessionBusy})
			return
		}
		directory.restoring = true
		operationContext, cancel := context.WithCancel(command.operationContext)
		directory.restoreCancel = cancel
		directory.operations++
		go directory.queue.restoreSessions(operationContext, command)
	case stopInstance, resumeInstance:
		if directory.controlling[command.instanceID] {
			command.respond(queueReply{operationError: ErrSessionBusy})
			return
		}
		directory.controlling[command.instanceID] = true
		if command.kind == stopInstance {
			directory.stopped[command.instanceID] = true
		}
		coordinators := directory.coordinatorsForInstance(command.instanceID)
		directory.operations++
		go directory.queue.controlSessions(command, coordinators)
	default:
		panic("unknown queue command")
	}
}

func (directory *queueDirectory) beginClose(command queueCommand) {
	directory.closeWaiters = append(directory.closeWaiters, command)
	if directory.closing {
		return
	}
	directory.closing = true
	if directory.restoreCancel != nil {
		directory.restoreCancel()
	}
	directory.operations++
	go directory.queue.controlSessions(queueCommand{kind: closeQueue, operationContext: context.Background()}, directory.coordinatorsForInstance(""))
}

func (directory *queueDirectory) coordinatorsForInstance(instanceID string) []*sessionCoordinator {
	var coordinators []*sessionCoordinator
	for _, coordinator := range directory.sessions {
		if instanceID == "" || coordinator.session.InstanceID == instanceID {
			coordinators = append(coordinators, coordinator)
		}
	}
	return coordinators
}

func (directory *queueDirectory) handleCompletion(completed directoryCompletion) {
	directory.operations--
	command := completed.command
	if command.kind == closeQueue {
		return
	}
	if command.kind == restoreQueue {
		directory.restoring = false
		directory.restoreCancel()
		directory.restoreCancel = nil
		if !directory.closing && completed.operationError == nil {
			for _, restored := range completed.restored {
				stopped := directory.queue.loop.configuration.Instances != nil && !directory.queue.loop.configuration.Instances.IsRunning(restored.session.InstanceID)
				directory.sessions[restored.session.ID] = newSessionCoordinator(directory.queue.loop, restored.session, restored.pending, stopped)
			}
		}
	} else {
		delete(directory.controlling, command.instanceID)
		delete(directory.deleting, command.instanceID)
		for identifier := range completed.protected {
			delete(directory.blocked, identifier)
		}
		for _, identifier := range completed.deleted {
			directory.deleted[identifier] = true
			delete(directory.sessions, identifier)
		}
		if command.kind == resumeInstance && completed.operationError == nil {
			delete(directory.stopped, command.instanceID)
		}
	}
	if directory.closing && command.kind != deleteConversation {
		command.respond(queueReply{operationError: ErrQueueClosed})
		return
	}
	command.respond(queueReply{deleted: completed.deleted, operationError: completed.operationError})
}
