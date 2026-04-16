package handler_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func skipWithoutDB(t *testing.T) {
	t.Helper()
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL が未設定のためスキップ")
	}
}

func TestBlogsHandler_Unauthorized(t *testing.T) {
	srv := buildTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/blogs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBlogsHandler_WithAPIKey(t *testing.T) {
	skipWithoutDB(t)
	srv := buildTestServer(t)
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		apiKey = testAPIKey
	}

	req := httptest.NewRequest(http.MethodGet, "/blogs", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	// DB がなければ 500 になるが、401 でないことを確認
	assert.NotEqual(t, http.StatusUnauthorized, w.Code)
}

func TestStatsHandler_Unauthorized(t *testing.T) {
	srv := buildTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestStatsHandler_WithAPIKey(t *testing.T) {
	skipWithoutDB(t)
	srv := buildTestServer(t)
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		apiKey = testAPIKey
	}

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusUnauthorized, w.Code)
}
