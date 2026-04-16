package handler

import (
	"net/http"

	"github.com/minato8080/ambiance-blogdog/internal/repository"
)

type StatsHandler struct {
	blogRepo    *repository.BlogRepository
	articleRepo *repository.ArticleRepository
}

func NewStatsHandler(blogRepo *repository.BlogRepository, articleRepo *repository.ArticleRepository) *StatsHandler {
	return &StatsHandler{blogRepo: blogRepo, articleRepo: articleRepo}
}

func (h *StatsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	blogCounts, err := h.blogRepo.CountByStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "統計情報の取得に失敗しました")
		return
	}

	articleCount, err := h.articleRepo.CountTotal(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "統計情報の取得に失敗しました")
		return
	}

	totalBlogs := 0
	for _, c := range blogCounts {
		totalBlogs += c
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"blogs": map[string]any{
			"total":    totalBlogs,
			"by_status": blogCounts,
		},
		"articles": map[string]any{
			"total": articleCount,
		},
	})
}
