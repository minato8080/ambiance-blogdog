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

	"github.com/minato8080/ambiance-blogdog/internal/embedding"
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
		Embedding:   make([]float32, embedding.Dimensions),
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
			Embedding: make([]float32, embedding.Dimensions),
		},
		{
			ID:        "01TEST0000000000000000003A",
			BlogID:    blogID,
			URL:       "https://example.hatenablog.com/entry/similar-b",
			Title:     "類似記事B",
			Summary:   "内容B",
			Tags:      []string{},
			Embedding: make([]float32, embedding.Dimensions),
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

	results, err := repo.SearchSimilar(ctx, make([]float32, embedding.Dimensions), articles[0].URL, "", 5)
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

func TestArticleRepository_TrimToNewest(t *testing.T) {
	pool := setupPool(t)
	repo := repository.NewArticleRepository(pool)
	ctx := context.Background()

	// このテスト専用のブログを作る（削除すると記事も CASCADE で消える）
	blogID := "01TESTTRIMBLOG000000000000"
	_, err := pool.Exec(ctx, `
		INSERT INTO blogs (id, platform_id, blog_url, status)
		VALUES ($1, '01ARZ3NDEKTSV4RRFFQ69G5FAV', 'https://trim-test.hatenablog.com', 'ready')
		ON CONFLICT (id) DO NOTHING`, blogID)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM blogs WHERE id = $1`, blogID) })

	// 公開日時が新しい順に 2025, 2024, 2023, NULL の4件を用意し、2件に絞る
	base := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	ids := []string{"01TESTTRIM0000000000000001", "01TESTTRIM0000000000000002", "01TESTTRIM0000000000000003", "01TESTTRIM0000000000000004"}
	urls := make([]string, len(ids))
	for i, id := range ids {
		var pub *time.Time
		if i < 3 {
			p := base.AddDate(2-i, 0, 0)
			pub = &p
		}
		urls[i] = "https://trim-test.hatenablog.com/entry/" + id
		require.NoError(t, repo.Upsert(ctx, &model.Article{
			ID: id, BlogID: blogID, URL: urls[i], Title: "trim", Tags: []string{},
			Embedding: make([]float32, embedding.Dimensions), PublishedAt: pub,
		}))
	}

	deleted, err := repo.TrimToNewest(ctx, blogID, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	count, err := repo.CountByBlogID(ctx, blogID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	for i, u := range urls {
		got, err := repo.FindByURL(ctx, u)
		require.NoError(t, err)
		if i < 2 {
			assert.NotNil(t, got, "新しい2件は残る: %s", u)
		} else {
			assert.Nil(t, got, "古い記事と公開日時 NULL の記事は削除される: %s", u)
		}
	}
}
