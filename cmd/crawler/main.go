package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvector "github.com/pgvector/pgvector-go/pgx"

	openai "github.com/sashabaranov/go-openai"

	"github.com/minato8080/ambiance-blogdog/config"
	"github.com/minato8080/ambiance-blogdog/internal/crawler"
	"github.com/minato8080/ambiance-blogdog/internal/embedding"
	"github.com/minato8080/ambiance-blogdog/internal/repository"
	"github.com/minato8080/ambiance-blogdog/internal/rss"
)

const hatenaPlatformID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	setupLogger(cfg.LogLevel)

	pool, err := newPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer pool.Close()

	blogRepo := repository.NewBlogRepository(pool)
	articleRepo := repository.NewArticleRepository(pool)
	keywordRepo := repository.NewKeywordRepository(pool)
	embedClient := embedding.NewClient(cfg.OpenAIAPIKey, cfg.CrawlConcurrency, openai.EmbeddingModel(cfg.EmbeddingModel))
	rssFetcher := rss.NewFetcher()

	discoverer := crawler.NewDiscoverer(blogRepo, articleRepo, keywordRepo, rssFetcher, hatenaPlatformID, cfg.TFIDFSampleSize, cfg.TFIDFKeywordCount)
	indexer := crawler.NewIndexer(blogRepo, articleRepo, rssFetcher, embedClient, cfg.MaxArticlesPerBlog, cfg.IndexBatchSize, cfg.IndexMaxErrorCount, cfg.CrawlConcurrency)
	syncer := crawler.NewSyncer(blogRepo, articleRepo, rssFetcher, embedClient, cfg.SyncStalenessDays, cfg.MaxArticlesPerBlog, cfg.SyncBatchSize, cfg.SyncMaxErrorCount)
	historical := crawler.NewHistorical(blogRepo, hatenaPlatformID, cfg.CrawlDateFrom, cfg.CrawlDateTo, cfg.HistoricalBookmarkMax, cfg.HistoricalDateWindowDays, cfg.HistoricalDateUsersMax)
	recent := crawler.NewRecent(blogRepo, hatenaPlatformID)
	sched := crawler.NewScheduler(discoverer, indexer, syncer, historical, recent, cfg.CrawlIntervalMin, cfg.SyncIntervalMin, cfg.HistoricalIntervalMin, cfg.RecentIntervalMin)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sched.Start(ctx)
	slog.Info("crawler started")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("crawler shutting down...")
	cancel()
	return nil
}

func newPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvector.RegisterTypes(ctx, conn)
	}
	return pgxpool.NewWithConfig(ctx, poolCfg)
}

func setupLogger(level string) {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})))
}
