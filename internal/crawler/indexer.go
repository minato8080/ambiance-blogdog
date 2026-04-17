package crawler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/minato8080/ambiance-blogdog/internal/embedding"
	"github.com/minato8080/ambiance-blogdog/internal/model"
	"github.com/minato8080/ambiance-blogdog/internal/repository"
	"github.com/minato8080/ambiance-blogdog/internal/rss"
	"github.com/oklog/ulid/v2"
)

// Indexer はフェーズ2: 記事インデックス構築クローラー。
type Indexer struct {
	blogRepo      *repository.BlogRepository
	articleRepo   *repository.ArticleRepository
	rssFetcher    *rss.Fetcher
	embedClient   *embedding.Client
	maxArticles   int
	batchSize     int
	maxErrorCount int
	concurrency   int
}

func NewIndexer(
	blogRepo *repository.BlogRepository,
	articleRepo *repository.ArticleRepository,
	rssFetcher *rss.Fetcher,
	embedClient *embedding.Client,
	maxArticles, batchSize, maxErrorCount, concurrency int,
) *Indexer {
	return &Indexer{
		blogRepo:      blogRepo,
		articleRepo:   articleRepo,
		rssFetcher:    rssFetcher,
		embedClient:   embedClient,
		maxArticles:   maxArticles,
		batchSize:     batchSize,
		maxErrorCount: maxErrorCount,
		concurrency:   concurrency,
	}
}

// Run は pending ブログを最大 batchSize 件、concurrency 並列で処理する。
func (ix *Indexer) Run(ctx context.Context) error {
	blogs, err := ix.blogRepo.FindPending(ctx, ix.batchSize)
	if err != nil {
		return fmt.Errorf("indexer.Run: %w", err)
	}

	sem := make(chan struct{}, ix.concurrency)
	var wg sync.WaitGroup

	for _, blog := range blogs {
		if ctx.Err() != nil {
			break
		}
		blog := blog
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := ix.indexBlog(ctx, blog); err != nil {
				slog.Warn("indexer: blog failed", "blog_url", blog.BlogURL, "error", err)
			}
		}()
	}
	wg.Wait()
	return nil
}

func (ix *Indexer) indexBlog(ctx context.Context, blog *model.Blog) error {
	now := time.Now()
	if err := ix.blogRepo.UpdateStatus(ctx, blog.ID, model.BlogStatusIndexing, blog.ErrorCount, &now); err != nil {
		return err
	}

	feedURL := blog.BlogURL + "/feed"
	articles, err := ix.rssFetcher.Fetch(ctx, feedURL, ix.maxArticles)
	if err != nil {
		blog.ErrorCount++
		status := model.BlogStatusPending
		if blog.ErrorCount >= ix.maxErrorCount {
			status = model.BlogStatusError
			slog.Warn("indexer: blog marked as error", "blog_url", blog.BlogURL, "error", err)
		}
		return ix.blogRepo.UpdateStatus(ctx, blog.ID, status, blog.ErrorCount, nil)
	}

	for _, a := range articles {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := ix.indexArticle(ctx, blog.ID, a); err != nil {
			slog.Warn("indexer: article failed", "url", a.URL, "error", err)
		}
	}

	syncedAt := time.Now()
	return ix.blogRepo.UpdateStatus(ctx, blog.ID, model.BlogStatusReady, 0, &syncedAt)
}

func (ix *Indexer) indexArticle(ctx context.Context, blogID string, a *rss.Article) error {
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
