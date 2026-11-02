package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minato8080/ambiance-blogdog/internal/model"
	pgvector "github.com/pgvector/pgvector-go"
)

type ArticleRepository struct {
	db *pgxpool.Pool
}

func NewArticleRepository(db *pgxpool.Pool) *ArticleRepository {
	return &ArticleRepository{db: db}
}

// Upsert は記事URLをキーとして UPSERT する。
func (r *ArticleRepository) Upsert(ctx context.Context, a *model.Article) error {
	var vec *pgvector.HalfVector
	if a.Embedding != nil {
		v := pgvector.NewHalfVector(a.Embedding)
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
	var vec pgvector.HalfVector
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

// SearchSimilarSQL は SearchSimilar が実行する SQL（評価ツールの dbcheck も同じものを使う）。
// $1: 検索ベクトル、$2: 除外する記事URL、$3: 除外するブログID（空文字なら除外しない）、$4: 件数。
//
//   - 距離は副問い合わせで1回だけ計算する（SELECT と ORDER BY に同じ式を書くと2回計算される）。
//   - blogs との結合は上位 $4 件に絞った後に行う（全行を結合してから並べ替えない）。
const SearchSimilarSQL = `
		SELECT s.url, s.title, COALESCE(b.blog_url, ''), COALESCE(b.name, ''), s.tags, s.published_at,
		       1 - s.distance AS similarity
		FROM (
			SELECT url, title, tags, published_at, blog_id, embedding <=> $1 AS distance
			FROM articles
			WHERE url != $2
			  AND ($3 = '' OR blog_id != $3)
			  AND embedding IS NOT NULL
			ORDER BY distance
			LIMIT $4
		) s
		LEFT JOIN blogs b ON b.id = s.blog_id
		ORDER BY s.distance`

// SearchSimilar はコサイン類似度で類似記事を検索する（excludeURL および同一ブログを除外）。
// embedding（halfvec(512)）には索引を張っていないため、全件比較で正確な上位 limit 件を返す。
func (r *ArticleRepository) SearchSimilar(ctx context.Context, embedding []float32, excludeURL string, excludeBlogID string, limit int) ([]*model.SimilarArticle, error) {
	vec := pgvector.NewHalfVector(embedding)
	rows, err := r.db.Query(ctx, SearchSimilarSQL, vec, excludeURL, excludeBlogID, limit)
	if err != nil {
		return nil, fmt.Errorf("article.SearchSimilar: %w", err)
	}
	defer rows.Close()

	var results []*model.SimilarArticle
	for rows.Next() {
		s := &model.SimilarArticle{}
		if err := rows.Scan(&s.URL, &s.Title, &s.BlogURL, &s.BlogName, &s.Tags, &s.PublishedAt, &s.Similarity); err != nil {
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

// TrimToNewest は blogID の記事を公開日時が新しい順に keep 件だけ残し、残りを削除する（FIFO）。
// 公開日時が NULL の記事は最も古いものとして扱う。削除した件数を返す。
func (r *ArticleRepository) TrimToNewest(ctx context.Context, blogID string, keep int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM articles
		WHERE blog_id = $1
		  AND id NOT IN (
			SELECT id FROM articles
			WHERE blog_id = $1
			ORDER BY published_at DESC NULLS LAST, indexed_at DESC, id DESC
			LIMIT $2
		  )`,
		blogID, keep,
	)
	if err != nil {
		return 0, fmt.Errorf("article.TrimToNewest: %w", err)
	}
	return tag.RowsAffected(), nil
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

// FindRandom は embedding を持つ記事からランダムに1件返す。記事が存在しない場合は nil を返す。
// 全行を並べ替える対象は id だけにして、選んだ1件の全列を主キーで取り直す。
func (r *ArticleRepository) FindRandom(ctx context.Context) (*model.Article, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, blog_id, url, title, summary, tags, embedding, published_at, indexed_at
		FROM articles
		WHERE id = (
			SELECT id FROM articles
			WHERE embedding IS NOT NULL
			ORDER BY RANDOM()
			LIMIT 1
		)`)
	a := &model.Article{}
	var vec pgvector.HalfVector
	err := row.Scan(
		&a.ID, &a.BlogID, &a.URL, &a.Title, &a.Summary, &a.Tags,
		&vec, &a.PublishedAt, &a.IndexedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("article.FindRandom: %w", err)
	}
	a.Embedding = vec.Slice()
	return a, nil
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
