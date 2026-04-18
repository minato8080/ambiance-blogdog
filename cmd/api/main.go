package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minato8080/ambiance-blogdog/migrations"
	pgxvector "github.com/pgvector/pgvector-go/pgx"

	openai "github.com/sashabaranov/go-openai"

	"github.com/minato8080/ambiance-blogdog/config"
	"github.com/minato8080/ambiance-blogdog/internal/embedding"
	"github.com/minato8080/ambiance-blogdog/internal/handler"
	"github.com/minato8080/ambiance-blogdog/internal/middleware"
	"github.com/minato8080/ambiance-blogdog/internal/repository"
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

	if err := runMigrations(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	blogRepo := repository.NewBlogRepository(pool)
	articleRepo := repository.NewArticleRepository(pool)
	embedClient := embedding.NewClient(cfg.OpenAIAPIKey, cfg.CrawlConcurrency, openai.EmbeddingModel(cfg.EmbeddingModel))

	mux := http.NewServeMux()
	mux.Handle("GET /similar", handler.NewSimilarHandler(articleRepo, blogRepo, embedClient, hatenaPlatformID))
	mux.Handle("GET /blogs", middleware.APIKey(cfg.APIKey)(handler.NewBlogsHandler(blogRepo)))
	mux.Handle("GET /stats", middleware.APIKey(cfg.APIKey)(handler.NewStatsHandler(blogRepo, articleRepo)))

	corsMiddleware := buildCORSMiddleware(cfg.CORSAllowedOrigins)
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      middleware.Logger(corsMiddleware(mux)),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-quit:
		slog.Info("shutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return srv.Shutdown(shutdownCtx)
	}
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

func runMigrations(databaseURL string) error {
	d, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migrations source: %w", err)
	}
	migrateURL := strings.NewReplacer(
		"postgres://", "pgx5://",
		"postgresql://", "pgx5://",
	).Replace(databaseURL)
	m, err := migrate.NewWithSourceInstance("iofs", d, migrateURL)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
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

func buildCORSMiddleware(origins []string) func(http.Handler) http.Handler {
	allowAll := len(origins) == 1 && origins[0] == "*"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					for _, o := range origins {
						if o == origin {
							w.Header().Set("Access-Control-Allow-Origin", origin)
							w.Header().Set("Vary", "Origin")
							break
						}
					}
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
