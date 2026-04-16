package repository_test

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

	"github.com/minato8080/ambiance-blogdog/internal/model"
	"github.com/minato8080/ambiance-blogdog/internal/repository"
)

func setupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL が未設定のためスキップ")
	}
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

func TestArticleRepository_UpsertAndFindByURL(t *testing.T) {
	pool := setupPool(t)
	repo := repository.NewArticleRepository(pool)
	ctx := context.Background()

	// テスト用ブログID（blogs テーブルに対応する行が必要）
	blogID := os.Getenv("TEST_BLOG_ID")
	if blogID == "" {
		t.Skip("TEST_BLOG_ID が未設定のためスキップ")
	}

	pubAt := time.Now().Truncate(time.Second)
	article := &model.Article{
		ID:          "01TEST0000000000000000001A",
		BlogID:      blogID,
		URL:         "https://example.hatenablog.com/entry/test-article-1",
		Title:       "テスト記事",
		Summary:     "テスト本文",
		Tags:        []string{"Go", "テスト"},
		Embedding:   make([]float32, 1536),
		PublishedAt: &pubAt,
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM articles WHERE url = $1`, article.URL)
	})

	err := repo.Upsert(ctx, article)
	require.NoError(t, err)

	found, err := repo.FindByURL(ctx, article.URL)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, article.URL, found.URL)
	assert.Equal(t, article.Title, found.Title)

	// 存在しない URL は nil を返す
	notFound, err := repo.FindByURL(ctx, "https://example.com/not-found")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestArticleRepository_SearchSimilar(t *testing.T) {
	pool := setupPool(t)
	repo := repository.NewArticleRepository(pool)
	ctx := context.Background()

	blogID := os.Getenv("TEST_BLOG_ID")
	if blogID == "" {
		t.Skip("TEST_BLOG_ID が未設定のためスキップ")
	}

	// ゼロベクトルの記事を2件登録
	articles := []*model.Article{
		{
			ID:        "01TEST0000000000000000002A",
			BlogID:    blogID,
			URL:       "https://example.hatenablog.com/entry/similar-a",
			Title:     "類似記事A",
			Summary:   "内容A",
			Tags:      []string{},
			Embedding: make([]float32, 1536),
		},
		{
			ID:        "01TEST0000000000000000003A",
			BlogID:    blogID,
			URL:       "https://example.hatenablog.com/entry/similar-b",
			Title:     "類似記事B",
			Summary:   "内容B",
			Tags:      []string{},
			Embedding: make([]float32, 1536),
		},
	}
	t.Cleanup(func() {
		for _, a := range articles {
			pool.Exec(ctx, `DELETE FROM articles WHERE url = $1`, a.URL)
		}
	})
	for _, a := range articles {
		require.NoError(t, repo.Upsert(ctx, a))
	}

	results, err := repo.SearchSimilar(ctx, make([]float32, 1536), articles[0].URL, 5)
	require.NoError(t, err)
	// excludeURL は結果に含まれない
	for _, r := range results {
		assert.NotEqual(t, articles[0].URL, r.URL)
	}
}

func TestArticleRepository_CountByBlogID(t *testing.T) {
	pool := setupPool(t)
	repo := repository.NewArticleRepository(pool)
	ctx := context.Background()

	blogID := os.Getenv("TEST_BLOG_ID")
	if blogID == "" {
		t.Skip("TEST_BLOG_ID が未設定のためスキップ")
	}

	count, err := repo.CountByBlogID(ctx, blogID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 0)
}

func TestArticleRepository_DeleteOldest(t *testing.T) {
	pool := setupPool(t)
	repo := repository.NewArticleRepository(pool)
	ctx := context.Background()

	blogID := os.Getenv("TEST_BLOG_ID")
	if blogID == "" {
		t.Skip("TEST_BLOG_ID が未設定のためスキップ")
	}

	older := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	a := &model.Article{
		ID:          "01TEST0000000000000000004A",
		BlogID:      blogID,
		URL:         "https://example.hatenablog.com/entry/oldest",
		Title:       "最古記事",
		Summary:     "",
		Tags:        []string{},
		Embedding:   make([]float32, 1536),
		PublishedAt: &older,
	}
	require.NoError(t, repo.Upsert(ctx, a))

	beforeCount, _ := repo.CountByBlogID(ctx, blogID)
	require.NoError(t, repo.DeleteOldest(ctx, blogID))
	afterCount, _ := repo.CountByBlogID(ctx, blogID)
	assert.Equal(t, beforeCount-1, afterCount)
}
