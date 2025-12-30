package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	pool *pgxpool.Pool
}

func NewDB(ctx context.Context, dsn string) (*DB, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	s := &DB{pool: pool}
	if err := s.init(ctx); err != nil {
		return nil, fmt.Errorf("failed to init database: %w", err)
	}

	return s, nil
}

func (s *DB) Close() {
	s.pool.Close()
}

func (s *DB) init(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS rpc_cache (
			key TEXT PRIMARY KEY,
			method TEXT NOT NULL,
			response BYTEA NOT NULL,
			result_length BIGINT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			last_accessed_at TIMESTAMP NOT NULL
		)`,
	}

	for _, query := range queries {
		if _, err := s.pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to execute query %s: %w", query, err)
		}
	}

	return nil
}

func (s *DB) GetCachedRPCResult(ctx context.Context, key string) ([]byte, error) {
	var response []byte
	// We update last_accessed_at on read
	err := s.pool.QueryRow(ctx, `
		UPDATE rpc_cache 
		SET last_accessed_at = NOW() 
		WHERE key = $1 
		RETURNING response
	`, key).Scan(&response)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get cached rpc result: %w", err)
	}

	return response, nil
}

func (s *DB) SetCachedRPCResult(ctx context.Context, key string, method string, response []byte) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO rpc_cache (key, method, response, result_length, created_at, last_accessed_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (key) DO UPDATE
		SET response = $3, result_length = $4, last_accessed_at = NOW()
	`, key, method, response, len(response))

	if err != nil {
		return fmt.Errorf("failed to set cached rpc result: %w", err)
	}
	return nil
}

func (s *DB) GetCacheSize(ctx context.Context) (int64, error) {
	var size int64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(result_length + 64), 0) FROM rpc_cache
	`).Scan(&size)
	if err != nil {
		return 0, fmt.Errorf("failed to get cache size: %w", err)
	}
	return size, nil
}

func (s *DB) PruneCache(ctx context.Context, bytesToFree int64) (int64, error) {
	var freedBytes int64
	err := s.pool.QueryRow(ctx, `
		WITH deleted AS (
			DELETE FROM rpc_cache
			WHERE key IN (
				SELECT key
				FROM (
					SELECT key, result_length + 64 as item_size, SUM(result_length + 64) OVER (ORDER BY last_accessed_at ASC, result_length DESC) as running_total
					FROM rpc_cache
				) t
				WHERE running_total - item_size < $1
			)
			RETURNING result_length
		)
		SELECT COALESCE(SUM(result_length + 64), 0) FROM deleted;
	`, bytesToFree).Scan(&freedBytes)

	if err != nil {
		return 0, fmt.Errorf("failed to prune cache: %w", err)
	}
	return freedBytes, nil
}
