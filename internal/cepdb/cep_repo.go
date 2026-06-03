package cepdb

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type CEP struct {
	ID            int64
	CEP           string
	CEPFormatted  string
	Logradouro    string
	Complemento   string
	Bairro        string
	Localidade    string
	UF            string
	IBGE          string
	GIA           string
	DDD           string
	SIAFI         string
	Latitude      float64
	Longitude     float64
	Sources       []string
	QualityScore  int16
	NotFoundCount int16
	FirstSeenAt   time.Time
	VerifiedAt    time.Time
	UpdatedAt     time.Time
	ExpiresAt     time.Time
}

var ErrNotFound = errors.New("cep not found")

func normalizeCEP(cep string) string {
	cep = strings.ReplaceAll(cep, "-", "")
	cep = strings.ReplaceAll(cep, ".", "")
	return cep
}

func formatCEP(cep string) string {
	if len(cep) == 8 {
		return cep[:5] + "-" + cep[5:]
	}
	return cep
}

func (db *DB) GetByCEP(ctx context.Context, cep string) (*CEP, error) {
	cep = normalizeCEP(cep)
	row := db.Pool.QueryRow(ctx, `
		SELECT id, cep, cep_formatted, COALESCE(logradouro,''), COALESCE(complemento,''),
		       COALESCE(bairro,''), localidade, uf, COALESCE(ibge,''), COALESCE(gia,''),
		       COALESCE(ddd,''), COALESCE(siafi,''),
		       COALESCE(latitude,0), COALESCE(longitude,0),
		       sources, quality_score, not_found_count,
		       first_seen_at, verified_at, updated_at, expires_at
		FROM ceps WHERE cep = $1
	`, cep)

	var c CEP
	err := row.Scan(
		&c.ID, &c.CEP, &c.CEPFormatted, &c.Logradouro, &c.Complemento, &c.Bairro,
		&c.Localidade, &c.UF, &c.IBGE, &c.GIA, &c.DDD, &c.SIAFI,
		&c.Latitude, &c.Longitude, &c.Sources, &c.QualityScore,
		&c.NotFoundCount, &c.FirstSeenAt, &c.VerifiedAt, &c.UpdatedAt, &c.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (db *DB) Upsert(ctx context.Context, c *CEP) error {
	c.CEP = normalizeCEP(c.CEP)
	if c.CEPFormatted == "" {
		c.CEPFormatted = formatCEP(c.CEP)
	}
	if len(c.Sources) == 0 {
		c.Sources = []string{}
	}

	_, err := db.Pool.Exec(ctx, `
		INSERT INTO ceps (
			cep, cep_formatted, logradouro, complemento, bairro, localidade, uf,
			ibge, gia, ddd, siafi, latitude, longitude, sources, quality_score
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (cep) DO UPDATE SET
			logradouro    = EXCLUDED.logradouro,
			complemento   = EXCLUDED.complemento,
			bairro        = EXCLUDED.bairro,
			localidade    = EXCLUDED.localidade,
			uf            = EXCLUDED.uf,
			ibge          = COALESCE(NULLIF(EXCLUDED.ibge,''), ceps.ibge),
			gia           = COALESCE(NULLIF(EXCLUDED.gia,''), ceps.gia),
			ddd           = COALESCE(NULLIF(EXCLUDED.ddd,''), ceps.ddd),
			siafi         = COALESCE(NULLIF(EXCLUDED.siafi,''), ceps.siafi),
			latitude      = CASE WHEN EXCLUDED.latitude != 0 THEN EXCLUDED.latitude ELSE ceps.latitude END,
			longitude     = CASE WHEN EXCLUDED.longitude != 0 THEN EXCLUDED.longitude ELSE ceps.longitude END,
			sources       = (SELECT ARRAY(SELECT DISTINCT unnest FROM unnest(ceps.sources || EXCLUDED.sources))),
			quality_score = LEAST(ceps.quality_score + 10, 100),
			verified_at   = NOW(),
			updated_at    = NOW(),
			expires_at    = NOW() + INTERVAL '90 days'
	`,
		c.CEP, c.CEPFormatted, c.Logradouro, c.Complemento, c.Bairro, c.Localidade, c.UF,
		c.IBGE, c.GIA, c.DDD, c.SIAFI, c.Latitude, c.Longitude, c.Sources, c.QualityScore,
	)
	return err
}

func (db *DB) GetExpiredCEPs(ctx context.Context, limit int) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT cep FROM ceps
		WHERE expires_at < NOW()
		ORDER BY expires_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ceps []string
	for rows.Next() {
		var cep string
		if err := rows.Scan(&cep); err != nil {
			return nil, err
		}
		ceps = append(ceps, cep)
	}
	return ceps, nil
}

func (db *DB) TotalCount(ctx context.Context) (int64, error) {
	var count int64
	err := db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM ceps").Scan(&count)
	return count, err
}

func (db *DB) SearchByAddress(ctx context.Context, uf, query string, limit int) ([]*CEP, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT cep, cep_formatted, COALESCE(logradouro,''), COALESCE(complemento,''),
		       COALESCE(bairro,''), localidade, uf, COALESCE(ibge,''), COALESCE(ddd,'')
		FROM ceps
		WHERE uf = $1
		  AND to_tsvector('portuguese',
		        COALESCE(logradouro,'') || ' ' ||
		        COALESCE(bairro,'') || ' ' || localidade)
		      @@ plainto_tsquery('portuguese', $2)
		ORDER BY quality_score DESC
		LIMIT $3
	`, strings.ToUpper(uf), query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*CEP
	for rows.Next() {
		c := &CEP{}
		if err := rows.Scan(&c.CEP, &c.CEPFormatted, &c.Logradouro, &c.Complemento,
			&c.Bairro, &c.Localidade, &c.UF, &c.IBGE, &c.DDD); err != nil {
			return nil, err
		}
		results = append(results, c)
	}
	return results, nil
}
