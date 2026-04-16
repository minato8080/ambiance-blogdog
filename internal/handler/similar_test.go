package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// similar_test は httptest を使った統合テスト。
// 実際の DB + OpenAI API が必要なものは integration build tag 付きの別ファイルで管理する。

func TestSimilarHandler_MissingURL(t *testing.T) {
	srv := buildTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/similar", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "INVALID_PARAMS", body["code"])
}

func TestSimilarHandler_InvalidURL(t *testing.T) {
	srv := buildTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/similar?url=not-a-url", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSimilarHandler_InvalidLimit(t *testing.T) {
	srv := buildTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/similar?url=https://example.com/&limit=abc", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
