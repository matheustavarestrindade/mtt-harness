package store

import "errors"

var ErrQueueFull = errors.New("session message queue is full")
