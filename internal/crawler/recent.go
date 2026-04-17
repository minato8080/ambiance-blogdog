package crawler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/minato8080/ambiance-blogdog/internal/model"
	"github.com/minato8080/ambiance-blogdog/internal/repository"
	"github.com/oklog/ulid/v2"
)

const recentURL = "https://b.hatena.ne.jp/q/entry?target=all&sort=recent&users=0"

// Recent はブックマーク数0の最新エントリーを継続収集するクローラー。
type Recent struct {
	blogRepo   *repository.BlogRepository
	platformID string
	httpClient *http.Client
}

func NewRecent(blogRepo *repository.BlogRepository, platformID string) *Recent {
	return &Recent{
		blogRepo:   blogRepo,
		platformID: platformID,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (rc *Recent) Run(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, recentURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ambiance-blogdog/1.0 (+https://github.com/minato8080/ambiance-blogdog)")

	resp, err := rc.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("recent: HTTP %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return err
	}

	seen := map[string]bool{}
	doc.Find("[data-entry-url]").Each(func(_ int, s *goquery.Selection) {
		entryURL, _ := s.Attr("data-entry-url")
		blogURL := extractBlogURL(entryURL)
		if blogURL == "" || seen[blogURL] {
			return
		}
		seen[blogURL] = true

		blogName, _ := s.Attr("data-blog-name")
		blog := &model.Blog{
			ID:           ulid.Make().String(),
			PlatformID:   rc.platformID,
			BlogURL:      blogURL,
			Name:         blogName,
			Status:       model.BlogStatusPending,
			DiscoveredAt: time.Now(),
		}
		if err := rc.blogRepo.Upsert(ctx, blog); err != nil {
			slog.Warn("recent: upsert failed", "url", blogURL, "error", err)
		}
	})
	return nil
}
