package crawler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/minato8080/ambiance-blogdog/internal/embedding"
	"github.com/minato8080/ambiance-blogdog/internal/model"
	"github.com/minato8080/ambiance-blogdog/internal/repository"
	"github.com/minato8080/ambiance-blogdog/internal/rss"
	"github.com/oklog/ulid/v2"
)

// Syncer はフェーズ3: 差分更新クローラー。
type Syncer struct {
	blogRepo      *repository.BlogRepository
	articleRepo   *repository.ArticleRepository
	rssFetcher    *rss.Fetcher
	embedClient   *embedding.Client
	stalenessDays int
	maxArticles   int
	batchSize     int
	maxErrorCount int
}

func NewSyncer(
	blogRepo *repository.BlogRepository,
	articleRepo *repository.ArticleRepository,
	rssFetcher *rss.Fetcher,
	embedClient *embedding.Client,
	stalenessDays, maxArticles, batchSize, maxErrorCount int,
) *Syncer {
	return &Syncer{
		blogRepo:      blogRepo,
		articleRepo:   articleRepo,
		rssFetcher:    rssFetcher,
		embedClient:   embedClient,
		stalenessDays: stalenessDays,
		maxArticles:   maxArticles,
		batchSize:     batchSize,
		maxErrorCount: maxErrorCount,
	}
}

// Run は stale な ready ブログを差分チェックする。
func (s *Syncer) Run(ctx context.Context) error {
	blogs, err := s.blogRepo.FindStale(ctx, s.stalenessDays, s.batchSize)
	if err != nil {
		return fmt.Errorf("syncer.Run: %w", err)
	}

	for i, blog := range blogs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.syncBlog(ctx, blog); err != nil {
			slog.Warn("syncer: blog failed", "blog_url", blog.BlogURL, "error", err)
		}
		// robots.txt 遵守: ブログ間に1秒以上のインターバル（最後の1件はスキップ）
		if i < len(blogs)-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}
	return nil
}

func (s *Syncer) syncBlog(ctx context.Context, blog *model.Blog) error {
	feedURL := blog.BlogURL + "/feed"
	articles, err := s.rssFetcher.Fetch(ctx, feedURL, s.maxArticles)
	if err != nil {
		blog.ErrorCount++
		status := model.BlogStatusReady
		if blog.ErrorCount >= s.maxErrorCount {
			status = model.BlogStatusError
		}
		return s.blogRepo.UpdateStatus(ctx, blog.ID, status, blog.ErrorCount, blog.LastSyncedAt)
	}

	for _, a := range articles {
		if err := ctx.Err(); err != nil {
			return err
		}
		existing, err := s.articleRepo.FindByURL(ctx, a.URL)
		if err != nil {
			slog.Warn("syncer: findByURL failed", "url", a.URL, "error", err)
			continue
		}

		// published_at が変化していれば再ベクトル化
		changed := existing == nil ||
			(a.PublishedAt != nil && existing.PublishedAt == nil) ||
			(a.PublishedAt != nil && existing.PublishedAt != nil && !a.PublishedAt.Equal(*existing.PublishedAt))

		if !changed {
			continue
		}

		if err := s.upsertArticle(ctx, blog.ID, a); err != nil {
			slog.Warn("syncer: upsert failed", "url", a.URL, "error", err)
		}
		time.Sleep(time.Second)
	}

	syncedAt := time.Now()
	return s.blogRepo.UpdateStatus(ctx, blog.ID, model.BlogStatusReady, 0, &syncedAt)
}

func (s *Syncer) upsertArticle(ctx context.Context, blogID string, a *rss.Article) error {
	text := strings.TrimSpace(a.Title + "\n" + a.Summary)
	emb, err := s.embedClient.Embed(ctx, text)
	if err != nil {
		return fmt.Errorf("embed %s: %w", a.URL, err)
	}
	article := &model.Article{
		ID:          ulid.Make().String(),
		BlogID:      blogID,
		URL:         a.URL,
		Title:       a.Title,
		Summary:     a.Summary,
		Tags:        a.Tags,
		Embedding:   emb,
		PublishedAt: a.PublishedAt,
	}
	return s.articleRepo.Upsert(ctx, article)
}
