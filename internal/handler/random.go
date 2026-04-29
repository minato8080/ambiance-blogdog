package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/minato8080/ambiance-blogdog/internal/repository"
)

type RandomHandler struct {
	articleRepo *repository.ArticleRepository
	blogRepo    *repository.BlogRepository
}

func NewRandomHandler(
	articleRepo *repository.ArticleRepository,
	blogRepo *repository.BlogRepository,
) *RandomHandler {
	return &RandomHandler{articleRepo: articleRepo, blogRepo: blogRepo}
}

func (h *RandomHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	article, err := h.articleRepo.FindRandom(r.Context())
	if err != nil {
		slog.Error("random: find failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "記事の取得に失敗しました")
		return
	}
	if article == nil {
		writeError(w, http.StatusServiceUnavailable, "NO_ARTICLES", "インデックス済み記事がありません")
		return
	}

	var sourceBlogURL, sourceBlogName string
	if blog, err := h.blogRepo.FindByID(r.Context(), article.BlogID); err == nil && blog != nil {
		sourceBlogURL = blog.BlogURL
		sourceBlogName = blog.Name
	}

	similars, err := h.articleRepo.SearchSimilar(r.Context(), article.Embedding, article.URL, article.BlogID, limit)
	if err != nil {
		slog.Error("random: search failed", "error", err)
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

	writeJSON(w, http.StatusOK, response{
		Source: sourceInfo{
			URL:      article.URL,
			Title:    article.Title,
			BlogURL:  sourceBlogURL,
			BlogName: sourceBlogName,
		},
		SimilarArticles: items,
		Total:           len(items),
	})
}
