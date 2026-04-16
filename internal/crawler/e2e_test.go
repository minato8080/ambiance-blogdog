//go:build e2e

package crawler_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvector "github.com/pgvector/pgvector-go/pgx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/minato8080/ambiance-blogdog/internal/crawler"
	"github.com/minato8080/ambiance-blogdog/internal/embedding"
	"github.com/minato8080/ambiance-blogdog/internal/repository"
	"github.com/minato8080/ambiance-blogdog/internal/rss"
)

const hatenaPlatformID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

func setupE2EPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	require.NotEmpty(t, dbURL, "DATABASE_URL が必要です")
	cfg, err := pgxpool.ParseConfig(dbURL)
	require.NoError(t, err)
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvector.RegisterTypes(ctx, conn)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

// TestDiscoverer_Run はフェーズ1の実際のクロールを検証する。
// 実行: go test -tags e2e ./internal/crawler/... -run TestDiscoverer_Run -v
func TestDiscoverer_Run(t *testing.T) {
	pool := setupE2EPool(t)
	blogRepo := repository.NewBlogRepository(pool)
	articleRepo := repository.NewArticleRepository(pool)

	d := crawler.NewDiscoverer(blogRepo, articleRepo, hatenaPlatformID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err := d.Run(ctx)
	require.NoError(t, err)

	// 少なくとも1件のブログが発見されているはず
	blogs, err := blogRepo.FindPending(ctx, 10)
	require.NoError(t, err)
	assert.NotEmpty(t, blogs, "ブログが1件以上発見されること")
}

// TestIndexer_Run はフェーズ2の実際のインデックス構築を検証する。
func TestIndexer_Run(t *testing.T) {
	pool := setupE2EPool(t)
	openaiKey := os.Getenv("OPENAI_API_KEY")
	require.NotEmpty(t, openaiKey, "OPENAI_API_KEY が必要です")

	blogRepo := repository.NewBlogRepository(pool)
	articleRepo := repository.NewArticleRepository(pool)
	embedClient := embedding.NewClient(openaiKey, 2)
	rssFetcher := rss.NewFetcher()

	ix := crawler.NewIndexer(blogRepo, articleRepo, rssFetcher, embedClient, 3)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := ix.Run(ctx)
	require.NoError(t, err)

	count, err := articleRepo.CountTotal(ctx)
	require.NoError(t, err)
	t.Logf("インデックス済み記事数: %d", count)
}
