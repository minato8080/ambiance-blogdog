package crawler

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/minato8080/ambiance-blogdog/internal/model"
	"github.com/minato8080/ambiance-blogdog/internal/repository"
	"github.com/oklog/ulid/v2"
)

// Historical は時間断面サンプリングクローラー。
type Historical struct {
	blogRepo   *repository.BlogRepository
	platformID string
	httpClient *http.Client
	dateFrom   time.Time
	dateTo     time.Time
}

func NewHistorical(blogRepo *repository.BlogRepository, platformID string, dateFrom, dateTo time.Time) *Historical {
	return &Historical{
		blogRepo:   blogRepo,
		platformID: platformID,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		dateFrom:   dateFrom,
		dateTo:     dateTo,
	}
}

// Run は設定期間内のランダム日付でエントリーリストを取得する。
func (h *Historical) Run(ctx context.Context) error {
	date := randomDateBetween(h.dateFrom, h.dateTo)
	srcURL := fmt.Sprintf("https://b.hatena.ne.jp/entrylist?date=%s", date.Format("20060102"))
	slog.Info("historical: crawling", "url", srcURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ambiance-blogdog/1.0 (+https://github.com/minato8080/ambiance-blogdog)")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("historical: HTTP %d for %s", resp.StatusCode, srcURL)
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

		blog := &model.Blog{
			ID:           ulid.Make().String(),
			PlatformID:   h.platformID,
			BlogURL:      blogURL,
			Status:       model.BlogStatusPending,
			DiscoveredAt: time.Now(),
		}
		if err := h.blogRepo.Upsert(ctx, blog); err != nil {
			slog.Warn("historical: upsert failed", "url", blogURL, "error", err)
		}
	})
	return nil
}

func randomDateBetween(from, to time.Time) time.Time {
	if !to.After(from) {
		return from
	}
	delta := to.Unix() - from.Unix()
	offset := rand.Int63n(delta)
	return from.Add(time.Duration(offset) * time.Second)
}
