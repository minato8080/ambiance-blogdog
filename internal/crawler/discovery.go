package crawler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/minato8080/ambiance-blogdog/internal/model"
	"github.com/minato8080/ambiance-blogdog/internal/repository"
	"github.com/minato8080/ambiance-blogdog/internal/tfidf"
	"github.com/oklog/ulid/v2"
)

// hatenaBlogHosts はインデックス対象とするはてなブログのホストサフィックス。
var hatenaBlogHosts = []string{
	".hatena.blog",
	".hatenablog.jp",
	".hateblo.jp",
	".hatenadiary.com",
	".hatenadiary.jp",
}

// defaultKeywords はDBが空の場合のフォールバック用キーワードリスト。
var defaultKeywords = []string{
	"実体験", "ルポルタージュ", "失敗談", "雑記", "後悔", "旅行記", "備忘録",
}

// Discoverer はフェーズ1: ブログ発見クローラー。
type Discoverer struct {
	blogRepo     *repository.BlogRepository
	articleRepo  *repository.ArticleRepository
	platformID   string
	httpClient   *http.Client
	keywordIndex int
	keywords     []string
}

func NewDiscoverer(blogRepo *repository.BlogRepository, articleRepo *repository.ArticleRepository, platformID string) *Discoverer {
	return &Discoverer{
		blogRepo:    blogRepo,
		articleRepo: articleRepo,
		platformID:  platformID,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		keywords:    defaultKeywords,
	}
}

// refreshKeywords はインデックス済み記事から TF-IDF でキーワードを更新する。
// 記事が少ない場合は defaultKeywords を維持する。
func (d *Discoverer) refreshKeywords(ctx context.Context) {
	docs, err := d.articleRepo.SampleSummaries(ctx, 500)
	if err != nil {
		slog.Warn("discovery: keyword refresh failed", "error", err)
		return
	}
	keywords := tfidf.TopKeywords(docs, 20)
	if len(keywords) > 0 {
		d.keywords = keywords
		d.keywordIndex = d.keywordIndex % len(d.keywords)
		slog.Info("discovery: keywords refreshed via TF-IDF", "count", len(keywords))
	}
}

// Run は全収集元を巡回してブログURLを発見・登録する。
func (d *Discoverer) Run(ctx context.Context) error {
	d.refreshKeywords(ctx)
	sources := d.buildSources()
	for _, srcURL := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := d.crawlSource(ctx, srcURL); err != nil {
			slog.Warn("discovery: source failed", "url", srcURL, "error", err)
		}
		// robots.txt 遵守: 1秒以上のインターバル
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	d.rotateKeyword()
	return nil
}

func (d *Discoverer) buildSources() []string {
	keyword := d.keywords[d.keywordIndex%len(d.keywords)]
	return []string{
		"https://hatena.blog/",
		"https://hatena.blog/topics/journal?sort=recent",
		"https://b.hatena.ne.jp/hotentry",
		"https://b.hatena.ne.jp/entrylist/all",
		fmt.Sprintf("https://www.hatena.ne.jp/o/search/top?q=%s", url.QueryEscape(keyword)),
	}
}

func (d *Discoverer) crawlSource(ctx context.Context, srcURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ambiance-blogdog/1.0 (+https://github.com/minato8080/ambiance-blogdog)")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return err
	}

	blogURLs := d.extractBlogURLs(srcURL, doc)
	for _, blogURL := range blogURLs {
		blog := &model.Blog{
			ID:           ulid.Make().String(),
			PlatformID:   d.platformID,
			BlogURL:      blogURL,
			Name:         "",
			Status:       model.BlogStatusPending,
			DiscoveredAt: time.Now(),
		}
		if err := d.blogRepo.Upsert(ctx, blog); err != nil {
			slog.Warn("discovery: upsert failed", "url", blogURL, "error", err)
		}
	}
	return nil
}

func (d *Discoverer) extractBlogURLs(srcURL string, doc *goquery.Document) []string {
	var blogURLs []string
	seen := map[string]bool{}

	if strings.Contains(srcURL, "b.hatena.ne.jp") {
		// data-entry-url 属性からブログURLを抽出
		doc.Find("[data-entry-url]").Each(func(_ int, s *goquery.Selection) {
			entryURL, _ := s.Attr("data-entry-url")
			if blogURL := extractBlogURL(entryURL); blogURL != "" && !seen[blogURL] {
				seen[blogURL] = true
				blogURLs = append(blogURLs, blogURL)
			}
		})
	} else {
		// 記事リンクのホスト部分からブログURLを抽出
		doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
			href, _ := s.Attr("href")
			if blogURL := extractBlogURL(href); blogURL != "" && !seen[blogURL] {
				seen[blogURL] = true
				blogURLs = append(blogURLs, blogURL)
			}
		})
	}
	return blogURLs
}

func extractBlogURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	host := u.Host
	for _, suffix := range hatenaBlogHosts {
		if strings.HasSuffix(host, suffix) {
			return u.Scheme + "://" + host
		}
	}
	// 独自ドメインのはてなブログ: /entry/ パスパターンで判定
	if strings.Contains(u.Path, "/entry/") {
		return u.Scheme + "://" + host
	}
	return ""
}

func (d *Discoverer) rotateKeyword() {
	d.keywordIndex = (d.keywordIndex + 1) % len(d.keywords)
}
