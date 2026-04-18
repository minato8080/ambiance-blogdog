package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type KeywordRepository struct {
	db *pgxpool.Pool
}

func NewKeywordRepository(db *pgxpool.Pool) *KeywordRepository {
	return &KeywordRepository{db: db}
}

// Replace は既存キーワードを全削除して新しいキーワード一覧を INSERT する。
func (r *KeywordRepository) Replace(ctx context.Context, keywords []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("keyword.Replace: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM crawler_keywords`); err != nil {
		return fmt.Errorf("keyword.Replace delete: %w", err)
	}

	now := time.Now()
	for _, kw := range keywords {
		if _, err := tx.Exec(ctx,
			`INSERT INTO crawler_keywords (keyword, updated_at) VALUES ($1, $2)`,
			kw, now,
		); err != nil {
			return fmt.Errorf("keyword.Replace insert %q: %w", kw, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("keyword.Replace commit: %w", err)
	}
	return nil
}

// List は現在のキーワード一覧を返す。
func (r *KeywordRepository) List(ctx context.Context) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT keyword FROM crawler_keywords ORDER BY keyword`)
	if err != nil {
		return nil, fmt.Errorf("keyword.List: %w", err)
	}
	defer rows.Close()

	var keywords []string
	for rows.Next() {
		var kw string
		if err := rows.Scan(&kw); err != nil {
			return nil, err
		}
		keywords = append(keywords, kw)
	}
	return keywords, rows.Err()
}
