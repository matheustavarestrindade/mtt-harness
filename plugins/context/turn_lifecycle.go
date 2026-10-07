package contextplugin

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (plugin *Plugin) EndTurn(operationContext context.Context, session atom.Session, status string) error {
	if status == "completed" || plugin.database == nil {
		return nil
	}
	var cancelled []string
	operationError := plugin.database.Transact(operationContext, session.InstanceID, func(database transaction) error {
		data, operationError := database.Get("view", string(session.ID))
		if operationError != nil || len(data) == 0 {
			return operationError
		}
		view, operationError := loadView(database, session.ID)
		if operationError != nil {
			return operationError
		}
		if view.Deleted || view.Mutation != nil {
			return nil
		}
		view.Epoch++
		view.WrapRequested = false
		jobs, operationError := readTransactionValues[memoryJob](database, "job")
		if operationError != nil {
			return operationError
		}
		for _, job := range jobs {
			if job.SessionID != session.ID || job.Kind == "archive" || (job.Status != "pending" && job.Status != "running" && job.Status != "ready") {
				continue
			}
			job.Status = "cancelled"
			job.Error = "originating turn " + status
			cancelled = append(cancelled, job.ID)
			if operationError := database.Put("job", job.ID, job); operationError != nil {
				return operationError
			}
		}
		return database.Put("view", string(session.ID), view)
	})
	plugin.mutex.Lock()
	if scope := plugin.scopes[session.InstanceID]; scope != nil {
		for _, identifier := range cancelled {
			if cancel := scope.jobs[identifier]; cancel != nil {
				cancel()
			}
		}
	}
	plugin.mutex.Unlock()
	return operationError
}
