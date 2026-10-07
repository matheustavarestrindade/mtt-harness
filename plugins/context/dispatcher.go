package contextplugin

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

const maximumActiveWorkers = 32

func (plugin *Plugin) runDispatcher() {
	defer close(plugin.dispatchDone)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if plugin.database != nil {
			if operationError := plugin.dispatchJobs(plugin.operationContext); operationError != nil && !errors.Is(operationError, context.Canceled) {
				plugin.mutex.Lock()
				plugin.lastError = operationError.Error()
				plugin.mutex.Unlock()
			}
		}
		select {
		case <-plugin.operationContext.Done():
			return
		case <-ticker.C:
		case <-plugin.wake:
		}
	}
}

func (plugin *Plugin) dispatchJobs(operationContext context.Context) error {
	workspaces, operationError := plugin.database.PendingWorkspaces(operationContext, 4096)
	if operationError != nil {
		return operationError
	}
	for _, workspaceID := range workspaces {
		if operationError := operationContext.Err(); operationError != nil {
			return operationError
		}
		if operationError := plugin.recoverHistoryChanges(operationContext, workspaceID); operationError != nil {
			return operationError
		}
		plugin.configurationMutex.Lock()
		configuration, _, configurationError := plugin.loadConfiguration(operationContext, workspaceID)
		if configurationError != nil {
			plugin.configurationMutex.Unlock()
			return configurationError
		}
		_, workspaceError := plugin.services.Workspaces.Workspace(operationContext, workspaceID)
		plugin.mutex.Lock()
		scope := plugin.scopeLocked(workspaceID)
		if !scope.initialized || !configurationEqual(scope.configuration, configuration) {
			if scope.requests == 0 {
				plugin.applyConfigurationLocked(scope, configuration)
			} else {
				desired := configuration
				scope.pending = &desired
			}
		}
		configuration = scope.configuration
		active := 0
		for _, candidate := range plugin.scopes {
			active += len(candidate.jobs)
		}
		if workspaceError != nil {
			for _, cancel := range scope.jobs {
				cancel()
			}
		}
		available := !plugin.closed && workspaceError == nil && configuration.Enabled && len(scope.jobs) < configuration.WorkerCount && active < maximumActiveWorkers
		plugin.mutex.Unlock()
		plugin.configurationMutex.Unlock()
		if !available {
			continue
		}
		job, found, operationError := plugin.claimJob(operationContext, workspaceID, configuration)
		if operationError != nil {
			return operationError
		}
		if !found {
			if operationError := plugin.scheduleHistorian(operationContext, workspaceID, configuration); operationError != nil {
				return operationError
			}
			continue
		}
		workerContext, cancel := context.WithTimeout(operationContext, time.Duration(configuration.JobTimeoutMilliseconds)*time.Millisecond)
		plugin.mutex.Lock()
		if plugin.closed || !scope.configuration.Enabled {
			plugin.mutex.Unlock()
			cancel()
			if operationError := plugin.finishJob(context.WithoutCancel(operationContext), workspaceID, job, configuration, context.Canceled); operationError != nil {
				return operationError
			}
			continue
		}
		scope.jobs[job.ID] = cancel
		plugin.workers.Add(1)
		plugin.mutex.Unlock()
		go func(workspaceID string, job memoryJob) {
			defer plugin.workers.Done()
			defer cancel()
			operationError := plugin.executeJob(workerContext, workspaceID, job, configuration)
			completionContext, stopCompletion := context.WithTimeout(context.WithoutCancel(workerContext), 10*time.Second)
			finishError := plugin.finishJob(completionContext, workspaceID, job, configuration, operationError)
			stopCompletion()
			plugin.mutex.Lock()
			delete(scope.jobs, job.ID)
			if finishError != nil {
				plugin.lastError = finishError.Error()
			}
			plugin.mutex.Unlock()
			plugin.signalWork()
		}(workspaceID, job)
	}
	return nil
}

func (plugin *Plugin) claimJob(operationContext context.Context, workspaceID string, configuration configuration) (memoryJob, bool, error) {
	var selected memoryJob
	operationError := plugin.database.Transact(operationContext, workspaceID, func(database transaction) error {
		jobs, operationError := readTransactionValues[memoryJob](database, "job")
		if operationError != nil {
			return operationError
		}
		sort.Slice(jobs, func(first, second int) bool {
			if jobs[first].Priority != jobs[second].Priority {
				return jobs[first].Priority < jobs[second].Priority
			}
			return jobs[first].CreatedAt.Before(jobs[second].CreatedAt)
		})
		now := time.Now().UTC()
		for _, job := range jobs {
			if job.Status != "pending" && !(job.Status == "running" && job.LeaseUntil.Before(now)) {
				continue
			}
			if job.NextAttempt.After(now) {
				continue
			}
			if job.SessionID != "" {
				view, operationError := loadView(database, job.SessionID)
				if operationError != nil {
					return operationError
				}
				if view.Epoch != job.Epoch || view.Mutation != nil || (view.Deleted && job.Kind != "archive") {
					job.Status = "cancelled"
					job.Error = "source history changed"
					if operationError := database.Put("job", job.ID, job); operationError != nil {
						return operationError
					}
					continue
				}
			}
			job.Status = "running"
			job.Attempts++
			job.Lease = newIdentifier()
			job.LeaseUntil = now.Add(time.Duration(configuration.JobTimeoutMilliseconds)*time.Millisecond + 30*time.Second)
			selected = job
			return database.Put("job", job.ID, job)
		}
		return nil
	})
	return selected, selected.ID != "", operationError
}

func validateJobLease(database transaction, job memoryJob) (memoryJob, error) {
	stored, operationError := readTransactionValue[memoryJob](database, "job", job.ID)
	if operationError != nil {
		return stored, operationError
	}
	if stored.Status != "running" || stored.Lease != job.Lease || stored.LeaseUntil.Before(time.Now()) {
		return stored, fmt.Errorf("memory job lease is no longer current")
	}
	if job.SessionID != "" {
		view, operationError := loadView(database, job.SessionID)
		if operationError != nil {
			return stored, operationError
		}
		if view.Epoch != job.Epoch || view.Mutation != nil || (view.Deleted && job.Kind != "archive") {
			return stored, fmt.Errorf("memory job source history changed")
		}
	}
	return stored, nil
}

func (plugin *Plugin) finishJob(operationContext context.Context, workspaceID string, job memoryJob, configuration configuration, jobError error) error {
	return plugin.database.Transact(operationContext, workspaceID, func(database transaction) error {
		stored, operationError := readTransactionValue[memoryJob](database, "job", job.ID)
		if operationError != nil {
			return operationError
		}
		if stored.Lease != job.Lease || stored.Status != "running" {
			return nil
		}
		if jobError == nil {
			return fmt.Errorf("memory worker ended without publishing its result")
		}
		stored.Error = jobError.Error()
		stored.Lease = ""
		stored.LeaseUntil = time.Time{}
		if errors.Is(jobError, context.Canceled) || (errors.Is(jobError, context.DeadlineExceeded) && stored.Progress > job.Progress) {
			stored.Status = "pending"
			stored.NextAttempt = time.Now().Add(time.Second)
		} else {
			stored.Failures++
			stored.Status = "failed"
			if stored.Failures <= configuration.RetryLimit {
				stored.Status = "pending"
				stored.NextAttempt = time.Now().Add(time.Duration(1<<min(stored.Failures, 5)) * time.Second)
			}
		}
		return database.Put("job", job.ID, stored)
	})
}
