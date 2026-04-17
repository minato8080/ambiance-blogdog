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
// ブックマーク数ランダム検索と日付範囲ランダム検索の2種類を実行する。
type Historical struct {
	blogRepo         *repository.BlogRepository
	platformID       string
	httpClient       *http.Client
	dateFrom         time.Time
	dateTo           time.Time
	bookmarkMax      int
	dateWindowDays   int
	dateUsersMax     int
}

func NewHistorical(
	blogRepo *repository.BlogRepository,
	platformID string,
	dateFrom, dateTo time.Time,
	bookmarkMax, dateWindowDays, dateUsersMax int,
) *Historical {
	return &Historical{
		blogRepo:       blogRepo,
		platformID:     platformID,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		dateFrom:       dateFrom,
		dateTo:         dateTo,
		bookmarkMax:    bookmarkMax,
		dateWindowDays: dateWindowDays,
		dateUsersMax:   dateUsersMax,
	}
}

// Run はブックマーク数検索と日付範囲検索を順に実行する。
func (h *Historical) Run(ctx context.Context) error {
	if err := h.crawlByBookmarkCount(ctx); err != nil {
		slog.Warn("historical: bookmark search failed", "error", err)
	}
	// robots.txt 遵守: 1秒以上のインターバル
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Second):
	}
	if err := h.crawlByDateRange(ctx); err != nil {
		slog.Warn("historical: date range search failed", "error", err)
	}
	return nil
}

// crawlByBookmarkCount はランダムなブックマーク数で検索する。
func (h *Historical) crawlByBookmarkCount(ctx context.Context) error {
	users := rand.Intn(h.bookmarkMax + 1)
	srcURL := fmt.Sprintf(
		"https://b.hatena.ne.jp/q/entry?target=all&sort=recent&users=%d",
		users,
	)
	slog.Info("historical: bookmark count search", "users", users)
	return h.crawl(ctx, srcURL)
}

// crawlByDateRange はランダムな日付範囲と低ブックマーク数で検索する。
func (h *Historical) crawlByDateRange(ctx context.Context) error {
	maxBegin := h.dateTo.AddDate(0, 0, -h.dateWindowDays)
	if !maxBegin.After(h.dateFrom) {
		maxBegin = h.dateFrom
	}
	begin := randomDateBetween(h.dateFrom, maxBegin)
	end := begin.AddDate(0, 0, h.dateWindowDays)
	users := rand.Intn(h.dateUsersMax + 1)
	srcURL := fmt.Sprintf(
		"https://b.hatena.ne.jp/q/entry?target=all&sort=recent&users=%d&safe=on&date_begin=%s&date_end=%s",
		users,
		begin.Format("2006-01-02"),
		end.Format("2006-01-02"),
	)
	slog.Info("historical: date range search",
		"begin", begin.Format("2006-01-02"),
		"end", end.Format("2006-01-02"),
		"users", users,
	)
	return h.crawl(ctx, srcURL)
}

func (h *Historical) crawl(ctx context.Context, srcURL string) error {
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
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, srcURL)
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
			PlatformID:   h.platformID,
			BlogURL:      blogURL,
			Name:         blogName,
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
