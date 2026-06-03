package cepdb

import "context"

const ddl = `
CREATE TABLE IF NOT EXISTS ceps (
    id              BIGSERIAL PRIMARY KEY,
    cep             CHAR(8)  NOT NULL UNIQUE,
    cep_formatted   CHAR(9)  NOT NULL,
    logradouro      TEXT,
    complemento     TEXT,
    bairro          TEXT,
    localidade      TEXT NOT NULL DEFAULT '',
    uf              CHAR(2) NOT NULL DEFAULT '',
    ibge            CHAR(7),
    gia             TEXT,
    ddd             CHAR(2),
    siafi           TEXT,
    latitude        DECIMAL(10,8),
    longitude       DECIMAL(11,8),
    sources         TEXT[] NOT NULL DEFAULT '{}',
    quality_score   SMALLINT NOT NULL DEFAULT 0,
    not_found_count SMALLINT NOT NULL DEFAULT 0,
    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    verified_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '90 days'
);

CREATE INDEX IF NOT EXISTS idx_ceps_uf      ON ceps(uf);
CREATE INDEX IF NOT EXISTS idx_ceps_ibge    ON ceps(ibge);
CREATE INDEX IF NOT EXISTS idx_ceps_ddd     ON ceps(ddd);
CREATE INDEX IF NOT EXISTS idx_ceps_expires ON ceps(expires_at);

CREATE INDEX IF NOT EXISTS idx_ceps_fts ON ceps USING gin(
    to_tsvector('portuguese',
        COALESCE(logradouro, '') || ' ' ||
        COALESCE(bairro, '')     || ' ' ||
        localidade
    )
);

CREATE TABLE IF NOT EXISTS cep_crawl_queue (
    id               BIGSERIAL PRIMARY KEY,
    cep              CHAR(8)     NOT NULL UNIQUE,
    priority         SMALLINT    NOT NULL DEFAULT 0,
    attempts         SMALLINT    NOT NULL DEFAULT 0,
    status           VARCHAR(20) NOT NULL DEFAULT 'pending',
    next_attempt_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_queue_pick ON cep_crawl_queue(priority DESC, next_attempt_at ASC)
    WHERE status IN ('pending', 'failed');

CREATE TABLE IF NOT EXISTS cep_crawl_logs (
    id          BIGSERIAL   PRIMARY KEY,
    cep         CHAR(8)     NOT NULL,
    provider    VARCHAR(50) NOT NULL,
    success     BOOLEAN     NOT NULL,
    latency_ms  SMALLINT,
    error       TEXT,
    crawled_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_crawl_logs_date ON cep_crawl_logs(crawled_at DESC);
`

func (db *DB) autoMigrate(ctx context.Context) error {
	if _, err := db.Pool.Exec(ctx, ddl); err != nil {
		return err
	}
	db.log.Info().Msg("cepdb_schema_ready")
	return nil
}
