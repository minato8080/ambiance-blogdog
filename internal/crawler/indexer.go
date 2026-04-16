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

const (
	batchSize     = 50
	maxErrorCount = 3
)

// Indexer はフェーズ2: 記事インデックス構築クローラー。
type Indexer struct {
	blogRepo    *repository.BlogRepository
	articleRepo *repository.ArticleRepository
	rssFetcher  *rss.Fetcher
	embedClient *embedding.Client
	maxArticles int
}

func NewIndexer(
	blogRepo *repository.BlogRepository,
	articleRepo *repository.ArticleRepository,
	rssFetcher *rss.Fetcher,
	embedClient *embedding.Client,
	maxArticles int,
) *Indexer {
	return &Indexer{
		blogRepo:    blogRepo,
		articleRepo: articleRepo,
		rssFetcher:  rssFetcher,
		embedClient: embedClient,
		maxArticles: maxArticles,
	}
}

// Run は pending ブログを最大 batchSize 件処理する。
func (ix *Indexer) Run(ctx context.Context) error {
	blogs, err := ix.blogRepo.FindPending(ctx, batchSize)
	if err != nil {
		return fmt.Errorf("indexer.Run: %w", err)
	}

	for _, blog := range blogs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := ix.indexBlog(ctx, blog); err != nil {
			slog.Warn("indexer: blog failed", "blog_url", blog.BlogURL, "error", err)
		}
	}
	return nil
}

func (ix *Indexer) indexBlog(ctx context.Context, blog *model.Blog) error {
	// インデックス中に更新
	now := time.Now()
	if err := ix.blogRepo.UpdateStatus(ctx, blog.ID, model.BlogStatusIndexing, blog.ErrorCount, &now); err != nil {
		return err
	}

	feedURL := blog.BlogURL + "/feed"
	articles, err := ix.rssFetcher.Fetch(ctx, feedURL, ix.maxArticles)
	if err != nil {
		blog.ErrorCount++
		status := model.BlogStatusPending
		if blog.ErrorCount >= maxErrorCount {
			status = model.BlogStatusError
			slog.Warn("indexer: blog marked as error", "blog_url", blog.BlogURL)
		}
		return ix.blogRepo.UpdateStatus(ctx, blog.ID, status, blog.ErrorCount, nil)
	}

	for _, a := range articles {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := ix.indexArticle(ctx, blog.ID, a); err != nil {
			slog.Warn("indexer: article failed", "url", a.URL, "error", err)
		}
		// レート制限
		time.Sleep(time.Second)
	}

	syncedAt := time.Now()
	return ix.blogRepo.UpdateStatus(ctx, blog.ID, model.BlogStatusReady, 0, &syncedAt)
}

func (ix *Indexer) indexArticle(ctx context.Context, blogID string, a *rss.Article) error {
	// 上限チェック: 超過分は最古記事を削除
	count, err := ix.articleRepo.CountByBlogID(ctx, blogID)
	if err != nil {
		return err
	}
	if count >= ix.maxArticles {
		if err := ix.articleRepo.DeleteOldest(ctx, blogID); err != nil {
			return err
		}
	}

	text := strings.TrimSpace(a.Title + "\n" + a.Summary)
	emb, err := ix.embedClient.Embed(ctx, text)
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
	return ix.articleRepo.Upsert(ctx, article)
}
