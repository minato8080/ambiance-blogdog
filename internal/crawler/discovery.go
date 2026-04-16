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
	"github.com/oklog/ulid/v2"
)

// hatenaBlogHosts はインデックス対象とするはてなブログのホストサフィックス。
var hatenaBlogHosts = []string{
	".hatenablog.com",
	".hatenablog.jp",
	".hateblo.jp",
	".hatenadiary.com",
	".hatenadiary.jp",
}

// nicheKeywords はニッチキーワード検索用リスト。
var nicheKeywords = []string{
	"golang", "rust", "kubernetes", "競技プログラミング", "機械学習",
	"読書", "登山", "DIY", "自作PC", "農業", "写真", "植物",
}

// Discoverer はフェーズ1: ブログ発見クローラー。
type Discoverer struct {
	blogRepo     *repository.BlogRepository
	platformID   string
	httpClient   *http.Client
	keywordIndex int
}

func NewDiscoverer(blogRepo *repository.BlogRepository, platformID string) *Discoverer {
	return &Discoverer{
		blogRepo:   blogRepo,
		platformID: platformID,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Run は全収集元を巡回してブログURLを発見・登録する。
func (d *Discoverer) Run(ctx context.Context) error {
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
	keyword := nicheKeywords[d.keywordIndex%len(nicheKeywords)]
	return []string{
		"https://hatenablog.com/",
		"https://hatenablog.com/genre/technology?sort=recent",
		"https://b.hatena.ne.jp/hotentry",
		"https://b.hatena.ne.jp/entrylist",
		fmt.Sprintf("https://hatenablog.com/search?q=%s", url.QueryEscape(keyword)),
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
	return ""
}

func (d *Discoverer) rotateKeyword() {
	d.keywordIndex = (d.keywordIndex + 1) % len(nicheKeywords)
}
