package loop

import "github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"

// Admission changes stay with the coordinator. Deletion workers own storage I/O.
func (coordinator *sessionCoordinator) handleDeletion(command sessionCommand) {
	if command.kind == releaseSessionDeletion {
		if coordinator.mode == sessionDeleting {
			coordinator.mode = coordinator.beforeDeletion
		}
		command.respond(sessionReply{})
		return
	}
	if coordinator.mode == sessionClosing {
		command.respond(sessionReply{operationError: ErrQueueClosed})
		return
	}
	if coordinator.mode == sessionReverting || coordinator.mode == sessionDeleting ||
		coordinator.active != nil || coordinator.working != nil || coordinator.revert != nil ||
		len(coordinator.pending) != 0 || len(coordinator.mutations) != 0 || len(coordinator.waiters) != 0 {
		command.respond(sessionReply{operationError: store.ErrConversationBusy})
		return
	}
	coordinator.beforeDeletion = coordinator.mode
	coordinator.mode = sessionDeleting
	command.respond(sessionReply{})
}
