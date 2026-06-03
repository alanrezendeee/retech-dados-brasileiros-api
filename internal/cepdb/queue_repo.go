package cepdb

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type QueueItem struct {
	ID       int64
	CEP      string
	Attempts int16
}

type QueueStats struct {
	Pending    int64
	Processing int64
	Done       int64
	Failed     int64
}

func (db *DB) Enqueue(ctx context.Context, cep string, priority int16) error {
	cep = normalizeCEP(cep)
	if len(cep) != 8 {
		return nil
	}
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO cep_crawl_queue (cep, priority)
		VALUES ($1, $2)
		ON CONFLICT (cep) DO UPDATE SET
			priority = GREATEST(cep_crawl_queue.priority, EXCLUDED.priority)
		WHERE cep_crawl_queue.status IN ('pending', 'failed')
	`, cep, priority)
	return err
}

func (db *DB) EnqueueBatch(ctx context.Context, ceps []string, priority int16) error {
	if len(ceps) == 0 {
		return nil
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, cep := range ceps {
		cep = normalizeCEP(cep)
		if len(cep) != 8 || strings.ContainsAny(cep, "abcdefghijklmnopqrstuvwxyz") {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cep_crawl_queue (cep, priority)
			VALUES ($1, $2)
			ON CONFLICT (cep) DO NOTHING
		`, cep, priority); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Dequeue pega o próximo CEP da fila com SKIP LOCKED — sem contention entre workers
func (db *DB) Dequeue(ctx context.Context) (*QueueItem, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var item QueueItem
	err = tx.QueryRow(ctx, `
		SELECT id, cep, attempts FROM cep_crawl_queue
		WHERE status IN ('pending', 'failed')
		  AND next_attempt_at <= NOW()
		ORDER BY priority DESC, next_attempt_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`).Scan(&item.ID, &item.CEP, &item.Attempts)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, err
	}

	if _, err = tx.Exec(ctx, `
		UPDATE cep_crawl_queue
		SET status = 'processing', attempts = attempts + 1
		WHERE id = $1
	`, item.ID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &item, nil
}

func (db *DB) Ack(ctx context.Context, id int64) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE cep_crawl_queue SET status = 'done' WHERE id = $1
	`, id)
	return err
}

func (db *DB) Nack(ctx context.Context, id int64, errMsg string, nextAttempt time.Time) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE cep_crawl_queue
		SET status = 'failed', last_error = $2, next_attempt_at = $3
		WHERE id = $1
	`, id, errMsg, nextAttempt)
	return err
}

func (db *DB) QueueStats(ctx context.Context) (*QueueStats, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT status, COUNT(*) FROM cep_crawl_queue GROUP BY status
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := &QueueStats{}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		switch status {
		case "pending":
			stats.Pending = count
		case "processing":
			stats.Processing = count
		case "done":
			stats.Done = count
		case "failed":
			stats.Failed = count
		}
	}
	return stats, nil
}

func (db *DB) LogCrawl(ctx context.Context, cep, provider string, success bool, latencyMs int, errMsg string) {
	db.Pool.Exec(ctx, `
		INSERT INTO cep_crawl_logs (cep, provider, success, latency_ms, error)
		VALUES ($1, $2, $3, $4, NULLIF($5,''))
	`, cep, provider, success, int16(latencyMs), errMsg)
}
