package storage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Storage struct {
	PG    *pgxpool.Pool
	Redis *redis.Client
}

func New(ctx context.Context, pgDSN, redisAddr, redisPass string) (*Storage, error) {
	pgCfg, err := pgxpool.ParseConfig(pgDSN)
	if err != nil {
		return nil, err
	}
	pgCfg.MaxConns = 10
	pgCfg.MinConns = 2
	pgCfg.MaxConnLifetime = 30 * time.Minute

	pg, err := pgxpool.NewWithConfig(ctx, pgCfg)
	if err != nil {
		return nil, err
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPass,
		DB:       0,
	})

	return &Storage{PG: pg, Redis: rdb}, nil
}

func (s *Storage) Close() {
	if s == nil {
		return
	}
	if s.PG != nil {
		s.PG.Close()
	}
	if s.Redis != nil {
		_ = s.Redis.Close()
	}
}

func (s *Storage) Ping(ctx context.Context) error {
	if err := s.PG.Ping(ctx); err != nil {
		return err
	}

	if err := s.Redis.Ping(ctx).Err(); err != nil {
		return err
	}
	return nil
}
