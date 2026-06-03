package cepdb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

type CrawlerConfig struct {
	Workers   int
	Enumerate bool // iniciar enumeração progressiva de todos os CEPs possíveis
}

type Crawler struct {
	db       *DB
	cfg      CrawlerConfig
	log      zerolog.Logger
	limiters map[string]*rate.Limiter
	fetchers map[string]func(context.Context, string) (*CEP, error)
}

var providerOrder = []string{"viacep", "brasilapi", "opencep"}

func NewCrawler(db *DB, cfg CrawlerConfig, log zerolog.Logger) *Crawler {
	if cfg.Workers <= 0 {
		cfg.Workers = 3
	}
	return &Crawler{
		db:  db,
		cfg: cfg,
		log: log.With().Str("component", "crawler").Logger(),
		limiters: map[string]*rate.Limiter{
			"viacep":    rate.NewLimiter(rate.Every(time.Second), 1),        // 60/min
			"brasilapi": rate.NewLimiter(rate.Every(time.Second), 1),        // 60/min
			"opencep":   rate.NewLimiter(rate.Every(2*time.Second), 1),      // 30/min
		},
		fetchers: map[string]func(context.Context, string) (*CEP, error){
			"viacep":    fetchViaCEP,
			"brasilapi": fetchBrasilAPI,
			"opencep":   fetchOpenCEP,
		},
	}
}

// Run bloqueia até ctx ser cancelado
func (c *Crawler) Run(ctx context.Context) {
	c.log.Info().Int("workers", c.cfg.Workers).Msg("crawler_start")

	go c.requeueExpiredLoop(ctx)

	if c.cfg.Enumerate {
		c.StartEnumeration(ctx)
	}

	done := make(chan struct{}, c.cfg.Workers)
	for i := 0; i < c.cfg.Workers; i++ {
		go func(id int) {
			c.workerLoop(ctx, id)
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < c.cfg.Workers; i++ {
		<-done
	}
	c.log.Info().Msg("crawler_stopped")
}

func (c *Crawler) workerLoop(ctx context.Context, id int) {
	providerIdx := id % len(providerOrder)
	log := c.log.With().Int("worker", id).Logger()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		item, err := c.db.Dequeue(ctx)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				log.Debug().Msg("queue_empty_waiting")
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Second):
				}
				continue
			}
			log.Error().Err(err).Msg("dequeue_error")
			time.Sleep(5 * time.Second)
			continue
		}

		c.processCEP(ctx, item, &providerIdx, log)
	}
}

func (c *Crawler) processCEP(ctx context.Context, item *QueueItem, providerIdx *int, log zerolog.Logger) {
	cepLog := log.With().Str("cep", item.CEP).Logger()
	cepLog.Debug().Msg("processing")

	var result *CEP
	var lastErr error

	for attempt := 0; attempt < len(providerOrder); attempt++ {
		provider := providerOrder[(*providerIdx+attempt)%len(providerOrder)]

		if err := c.limiters[provider].Wait(ctx); err != nil {
			return // ctx cancelado
		}

		start := time.Now()
		r, err := c.fetchers[provider](ctx, item.CEP)
		latencyMs := int(time.Since(start).Milliseconds())

		if err != nil {
			cepLog.Warn().Str("provider", provider).Err(err).Int("latency_ms", latencyMs).Msg("fetch_failed")
			c.db.LogCrawl(ctx, item.CEP, provider, false, latencyMs, err.Error())
			lastErr = err
			continue
		}

		cepLog.Info().Str("provider", provider).Int("latency_ms", latencyMs).Msg("fetch_ok")
		c.db.LogCrawl(ctx, item.CEP, provider, true, latencyMs, "")
		result = r
		break
	}

	*providerIdx = (*providerIdx + 1) % len(providerOrder)

	if result == nil {
		errMsg := "all providers failed"
		if lastErr != nil {
			errMsg = lastErr.Error()
		}
		var next time.Time
		switch {
		case item.Attempts < 2:
			next = time.Now().Add(5 * time.Minute)
		case item.Attempts < 5:
			next = time.Now().Add(time.Hour)
		default:
			next = time.Now().Add(24 * time.Hour)
		}
		if err := c.db.Nack(ctx, item.ID, errMsg, next); err != nil {
			cepLog.Error().Err(err).Msg("nack_error")
		}
		return
	}

	if err := c.db.Upsert(ctx, result); err != nil {
		cepLog.Error().Err(err).Msg("upsert_error")
		c.db.Nack(ctx, item.ID, err.Error(), time.Now().Add(time.Minute))
		return
	}

	if err := c.db.Ack(ctx, item.ID); err != nil {
		cepLog.Error().Err(err).Msg("ack_error")
	} else {
		cepLog.Info().Strs("sources", result.Sources).Msg("cep_saved")
	}
}

func (c *Crawler) requeueExpiredLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ceps, err := c.db.GetExpiredCEPs(ctx, 1000)
			if err != nil {
				c.log.Error().Err(err).Msg("get_expired_error")
				continue
			}
			if len(ceps) == 0 {
				continue
			}
			if err := c.db.EnqueueBatch(ctx, ceps, 50); err != nil {
				c.log.Error().Err(err).Msg("requeue_error")
				continue
			}
			c.log.Info().Int("count", len(ceps)).Msg("expired_requeued")
		}
	}
}

// StartEnumeration enfileira CEPs progressivamente em background (priority=0)
// Apenas insere CEPs que ainda não estão na fila — ON CONFLICT DO NOTHING
func (c *Crawler) StartEnumeration(ctx context.Context) {
	// Faixas por densidade (capitais primeiro)
	ranges := [][2]int{
		{1000000, 19999999},
		{20000000, 28999999},
		{30000000, 39999999},
		{40000000, 48999999},
		{50000000, 56999999},
		{57000000, 65999999},
		{66000000, 69999999},
		{70000000, 79999999},
		{80000000, 89999999},
		{90000000, 99999999},
	}

	go func() {
		c.log.Info().Msg("enumeration_started")
		total := 0
		for _, r := range ranges {
			for n := r[0]; n <= r[1]; n++ {
				select {
				case <-ctx.Done():
					c.log.Info().Int("enqueued", total).Msg("enumeration_stopped")
					return
				default:
				}
				cep := fmt.Sprintf("%08d", n)
				c.db.Enqueue(ctx, cep, 0) //nolint — erros ignorados intencionalmente
				total++
				if total%100000 == 0 {
					c.log.Info().Int("enqueued", total).Msg("enumeration_progress")
				}
				time.Sleep(time.Millisecond) // yield para não monopolizar CPU
			}
		}
		c.log.Info().Int("enqueued", total).Msg("enumeration_complete")
	}()
}
