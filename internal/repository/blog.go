package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minato8080/ambiance-blogdog/internal/model"
)

type BlogRepository struct {
	db *pgxpool.Pool
}

func NewBlogRepository(db *pgxpool.Pool) *BlogRepository {
	return &BlogRepository{db: db}
}

// Upsert はブログURLをキーとして UPSERT する。既存行の name が空の場合のみ更新する。
func (r *BlogRepository) Upsert(ctx context.Context, blog *model.Blog) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO blogs (id, platform_id, blog_url, name, status, discovered_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (blog_url) DO UPDATE SET name = EXCLUDED.name
		WHERE blogs.name = '' AND EXCLUDED.name <> ''`,
		blog.ID, blog.PlatformID, blog.BlogURL, blog.Name, blog.Status, blog.DiscoveredAt,
	)
	if err != nil {
		return fmt.Errorf("blog.Upsert: %w", err)
	}
	return nil
}

// FindPending は status=pending または status=indexing のブログを最大 limit 件取得する。
func (r *BlogRepository) FindPending(ctx context.Context, limit int) ([]*model.Blog, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, platform_id, blog_url, name, status, error_count, last_synced_at, discovered_at
		FROM blogs
		WHERE status IN ('pending', 'indexing')
		ORDER BY discovered_at ASC
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("blog.FindPending: %w", err)
	}
	defer rows.Close()
	return scanBlogs(rows)
}

// FindStale は status=ready かつ last_synced_at が stalenessDays 日以上前のブログを返す。
func (r *BlogRepository) FindStale(ctx context.Context, stalenessDays, limit int) ([]*model.Blog, error) {
	threshold := time.Now().AddDate(0, 0, -stalenessDays)
	rows, err := r.db.Query(ctx, `
		SELECT id, platform_id, blog_url, name, status, error_count, last_synced_at, discovered_at
		FROM blogs
		WHERE status = 'ready'
		  AND (last_synced_at IS NULL OR last_synced_at < $1)
		ORDER BY last_synced_at ASC NULLS FIRST
		LIMIT $2`,
		threshold, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("blog.FindStale: %w", err)
	}
	defer rows.Close()
	return scanBlogs(rows)
}

// FindByBlogURL は blog_url でブログを取得する。見つからない場合は nil を返す。
func (r *BlogRepository) FindByBlogURL(ctx context.Context, blogURL string) (*model.Blog, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, platform_id, blog_url, name, status, error_count, last_synced_at, discovered_at
		FROM blogs WHERE blog_url = $1`, blogURL)
	b := &model.Blog{}
	err := row.Scan(&b.ID, &b.PlatformID, &b.BlogURL, &b.Name,
		&b.Status, &b.ErrorCount, &b.LastSyncedAt, &b.DiscoveredAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("blog.FindByBlogURL: %w", err)
	}
	return b, nil
}

// Delete はブログとその関連記事を削除する。
func (r *BlogRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM blogs WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("blog.Delete: %w", err)
	}
	return nil
}

// UpdateStatus は status・error_count・last_synced_at を更新する。
func (r *BlogRepository) UpdateStatus(ctx context.Context, id string, status model.BlogStatus, errorCount int, lastSyncedAt *time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE blogs
		SET status = $2, error_count = $3, last_synced_at = $4
		WHERE id = $1`,
		id, status, errorCount, lastSyncedAt,
	)
	if err != nil {
		return fmt.Errorf("blog.UpdateStatus: %w", err)
	}
	return nil
}

// List は全ブログを返す（管理用）。
func (r *BlogRepository) List(ctx context.Context) ([]*model.Blog, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, platform_id, blog_url, name, status, error_count, last_synced_at, discovered_at
		FROM blogs
		ORDER BY discovered_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("blog.List: %w", err)
	}
	defer rows.Close()
	return scanBlogs(rows)
}

// CountByStatus は各 status のブログ数を返す。
func (r *BlogRepository) CountByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.Query(ctx, `
		SELECT status, COUNT(*) FROM blogs GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("blog.CountByStatus: %w", err)
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

func scanBlogs(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]*model.Blog, error) {
	var blogs []*model.Blog
	for rows.Next() {
		b := &model.Blog{}
		if err := rows.Scan(
			&b.ID, &b.PlatformID, &b.BlogURL, &b.Name,
			&b.Status, &b.ErrorCount, &b.LastSyncedAt, &b.DiscoveredAt,
		); err != nil {
			return nil, err
		}
		blogs = append(blogs, b)
	}
	return blogs, rows.Err()
}
