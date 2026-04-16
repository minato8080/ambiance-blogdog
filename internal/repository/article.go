package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
	"github.com/minato8080/ambiance-blogdog/internal/model"
)

type ArticleRepository struct {
	db *pgxpool.Pool
}

func NewArticleRepository(db *pgxpool.Pool) *ArticleRepository {
	return &ArticleRepository{db: db}
}

// Upsert は記事URLをキーとして UPSERT する。
func (r *ArticleRepository) Upsert(ctx context.Context, a *model.Article) error {
	var vec *pgvector.Vector
	if a.Embedding != nil {
		v := pgvector.NewVector(a.Embedding)
		vec = &v
	}
	tags := a.Tags
	if tags == nil {
		tags = []string{}
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO articles (id, blog_id, url, title, summary, tags, embedding, published_at, indexed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (url) DO UPDATE SET
			title        = EXCLUDED.title,
			summary      = EXCLUDED.summary,
			tags         = EXCLUDED.tags,
			embedding    = EXCLUDED.embedding,
			published_at = EXCLUDED.published_at,
			indexed_at   = NOW()`,
		a.ID, a.BlogID, a.URL, a.Title, a.Summary, tags, vec, a.PublishedAt,
	)
	if err != nil {
		return fmt.Errorf("article.Upsert: %w", err)
	}
	return nil
}

// FindByURL は URL で記事を取得する。見つからない場合は nil を返す。
func (r *ArticleRepository) FindByURL(ctx context.Context, url string) (*model.Article, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, blog_id, url, title, summary, tags, embedding, published_at, indexed_at
		FROM articles
		WHERE url = $1`,
		url,
	)
	a := &model.Article{}
	var vec pgvector.Vector
	err := row.Scan(
		&a.ID, &a.BlogID, &a.URL, &a.Title, &a.Summary, &a.Tags,
		&vec, &a.PublishedAt, &a.IndexedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("article.FindByURL: %w", err)
	}
	a.Embedding = vec.Slice()
	return a, nil
}

// SearchSimilar はコサイン類似度で類似記事を検索する（excludeURL を除外）。
func (r *ArticleRepository) SearchSimilar(ctx context.Context, embedding []float32, excludeURL string, limit int) ([]*model.SimilarArticle, error) {
	vec := pgvector.NewVector(embedding)
	rows, err := r.db.Query(ctx, `
		SELECT url, title, tags, published_at,
		       1 - (embedding <=> $1) AS similarity
		FROM articles
		WHERE url != $2
		  AND embedding IS NOT NULL
		ORDER BY embedding <=> $1
		LIMIT $3`,
		vec, excludeURL, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("article.SearchSimilar: %w", err)
	}
	defer rows.Close()

	var results []*model.SimilarArticle
	for rows.Next() {
		s := &model.SimilarArticle{}
		if err := rows.Scan(&s.URL, &s.Title, &s.Tags, &s.PublishedAt, &s.Similarity); err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// CountByBlogID はブログ別記事数を返す。
func (r *ArticleRepository) CountByBlogID(ctx context.Context, blogID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM articles WHERE blog_id = $1`, blogID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("article.CountByBlogID: %w", err)
	}
	return count, nil
}

// DeleteOldest は blogID の記事のうち最古 1 件を削除する。
func (r *ArticleRepository) DeleteOldest(ctx context.Context, blogID string) error {
	_, err := r.db.Exec(ctx, `
		DELETE FROM articles
		WHERE id = (
			SELECT id FROM articles
			WHERE blog_id = $1
			ORDER BY published_at ASC NULLS FIRST
			LIMIT 1
		)`,
		blogID,
	)
	if err != nil {
		return fmt.Errorf("article.DeleteOldest: %w", err)
	}
	return nil
}

// SampleSummaries は最新記事のタイトルとサマリーを結合した文字列を最大 limit 件返す。
func (r *ArticleRepository) SampleSummaries(ctx context.Context, limit int) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT title || ' ' || COALESCE(summary, '')
		FROM articles
		ORDER BY indexed_at DESC
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("article.SampleSummaries: %w", err)
	}
	defer rows.Close()

	var docs []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		docs = append(docs, s)
	}
	return docs, rows.Err()
}

// CountTotal は全記事数を返す。
func (r *ArticleRepository) CountTotal(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM articles`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("article.CountTotal: %w", err)
	}
	return count, nil
}
