package handler

import (
	"net/http"
	"time"

	"github.com/minato8080/ambiance-blogdog/internal/repository"
)

type BlogsHandler struct {
	blogRepo *repository.BlogRepository
}

func NewBlogsHandler(blogRepo *repository.BlogRepository) *BlogsHandler {
	return &BlogsHandler{blogRepo: blogRepo}
}

func (h *BlogsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	blogs, err := h.blogRepo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "ブログ一覧の取得に失敗しました")
		return
	}

	type blogResp struct {
		ID           string     `json:"id"`
		BlogURL      string     `json:"blog_url"`
		Name         string     `json:"name"`
		Status       string     `json:"status"`
		ErrorCount   int        `json:"error_count"`
		LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
		DiscoveredAt time.Time  `json:"discovered_at"`
	}

	items := make([]blogResp, 0, len(blogs))
	for _, b := range blogs {
		items = append(items, blogResp{
			ID:           b.ID,
			BlogURL:      b.BlogURL,
			Name:         b.Name,
			Status:       string(b.Status),
			ErrorCount:   b.ErrorCount,
			LastSyncedAt: b.LastSyncedAt,
			DiscoveredAt: b.DiscoveredAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"blogs": items,
		"total": len(items),
	})
}
