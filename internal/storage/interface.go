package storage

import "context"

type Repository interface {
	Ping(ctx context.Context) error
	Close()
}

var _ Repository = (*Storage)(nil)
