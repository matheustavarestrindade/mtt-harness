package processes

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (processManager *Manager) observe(operationContext context.Context, session atom.Session, runningProcess harness.Process, record atom.ProcessRecord) {
	defer processManager.observers.Done()
	defer func() {
		processManager.mutex.Lock()
		delete(processManager.instances, runningProcess.ID())
		processManager.mutex.Unlock()
	}()
	events := runningProcess.Events()
	var ticks <-chan time.Time
	if record.Spec.Notify.Mode == atom.NotifyInterval {
		ticker := time.NewTicker(record.Spec.Notify.Interval)
		defer ticker.Stop()
		ticks = ticker.C
	}
	for {
		select {
		case event, open := <-events:
			if !open {
				if operationError := processManager.finish(operationContext, session, runningProcess, record, true); operationError != nil {
					log.Printf("mtt: process completion: %v", operationError)
				}
				return
			}
			for _, watcher := range processManager.harnessRuntime.Watchers() {
				if watcher.Match(event) {
					watcher.OnMatch(operationContext, event)
				}
			}
			if processManager.bus != nil {
				name := atom.EventProcessOutput
				if event.Stream == atom.StreamStart {
					name = atom.EventProcessStart
				}
				if event.Stream == atom.StreamExit {
					name = atom.EventProcessExit
				}
				if operationError := processManager.bus.Send(operationContext, atom.Event{InstanceID: session.InstanceID, SessionID: session.ID, Name: name, Time: event.At}); operationError != nil {
					log.Printf("mtt: process event: %v", operationError)
				}
			}
		case <-ticks:
			if operationError := processManager.send(operationContext, session, fmt.Sprintf("process %s: %s", runningProcess.ID(), processManager.tail(runningProcess))); operationError != nil {
				log.Printf("mtt: process notification: %v", operationError)
			}
		case <-operationContext.Done():
			return
		}
	}
}

func (processManager *Manager) finish(operationContext context.Context, session atom.Session, runningProcess harness.Process, record atom.ProcessRecord, notify bool) error {
	exit, _ := runningProcess.Wait()
	record.Status, record.Exit, record.EndedAt = "stopped", &exit, time.Now()
	if operationError := processManager.store.Processes().Save(operationContext, record); operationError != nil {
		return operationError
	}
	if !notify || (record.Spec.Notify.Mode == atom.NotifyError && exit.Code == 0 && exit.Error == "") {
		return nil
	}
	text := fmt.Sprintf("process %s stopped: %s", runningProcess.ID(), processManager.tail(runningProcess))
	if exit.Error != "" {
		text += "\n" + exit.Error
	}
	return processManager.send(operationContext, session, text)
}
