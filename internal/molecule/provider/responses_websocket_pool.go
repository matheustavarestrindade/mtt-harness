package provider

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

const maximumResponsesConnections = 64
const responsesConnectionIdleLimit = 5 * time.Minute
const responsesConnectionAgeLimit = 55 * time.Minute

type responsesConnectionScope struct{ instanceID, sessionID string }

// The pool mutex owns admission and idle-entry eviction only. A cancellable
// per-entry gate owns the socket and continuation state for a complete response.
// Database, authentication, socket I/O, and stream consumers never hold it.
type responsesWebsocketPool struct {
	mutex           sync.Mutex
	context         context.Context
	cancel          context.CancelFunc
	closed          bool
	entries         map[responsesConnectionScope]*responsesWebsocketEntry
	uncached        int
	active          sync.WaitGroup
	maintenance     sync.Once
	maintenanceDone chan struct{}
	shutdown        sync.Once
}

type responsesWebsocketEntry struct {
	gate         chan struct{}
	references   int
	lastUsed     time.Time
	socket       *responsesWebsocketConnection
	identity     [sha256.Size]byte
	baseline     *responsesContinuation
	httpFallback bool
}

type responsesWebsocketLease struct {
	pool         *responsesWebsocketPool
	entry        *responsesWebsocketEntry
	context      context.Context
	cancel       context.CancelFunc
	stopShutdown func() bool
	uncached     bool
	acquired     bool
	once         sync.Once
}

func newResponsesWebsocketPool() *responsesWebsocketPool {
	operationContext, cancel := context.WithCancel(context.Background())
	return &responsesWebsocketPool{context: operationContext, cancel: cancel, entries: map[responsesConnectionScope]*responsesWebsocketEntry{}, maintenanceDone: make(chan struct{})}
}

func (pool *responsesWebsocketPool) acquire(operationContext context.Context, scope responsesConnectionScope) (*responsesWebsocketLease, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	pool.mutex.Lock()
	if pool.closed {
		pool.mutex.Unlock()
		return nil, fmt.Errorf("provider WebSocket transport is closed")
	}
	pool.maintenance.Do(func() { go pool.closeIdleConnections() })
	entry := pool.entries[scope]
	uncached := scope.sessionID == ""
	var retired *responsesWebsocketEntry
	if entry == nil || uncached {
		if len(pool.entries)+pool.uncached >= maximumResponsesConnections {
			var oldestScope responsesConnectionScope
			for candidateScope, candidate := range pool.entries {
				if candidate.references == 0 && (retired == nil || candidate.lastUsed.Before(retired.lastUsed)) {
					retired = candidate
					oldestScope = candidateScope
				}
			}
			if retired == nil {
				pool.mutex.Unlock()
				return nil, fmt.Errorf("provider WebSocket connection limit reached (%d)", maximumResponsesConnections)
			}
			delete(pool.entries, oldestScope)
		}
		entry = &responsesWebsocketEntry{gate: make(chan struct{}, 1), lastUsed: time.Now()}
		entry.gate <- struct{}{}
		if uncached {
			pool.uncached++
		} else {
			pool.entries[scope] = entry
		}
	}
	entry.references++
	pool.active.Add(1)
	pool.mutex.Unlock()
	if retired != nil && retired.socket != nil {
		retired.socket.close()
	}
	requestContext, cancel := context.WithCancel(operationContext)
	lease := &responsesWebsocketLease{pool: pool, entry: entry, context: requestContext, cancel: cancel, uncached: uncached}
	lease.stopShutdown = context.AfterFunc(pool.context, cancel)
	select {
	case <-requestContext.Done():
		lease.release()
		return nil, requestContext.Err()
	case <-entry.gate:
		lease.acquired = true
	}
	if operationError := requestContext.Err(); operationError != nil {
		lease.release()
		return nil, operationError
	}
	return lease, nil
}

func (lease *responsesWebsocketLease) retireConnection() {
	if lease.entry.socket != nil {
		lease.entry.socket.close()
		lease.entry.socket = nil
	}
	lease.entry.baseline = nil
}

func (lease *responsesWebsocketLease) release() {
	lease.once.Do(func() {
		if lease.acquired && lease.uncached {
			lease.retireConnection()
		}
		lease.stopShutdown()
		lease.cancel()
		lease.pool.mutex.Lock()
		lease.entry.references--
		lease.entry.lastUsed = time.Now()
		if lease.uncached {
			lease.pool.uncached--
		}
		lease.pool.mutex.Unlock()
		if lease.acquired {
			lease.entry.gate <- struct{}{}
		}
		lease.pool.active.Done()
	})
}

func (pool *responsesWebsocketPool) closeIdleConnections() {
	defer close(pool.maintenanceDone)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-pool.context.Done():
			return
		case now := <-ticker.C:
			pool.evictIdleConnections(now)
		}
	}
}

func (pool *responsesWebsocketPool) evictIdleConnections(now time.Time) {
	var retired []*responsesWebsocketConnection
	pool.mutex.Lock()
	for scope, entry := range pool.entries {
		if entry.references == 0 && now.Sub(entry.lastUsed) >= responsesConnectionIdleLimit {
			delete(pool.entries, scope)
			if entry.socket != nil {
				retired = append(retired, entry.socket)
			}
		}
	}
	pool.mutex.Unlock()
	for _, socket := range retired {
		socket.close()
	}
}

func (pool *responsesWebsocketPool) Close() {
	pool.shutdown.Do(pool.closeConnections)
}

func (pool *responsesWebsocketPool) closeConnections() {
	pool.mutex.Lock()
	pool.closed = true
	pool.maintenance.Do(func() { close(pool.maintenanceDone) })
	pool.cancel()
	pool.mutex.Unlock()
	pool.active.Wait()
	<-pool.maintenanceDone
	pool.mutex.Lock()
	entries := pool.entries
	pool.entries = map[responsesConnectionScope]*responsesWebsocketEntry{}
	pool.mutex.Unlock()
	for _, entry := range entries {
		if entry.socket != nil {
			entry.socket.close()
		}
	}
}
