package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/minato8080/ambiance-blogdog/internal/embedding"
	"github.com/minato8080/ambiance-blogdog/internal/model"
	"github.com/minato8080/ambiance-blogdog/internal/repository"
	"github.com/oklog/ulid/v2"
)

const (
	defaultLimit = 5
	maxLimit     = 20
)

type SimilarHandler struct {
	articleRepo *repository.ArticleRepository
	blogRepo    *repository.BlogRepository
	embedClient *embedding.Client
	platformID  string
	httpClient  *http.Client
}

func NewSimilarHandler(
	articleRepo *repository.ArticleRepository,
	blogRepo *repository.BlogRepository,
	embedClient *embedding.Client,
	platformID string,
) *SimilarHandler {
	return &SimilarHandler{
		articleRepo: articleRepo,
		blogRepo:    blogRepo,
		embedClient: embedClient,
		platformID:  platformID,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *SimilarHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		writeError(w, http.StatusBadRequest, "INVALID_PARAMS", "url パラメータは必須です")
		return
	}
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PARAMS", "url が不正です")
		return
	}

	limit := defaultLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 {
			writeError(w, http.StatusBadRequest, "INVALID_PARAMS", "limit は正の整数で指定してください")
			return
		}
		if v > maxLimit {
			v = maxLimit
		}
		limit = v
	}

	article, sourceTitle, err := h.resolveArticle(r.Context(), rawURL)
	if err != nil {
		slog.Warn("similar: resolve failed", "url", rawURL, "error", err)
		if strings.Contains(err.Error(), "embed") {
			writeError(w, http.StatusServiceUnavailable, "EMBEDDING_UNAVAILABLE", "Embedding API が利用できません")
		} else {
			writeError(w, http.StatusUnprocessableEntity, "ARTICLE_FETCH_FAILED", "記事の取得・解析に失敗しました")
		}
		return
	}

	similars, err := h.articleRepo.SearchSimilar(r.Context(), article.Embedding, rawURL, limit)
	if err != nil {
		slog.Error("similar: search failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "類似記事検索に失敗しました")
		return
	}

	type sourceInfo struct {
		URL      string `json:"url"`
		Title    string `json:"title"`
		BlogURL  string `json:"blog_url,omitempty"`
		BlogName string `json:"blog_name,omitempty"`
	}
	type articleResp struct {
		URL         string     `json:"url"`
		Title       string     `json:"title"`
		BlogURL     string     `json:"blog_url,omitempty"`
		BlogName    string     `json:"blog_name,omitempty"`
		PublishedAt *time.Time `json:"published_at,omitempty"`
		Tags        []string   `json:"tags"`
		Similarity  float64    `json:"similarity"`
	}
	type response struct {
		Source          sourceInfo    `json:"source"`
		SimilarArticles []articleResp `json:"similar_articles"`
		Total           int           `json:"total"`
	}

	items := make([]articleResp, 0, len(similars))
	for _, s := range similars {
		tags := s.Tags
		if tags == nil {
			tags = []string{}
		}
		items = append(items, articleResp{
			URL:         s.URL,
			Title:       s.Title,
			BlogURL:     s.BlogURL,
			BlogName:    s.BlogName,
			PublishedAt: s.PublishedAt,
			Tags:        tags,
			Similarity:  s.Similarity,
		})
	}

	sourceBlogURL := extractBlogBaseURL(rawURL)
	var sourceBlogName string
	if blog, err := h.blogRepo.FindByBlogURL(r.Context(), sourceBlogURL); err == nil && blog != nil {
		sourceBlogName = blog.Name
	}

	writeJSON(w, http.StatusOK, response{
		Source: sourceInfo{
			URL:      rawURL,
			Title:    sourceTitle,
			BlogURL:  sourceBlogURL,
			BlogName: sourceBlogName,
		},
		SimilarArticles: items,
		Total:           len(items),
	})
}

// resolveArticle はインデックス済みなら既存 Embedding を使い、未登録ならオンデマンドで取得・保存する。
func (h *SimilarHandler) resolveArticle(ctx context.Context, rawURL string) (*model.Article, string, error) {
	existing, err := h.articleRepo.FindByURL(ctx, rawURL)
	if err != nil {
		return nil, "", err
	}
	if existing != nil && existing.Embedding != nil {
		return existing, existing.Title, nil
	}

	// オンデマンドフェッチ
	title, text, err := h.fetchArticleText(ctx, rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("fetch %s: %w", rawURL, err)
	}

	emb, err := h.embedClient.Embed(ctx, title+"\n"+text)
	if err != nil {
		return nil, "", fmt.Errorf("embed: %w", err)
	}

	// ブログURLを導出してブログ登録
	blogURL := extractBlogBaseURL(rawURL)
	blogID := h.ensureBlog(ctx, blogURL)

	article := &model.Article{
		ID:        ulid.Make().String(),
		BlogID:    blogID,
		URL:       rawURL,
		Title:     title,
		Summary:   truncate(text, 500),
		Tags:      []string{},
		Embedding: emb,
	}
	if err := h.articleRepo.Upsert(ctx, article); err != nil {
		slog.Warn("similar: upsert failed", "url", rawURL, "error", err)
	}
	return article, title, nil
}

func (h *SimilarHandler) fetchArticleText(ctx context.Context, rawURL string) (title, text string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "ambiance-blogdog/1.0")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB 上限
	if err != nil {
		return "", "", err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return "", "", err
	}

	title = strings.TrimSpace(doc.Find("title").First().Text())
	// はてなブログ本文セレクタ
	var textBuilder strings.Builder
	doc.Find(".entry-content, article").Each(func(_ int, s *goquery.Selection) {
		textBuilder.WriteString(s.Text())
		textBuilder.WriteString(" ")
	})
	text = strings.TrimSpace(textBuilder.String())
	if text == "" {
		text = doc.Find("body").Text()
	}
	return title, text, nil
}

func (h *SimilarHandler) ensureBlog(ctx context.Context, blogURL string) string {
	existing, err := h.blogRepo.FindByBlogURL(ctx, blogURL)
	if err != nil {
		slog.Warn("similar: findByBlogURL failed", "url", blogURL, "error", err)
	}
	if existing != nil {
		return existing.ID
	}
	id := ulid.Make().String()
	blog := &model.Blog{
		ID:           id,
		PlatformID:   h.platformID,
		BlogURL:      blogURL,
		Status:       model.BlogStatusPending,
		DiscoveredAt: time.Now(),
	}
	if err := h.blogRepo.Upsert(ctx, blog); err != nil {
		slog.Warn("similar: blog upsert failed", "url", blogURL, "error", err)
	}
	return id
}

func extractBlogBaseURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Scheme + "://" + u.Host
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "message": message})
}
