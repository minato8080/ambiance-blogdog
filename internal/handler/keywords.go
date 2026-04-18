package handler

import (
	"net/http"

	"github.com/minato8080/ambiance-blogdog/internal/repository"
)

type KeywordsHandler struct {
	keywordRepo *repository.KeywordRepository
}

func NewKeywordsHandler(keywordRepo *repository.KeywordRepository) *KeywordsHandler {
	return &KeywordsHandler{keywordRepo: keywordRepo}
}

func (h *KeywordsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	keywords, err := h.keywordRepo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "キーワードの取得に失敗しました")
		return
	}
	if keywords == nil {
		keywords = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"keywords": keywords,
		"total":    len(keywords),
	})
}
