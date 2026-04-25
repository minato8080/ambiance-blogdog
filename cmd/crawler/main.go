package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

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

	ctx := context.Background()

	// CRAWLER_PHASE で実行フェーズを選択（未指定時は indexer）
	phase := strings.ToLower(os.Getenv("CRAWLER_PHASE"))
	if phase == "" {
		phase = "indexer"
	}

	slog.Info("crawler: start", "phase", phase)

	var runErr error
	switch phase {
	case "discovery":
		discoverer := crawler.NewDiscoverer(blogRepo, articleRepo, keywordRepo, rssFetcher, hatenaPlatformID, cfg.TFIDFSampleSize, cfg.TFIDFKeywordCount)
		runErr = discoverer.Run(ctx)
	case "indexer":
		indexer := crawler.NewIndexer(blogRepo, articleRepo, rssFetcher, embedClient, cfg.MaxArticlesPerBlog, cfg.IndexBatchSize, cfg.IndexMaxErrorCount, cfg.CrawlConcurrency)
		runErr = indexer.Run(ctx)
	case "syncer":
		syncer := crawler.NewSyncer(blogRepo, articleRepo, rssFetcher, embedClient, cfg.SyncStalenessDays, cfg.MaxArticlesPerBlog, cfg.SyncBatchSize, cfg.SyncMaxErrorCount)
		runErr = syncer.Run(ctx)
	case "historical":
		historical := crawler.NewHistorical(blogRepo, hatenaPlatformID, cfg.CrawlDateFrom, cfg.CrawlDateTo, cfg.HistoricalBookmarkMax, cfg.HistoricalDateWindowDays, cfg.HistoricalDateUsersMax)
		runErr = historical.Run(ctx)
	case "recent":
		recent := crawler.NewRecent(blogRepo, hatenaPlatformID)
		runErr = recent.Run(ctx)
	default:
		return fmt.Errorf("unknown CRAWLER_PHASE: %s", phase)
	}

	if runErr != nil {
		return fmt.Errorf("crawler %s: %w", phase, runErr)
	}

	slog.Info("crawler: done", "phase", phase)
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
