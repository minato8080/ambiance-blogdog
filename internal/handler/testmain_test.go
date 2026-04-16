package handler_test

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvector "github.com/pgvector/pgvector-go/pgx"

	"github.com/minato8080/ambiance-blogdog/internal/embedding"
	"github.com/minato8080/ambiance-blogdog/internal/handler"
	"github.com/minato8080/ambiance-blogdog/internal/middleware"
	"github.com/minato8080/ambiance-blogdog/internal/repository"
)

const (
	testAPIKey    = "test-api-key"
	testPlatformID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
)

// buildTestServer はハンドラーをまとめた http.Handler を返す。
// DATABASE_URL が設定されていればリアル DB を使用し、なければ nil pool で起動する。
func buildTestServer(t *testing.T) http.Handler {
	t.Helper()

	apiKey := testAPIKey
	if v := os.Getenv("API_KEY"); v != "" {
		apiKey = v
	}

	var (
		blogRepo    *repository.BlogRepository
		articleRepo *repository.ArticleRepository
	)
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		pool := setupTestPool(t, dbURL)
		blogRepo = repository.NewBlogRepository(pool)
		articleRepo = repository.NewArticleRepository(pool)
	}

	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey == "" {
		openaiKey = "dummy"
	}
	embedClient := embedding.NewClient(openaiKey, 1)

	mux := http.NewServeMux()
	mux.Handle("GET /similar", handler.NewSimilarHandler(articleRepo, blogRepo, embedClient, testPlatformID))
	mux.Handle("GET /blogs", middleware.APIKey(apiKey)(handler.NewBlogsHandler(blogRepo)))
	mux.Handle("GET /stats", middleware.APIKey(apiKey)(handler.NewStatsHandler(blogRepo, articleRepo)))
	return mux
}

func setupTestPool(t *testing.T, dbURL string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("pool config: %v", err)
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvector.RegisterTypes(ctx, conn)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
