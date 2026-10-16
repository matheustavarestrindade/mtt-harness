package store

import "errors"

var ErrQueueFull = errors.New("session message queue is full")
var ErrQueuedMessageNotFound = errors.New("message is not pending in the session")
