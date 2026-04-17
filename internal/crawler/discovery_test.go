package crawler

import "testing"

func TestExtractBlogURL(t *testing.T) {
	cases := []struct {
		name   string
		rawURL string
		want   string
	}{
		{
			name:   "hatena blog host",
			rawURL: "https://example.hatenablog.jp/entry/2024/01/01/post",
			want:   "https://example.hatenablog.jp",
		},
		{
			name:   "custom domain entry path",
			rawURL: "https://custom-domain.example/entry/2024/01/01/post",
			want:   "https://custom-domain.example",
		},
		{
			name:   "non-entry path contains entry",
			rawURL: "https://example.com/not-an-entry/path",
			want:   "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractBlogURL(tc.rawURL)
			if got != tc.want {
				t.Fatalf("extractBlogURL(%q) = %q, want %q", tc.rawURL, got, tc.want)
			}
		})
	}
}
