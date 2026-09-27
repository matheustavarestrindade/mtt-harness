package postgres

import (
	"context"
	"errors"
)

type Store struct {
	dsn string
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	return nil, errors.New("postgres: not implemented")
}
