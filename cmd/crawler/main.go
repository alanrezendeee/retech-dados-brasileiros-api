package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/theretech/retech-core/internal/cepdb"
	"github.com/theretech/retech-core/internal/observability"
)

func main() {
	log := observability.NewLogger(getenv("ENV", "development"))
	log.Info().Msg("🕷️  CEP Crawler starting")

	cepDBURL := getenv("CEPDB_URL", "")
	if cepDBURL == "" {
		log.Fatal().Msg("CEPDB_URL not set — set it to a PostgreSQL connection string")
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
		s := <-sig
		log.Info().Str("signal", s.String()).Msg("shutdown_signal")
		cancel()
	}()

	connectCtx, connectCancel := context.WithTimeout(ctx, 30*time.Second)
	defer connectCancel()

	db, err := cepdb.NewDB(connectCtx, cepDBURL, log)
	if err != nil {
		log.Fatal().Err(err).Msg("cepdb_connect_error")
	}
	defer db.Close()

	workers, _ := strconv.Atoi(getenv("CRAWLER_WORKERS", "3"))
	enumerate := getenv("CRAWLER_ENUMERATE", "false") == "true"

	crawler := cepdb.NewCrawler(db, cepdb.CrawlerConfig{
		Workers:   workers,
		Enumerate: enumerate,
	}, log)

	log.Info().
		Int("workers", workers).
		Bool("enumerate", enumerate).
		Msg("crawler_config")

	crawler.Run(ctx)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
