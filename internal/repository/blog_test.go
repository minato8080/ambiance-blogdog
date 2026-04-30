package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/minato8080/ambiance-blogdog/internal/model"
	"github.com/minato8080/ambiance-blogdog/internal/repository"
)

const testPlatformID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

func newTestBlog(blogURL, name string) *model.Blog {
	return &model.Blog{
		ID:           ulid.Make().String(),
		PlatformID:   testPlatformID,
		BlogURL:      blogURL,
		Name:         name,
		Status:       model.BlogStatusPending,
		DiscoveredAt: time.Now(),
	}
}

func TestBlogRepository_Upsert_New(t *testing.T) {
	pool := setupPool(t)
	repo := repository.NewBlogRepository(pool)
	ctx := context.Background()

	blog := newTestBlog("https://upsert-new.hatenablog.com", "New Blog")
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM blogs WHERE blog_url = $1`, blog.BlogURL) })

	require.NoError(t, repo.Upsert(ctx, blog))

	found, err := repo.FindByBlogURL(ctx, blog.BlogURL)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "New Blog", found.Name)
}

func TestBlogRepository_Upsert_NameProtected(t *testing.T) {
	pool := setupPool(t)
	repo := repository.NewBlogRepository(pool)
	ctx := context.Background()

	blog := newTestBlog("https://name-protect.hatenablog.com", "Original Name")
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM blogs WHERE blog_url = $1`, blog.BlogURL) })
	require.NoError(t, repo.Upsert(ctx, blog))

	// 既存のnameがある場合、別のnameで上書きしようとしても変わらない
	dup := newTestBlog(blog.BlogURL, "New Name")
	require.NoError(t, repo.Upsert(ctx, dup))

	found, err := repo.FindByBlogURL(ctx, blog.BlogURL)
	require.NoError(t, err)
	assert.Equal(t, "Original Name", found.Name)
}

func TestBlogRepository_Upsert_EmptyNameFilledIn(t *testing.T) {
	pool := setupPool(t)
	repo := repository.NewBlogRepository(pool)
	ctx := context.Background()

	blog := newTestBlog("https://empty-name.hatenablog.com", "")
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM blogs WHERE blog_url = $1`, blog.BlogURL) })
	require.NoError(t, repo.Upsert(ctx, blog))

	// nameが空の場合は新しいnameで更新される
	dup := newTestBlog(blog.BlogURL, "Filled Name")
	require.NoError(t, repo.Upsert(ctx, dup))

	found, err := repo.FindByBlogURL(ctx, blog.BlogURL)
	require.NoError(t, err)
	assert.Equal(t, "Filled Name", found.Name)
}

func TestBlogRepository_UpdateName_SetsEmptyName(t *testing.T) {
	pool := setupPool(t)
	repo := repository.NewBlogRepository(pool)
	ctx := context.Background()

	blog := newTestBlog("https://update-name.hatenablog.com", "")
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM blogs WHERE blog_url = $1`, blog.BlogURL) })
	require.NoError(t, repo.Upsert(ctx, blog))

	require.NoError(t, repo.UpdateName(ctx, blog.ID, "Updated Name"))

	found, err := repo.FindByBlogURL(ctx, blog.BlogURL)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", found.Name)
}

func TestBlogRepository_UpdateName_IgnoresNonEmptyName(t *testing.T) {
	pool := setupPool(t)
	repo := repository.NewBlogRepository(pool)
	ctx := context.Background()

	blog := newTestBlog("https://no-overwrite.hatenablog.com", "Existing Name")
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM blogs WHERE blog_url = $1`, blog.BlogURL) })
	require.NoError(t, repo.Upsert(ctx, blog))

	require.NoError(t, repo.UpdateName(ctx, blog.ID, "Should Not Replace"))

	found, err := repo.FindByBlogURL(ctx, blog.BlogURL)
	require.NoError(t, err)
	assert.Equal(t, "Existing Name", found.Name)
}

func TestBlogRepository_FindPending_ReturnsPendingAndIndexing(t *testing.T) {
	pool := setupPool(t)
	repo := repository.NewBlogRepository(pool)
	ctx := context.Background()

	pending := newTestBlog("https://findpending-p.hatenablog.com", "")
	indexing := &model.Blog{
		ID:           ulid.Make().String(),
		PlatformID:   testPlatformID,
		BlogURL:      "https://findpending-i.hatenablog.com",
		Status:       model.BlogStatusIndexing,
		DiscoveredAt: time.Now(),
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM blogs WHERE blog_url IN ($1, $2)`, pending.BlogURL, indexing.BlogURL)
	})
	require.NoError(t, repo.Upsert(ctx, pending))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO blogs (id, platform_id, blog_url, name, status, discovered_at) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		indexing.ID, indexing.PlatformID, indexing.BlogURL, "", indexing.Status, indexing.DiscoveredAt,
	).Scan(&indexing.ID))

	blogs, err := repo.FindPending(ctx, 100)
	require.NoError(t, err)

	urls := make(map[string]bool)
	for _, b := range blogs {
		urls[b.BlogURL] = true
	}
	assert.True(t, urls[pending.BlogURL])
	assert.True(t, urls[indexing.BlogURL])
}

func TestBlogRepository_FindStale_ReturnsOldReadyBlogs(t *testing.T) {
	pool := setupPool(t)
	repo := repository.NewBlogRepository(pool)
	ctx := context.Background()

	staleURL := "https://findstale.hatenablog.com"
	oldSync := time.Now().AddDate(0, 0, -60)
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM blogs WHERE blog_url = $1`, staleURL) })

	_, err := pool.Exec(ctx, `
		INSERT INTO blogs (id, platform_id, blog_url, name, status, error_count, last_synced_at, discovered_at)
		VALUES ($1, $2, $3, '', 'ready', 0, $4, $5)`,
		ulid.Make().String(), testPlatformID, staleURL, oldSync, time.Now(),
	)
	require.NoError(t, err)

	blogs, err := repo.FindStale(ctx, 30, 100)
	require.NoError(t, err)

	urls := make(map[string]bool)
	for _, b := range blogs {
		urls[b.BlogURL] = true
	}
	assert.True(t, urls[staleURL])
}
