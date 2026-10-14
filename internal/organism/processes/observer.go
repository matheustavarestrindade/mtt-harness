package processes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
)

func (processManager *Manager) observeProcessEvents(operationContext context.Context, session atom.Session, runningProcess harness.Process, record atom.ProcessRecord, delivery <-chan atom.NotifyPolicy) {
	defer processManager.observers.Done()
	defer func() {
		processManager.mutex.Lock()
		delete(processManager.instances, runningProcess.ID())
		processManager.mutex.Unlock()
	}()
	events := runningProcess.Events()
	processHandle := runningProcess.(*process.Handle)
	var ticks <-chan time.Time
	var ticker *time.Ticker
	var idleTimer *time.Timer
	var idle <-chan time.Time
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
		if idleTimer != nil {
			idleTimer.Stop()
		}
	}()
	configureNotifications := func() {
		if ticker != nil {
			ticker.Stop()
			ticker = nil
		}
		ticks = nil
		if record.Spec.Notify.Mode == atom.NotifyInterval && record.Spec.Notify.Interval > 0 {
			ticker = time.NewTicker(record.Spec.Notify.Interval)
			ticks = ticker.C
		}
	}
	configureNotifications()
	// Foreground inactivity is returned by the tool itself. An explicit background
	// launch can send one inactivity notice, in addition to its chosen exit policy.
	if delivery == nil && record.Spec.Notify.Mode != atom.NotifyNone && record.Spec.IdleTimeout > 0 {
		idleTimer = time.NewTimer(record.Spec.IdleTimeout)
		idle = idleTimer.C
	}
	for {
		select {
		case event, open := <-events:
			if !open {
				if operationError := processManager.completeProcessObservation(operationContext, session, runningProcess, record, delivery); operationError != nil {
					log.Printf("mtt: process completion: %v", operationError)
				}
				return
			}
			for _, watcher := range processManager.harnessRuntime.Watchers() {
				if watcher.Match(event) {
					watcher.OnMatch(operationContext, event)
				}
			}
			if event.Stream == atom.StreamExit {
				if operationError := processManager.completeProcessObservation(operationContext, session, runningProcess, record, delivery); operationError != nil {
					log.Printf("mtt: process completion: %v", operationError)
				}
				return
			}
			name, payload := atom.EventProcessOutput, any(map[string]any{"ProcessID": event.ProcessID, "Stream": event.Stream, "Offset": event.Offset, "Bytes": len(event.Data)})
			if event.Stream == atom.StreamStart {
				name, payload = atom.EventProcessStart, record
			}
			if operationError := processManager.sendProcessEvent(operationContext, session, name, event.At, payload); operationError != nil {
				log.Printf("mtt: process event: %v", operationError)
			}
		case policy := <-delivery:
			delivery = nil
			record.Spec.Notify = policy
			configureNotifications()
			if operationError := processManager.store.Processes().Save(operationContext, record); operationError != nil {
				log.Printf("mtt: process notification policy: %v", operationError)
			}
		case <-idle:
			activity := processHandle.Activity()
			if !activity.Running {
				idle = nil
				continue
			}
			remaining := record.Spec.IdleTimeout - activity.ObservedAt.Sub(activity.LastOutputAt)
			if remaining > 0 {
				idleTimer.Reset(remaining)
				continue
			}
			idle = nil
			if operationError := processManager.sendProcessNotification(operationContext, session, InactivityReport(runningProcess.ID(), activity, activity.Stdout, activity.Stderr)); operationError != nil {
				log.Printf("mtt: process inactivity: %v", operationError)
			}
		case <-ticks:
			if operationError := processManager.sendProcessNotification(operationContext, session, fmt.Sprintf("process %s: %s", runningProcess.ID(), processManager.recentProcessOutput(runningProcess))); operationError != nil {
				log.Printf("mtt: process notification: %v", operationError)
			}
		case <-operationContext.Done():
			return
		}
	}
}

func (processManager *Manager) completeProcessObservation(operationContext context.Context, session atom.Session, runningProcess harness.Process, record atom.ProcessRecord, delivery <-chan atom.NotifyPolicy) error {
	exit, _ := runningProcess.Wait()
	record.Status, record.Exit, record.EndedAt = "stopped", &exit, time.Now()
	if operationError := processManager.store.Processes().Save(operationContext, record); operationError != nil {
		return operationError
	}
	if operationError := processManager.sendProcessEvent(operationContext, session, atom.EventProcessExit, record.EndedAt, record); operationError != nil {
		return operationError
	}
	if delivery != nil {
		select {
		case policy := <-delivery:
			record.Spec.Notify = policy
		case <-processManager.stopNotifications:
			return nil
		case <-operationContext.Done():
			return operationContext.Err()
		}
		if operationError := processManager.store.Processes().Save(operationContext, record); operationError != nil {
			return operationError
		}
	}
	if record.Spec.Notify.Mode == atom.NotifyNone || (record.Spec.Notify.Mode == atom.NotifyError && exit.Code == 0 && exit.Error == "") {
		return nil
	}
	text := fmt.Sprintf("process %s stopped: %s", runningProcess.ID(), processManager.recentProcessOutput(runningProcess))
	if exit.Error != "" {
		text += "\n" + exit.Error
	}
	return processManager.sendProcessNotification(operationContext, session, text)
}

func (processManager *Manager) sendProcessEvent(operationContext context.Context, session atom.Session, name atom.EventName, at time.Time, payload any) error {
	if processManager.bus == nil {
		return nil
	}
	encoded, operationError := json.Marshal(payload)
	if operationError != nil {
		return operationError
	}
	return processManager.bus.Send(operationContext, atom.Event{InstanceID: session.InstanceID, SessionID: session.ID, Name: name, Time: at, Payload: encoded})
}
