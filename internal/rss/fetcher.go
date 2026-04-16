package rss

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

type Article struct {
	URL         string
	Title       string
	Summary     string
	Tags        []string
	PublishedAt *time.Time
}

type Fetcher struct {
	parser *gofeed.Parser
}

func NewFetcher() *Fetcher {
	return &Fetcher{parser: gofeed.NewParser()}
}

// Fetch はフィード URL を取得・パースし、新しい順に最大 maxArticles 件の記事を返す。
func (f *Fetcher) Fetch(ctx context.Context, feedURL string, maxArticles int) ([]*Article, error) {
	feed, err := f.parser.ParseURLWithContext(feedURL, ctx)
	if err != nil {
		return nil, fmt.Errorf("rss.Fetch %s: %w", feedURL, err)
	}

	// 公開日時で降順ソート
	items := feed.Items
	sortByPublished(items)

	limit := maxArticles
	if limit > len(items) {
		limit = len(items)
	}

	articles := make([]*Article, 0, limit)
	for _, item := range items[:limit] {
		articles = append(articles, toArticle(item))
	}
	return articles, nil
}

func toArticle(item *gofeed.Item) *Article {
	summary := stripHTML(item.Content)
	if summary == "" {
		summary = stripHTML(item.Description)
	}
	// サマリーを 500 文字に制限
	if len([]rune(summary)) > 500 {
		runes := []rune(summary)
		summary = string(runes[:500])
	}

	var tags []string
	for _, c := range item.Categories {
		tags = append(tags, c)
	}

	var publishedAt *time.Time
	if item.PublishedParsed != nil {
		t := *item.PublishedParsed
		publishedAt = &t
	} else if item.UpdatedParsed != nil {
		t := *item.UpdatedParsed
		publishedAt = &t
	}

	return &Article{
		URL:         item.Link,
		Title:       item.Title,
		Summary:     summary,
		Tags:        tags,
		PublishedAt: publishedAt,
	}
}

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)
var spaceRe = regexp.MustCompile(`\s+`)

func stripHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func sortByPublished(items []*gofeed.Item) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0; j-- {
			a := items[j-1].PublishedParsed
			b := items[j].PublishedParsed
			if a == nil || (b != nil && b.After(*a)) {
				items[j-1], items[j] = items[j], items[j-1]
			} else {
				break
			}
		}
	}
}
