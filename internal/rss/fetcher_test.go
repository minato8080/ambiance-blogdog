package rss

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serveRSS(t *testing.T, xml string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, xml)
	}))
	t.Cleanup(srv.Close)
	return srv
}

const twoItemRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>テストブログ</title>
    <link>https://test.hatenablog.com</link>
    <item>
      <title>古い記事</title>
      <link>https://test.hatenablog.com/entry/old</link>
      <description>古い本文</description>
      <pubDate>Mon, 01 Jan 2024 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>新しい記事</title>
      <link>https://test.hatenablog.com/entry/new</link>
      <description>新しい本文</description>
      <pubDate>Wed, 01 Jan 2025 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

const htmlDescRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>HTML Blog</title>
    <item>
      <title>HTML記事</title>
      <link>https://test.hatenablog.com/entry/html</link>
      <description>&lt;p&gt;本文&lt;strong&gt;強調&lt;/strong&gt;テキスト&lt;/p&gt;</description>
      <pubDate>Wed, 01 Jan 2025 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

func TestFetcher_Fetch_ReturnsFeedTitle(t *testing.T) {
	srv := serveRSS(t, twoItemRSS)
	f := NewFetcher()

	feedTitle, _, err := f.Fetch(context.Background(), srv.URL, 10)
	require.NoError(t, err)
	assert.Equal(t, "テストブログ", feedTitle)
}

func TestFetcher_Fetch_SortedDescByPublishedAt(t *testing.T) {
	srv := serveRSS(t, twoItemRSS)
	f := NewFetcher()

	_, articles, err := f.Fetch(context.Background(), srv.URL, 10)
	require.NoError(t, err)
	require.Len(t, articles, 2)
	// 新しい記事が先頭
	assert.Equal(t, "新しい記事", articles[0].Title)
	assert.Equal(t, "古い記事", articles[1].Title)
}

func TestFetcher_Fetch_RespectsMaxArticles(t *testing.T) {
	srv := serveRSS(t, twoItemRSS)
	f := NewFetcher()

	_, articles, err := f.Fetch(context.Background(), srv.URL, 1)
	require.NoError(t, err)
	assert.Len(t, articles, 1)
}

func TestFetcher_Fetch_StripsHTMLFromDescription(t *testing.T) {
	srv := serveRSS(t, htmlDescRSS)
	f := NewFetcher()

	_, articles, err := f.Fetch(context.Background(), srv.URL, 10)
	require.NoError(t, err)
	require.Len(t, articles, 1)
	assert.NotContains(t, articles[0].Summary, "<p>")
	assert.NotContains(t, articles[0].Summary, "<strong>")
	assert.Contains(t, articles[0].Summary, "本文")
}

func TestFetcher_Fetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := NewFetcher()
	_, _, err := f.Fetch(context.Background(), srv.URL, 10)
	assert.Error(t, err)
}

func TestStripHTML_RemovesTags(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"<p>本文</p>", "本文"},
		{"<a href=\"url\">リンク</a>テキスト", "リンク テキスト"},
		{"plain text", "plain text"},
		{"", ""},
	}
	for _, tc := range cases {
		got := stripHTML(tc.input)
		assert.Equal(t, tc.want, got, "input: %q", tc.input)
	}
}

func TestSortByPublished_NewestFirst(t *testing.T) {
	older := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	items := []*gofeed.Item{
		{Title: "古い", PublishedParsed: &older},
		{Title: "新しい", PublishedParsed: &newer},
	}
	sortByPublished(items)
	assert.Equal(t, "新しい", items[0].Title)
	assert.Equal(t, "古い", items[1].Title)
}

func TestSortByPublished_NilDateLast(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	items := []*gofeed.Item{
		{Title: "日付なし", PublishedParsed: nil},
		{Title: "日付あり", PublishedParsed: &t1},
	}
	sortByPublished(items)
	assert.Equal(t, "日付あり", items[0].Title)
}
