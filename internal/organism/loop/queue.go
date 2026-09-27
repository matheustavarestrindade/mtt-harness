package loop

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Queue struct {
	loop    *Loop
	mu      sync.Mutex
	workers map[atom.SessionID]*sessionWorker
}

type sessionWorker struct {
	queue   []atom.Message
	running bool
	cancel  context.CancelFunc
}

func NewQueue(l *Loop) *Queue {
	return &Queue{loop: l, workers: map[atom.SessionID]*sessionWorker{}}
}

func (q *Queue) Submit(ctx context.Context, session atom.Session, content string) (atom.Message, int, error) {
	message := atom.Message{
		ID:        newID(),
		SessionID: session.ID,
		Role:      atom.RoleUser,
		Content:   []atom.Content{{Type: atom.Text, Text: content}},
		CreatedAt: time.Now(),
	}
	q.mu.Lock()
	worker, ok := q.workers[session.ID]
	if !ok {
		worker = &sessionWorker{}
		q.workers[session.ID] = worker
	}
	worker.queue = append(worker.queue, message)
	position := len(worker.queue)
	start := !worker.running
	if start {
		worker.running = true
	}
	q.mu.Unlock()
	if start {
		go q.work(session)
	}
	return message, position, nil
}

func (q *Queue) work(session atom.Session) {
	for {
		q.mu.Lock()
		worker := q.workers[session.ID]
		if worker == nil || len(worker.queue) == 0 {
			if worker != nil {
				worker.running = false
			}
			q.mu.Unlock()
			return
		}
		message := worker.queue[0]
		worker.queue = worker.queue[1:]
		ctx, cancel := context.WithCancel(context.Background())
		worker.cancel = cancel
		q.mu.Unlock()
		err := q.loop.cfg.Store.Sessions().Append(ctx, message)
		if err == nil {
			err = q.loop.Run(ctx, session)
		}
		cancel()
		q.mu.Lock()
		worker.cancel = nil
		q.mu.Unlock()
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("mtt: the run of the session %s: %v", session.ID, err)
		}
	}
}

func (q *Queue) Cancel(session atom.SessionID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	worker := q.workers[session]
	if worker == nil || worker.cancel == nil {
		return false
	}
	worker.cancel()
	return true
}

func (q *Queue) Status(session atom.SessionID) (bool, int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	worker := q.workers[session]
	if worker == nil {
		return false, 0
	}
	return worker.cancel != nil, len(worker.queue)
}

func (q *Queue) Clear(session atom.SessionID) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	worker := q.workers[session]
	if worker == nil {
		return 0
	}
	count := len(worker.queue)
	worker.queue = nil
	if worker.cancel != nil {
		worker.cancel()
	}
	return count
}
