package cepdb

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type DB struct {
	Pool *pgxpool.Pool
	log  zerolog.Logger
}

func NewDB(ctx context.Context, connURL string, log zerolog.Logger) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(connURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	db := &DB{Pool: pool, log: log.With().Str("component", "cepdb").Logger()}
	if err := db.autoMigrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	log.Info().Msg("cepdb_connected")
	return db, nil
}

func (db *DB) Close() {
	db.Pool.Close()
}
