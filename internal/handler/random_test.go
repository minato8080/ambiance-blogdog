package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/minato8080/ambiance-blogdog/internal/handler"
	"github.com/minato8080/ambiance-blogdog/internal/repository"
)

func TestRandomHandler_InvalidLimit(t *testing.T) {
	cases := []struct{ limit string }{
		{"abc"},
		{"0"},
		{"-1"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/random?limit="+tc.limit, nil)
		w := httptest.NewRecorder()
		handler.NewRandomHandler(nil, nil).ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code, "limit=%s", tc.limit)
	}
}

func TestRandomHandler_Success(t *testing.T) {
	skipWithoutDB(t)
	pool := setupTestPool(t, os.Getenv("DATABASE_URL"))
	articleRepo := repository.NewArticleRepository(pool)
	blogRepo := repository.NewBlogRepository(pool)

	h := handler.NewRandomHandler(articleRepo, blogRepo)
	req := httptest.NewRequest(http.MethodGet, "/random", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// インデックス済み記事がある場合は 200、ない場合は 503
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusServiceUnavailable)
	if w.Code == http.StatusOK {
		var body map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Contains(t, body, "source")
		assert.Contains(t, body, "similar_articles")
		assert.Contains(t, body, "total")
	}
}

func TestRandomHandler_LimitClamped(t *testing.T) {
	skipWithoutDB(t)
	pool := setupTestPool(t, os.Getenv("DATABASE_URL"))
	h := handler.NewRandomHandler(
		repository.NewArticleRepository(pool),
		repository.NewBlogRepository(pool),
	)

	req := httptest.NewRequest(http.MethodGet, "/random?limit=999", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		var body map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		total := int(body["total"].(float64))
		assert.LessOrEqual(t, total, 20)
	}
}
