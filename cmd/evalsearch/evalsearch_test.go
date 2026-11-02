package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundHalf(t *testing.T) {
	assert.Equal(t, float32(1), roundHalf(1))
	assert.Equal(t, float32(0), roundHalf(0))
	// 1 + 2^-11 はちょうど中間 → 偶数側（1）へ
	assert.Equal(t, float32(1), roundHalf(1+float32(math.Pow(2, -11))))
	// 1 + 3·2^-11 は中間 → 偶数側（1 + 2^-9）へ
	assert.Equal(t, float32(1+math.Pow(2, -9)), roundHalf(1+3*float32(math.Pow(2, -11))))
	assert.Equal(t, float32(-0.333251953125), roundHalf(-1.0/3))
	assert.True(t, math.IsInf(float64(roundHalf(70000)), 1))
	// 非正規化数域は 2^-24 刻み
	assert.Equal(t, float32(math.Pow(2, -24)), roundHalf(float32(math.Pow(2, -24))*1.2))
}

func TestTopK(t *testing.T) {
	scores := []float32{0.1, 0.9, 0.5, 0.7, 0.3}
	got := topK(len(scores), 3, func(j int) float32 { return scores[j] }, func(j int) bool { return j == 1 })
	require.Len(t, got, 3)
	assert.Equal(t, []int{3, 2, 4}, []int{got[0].Idx, got[1].Idx, got[2].Idx})
}

func TestParseRankConfig(t *testing.T) {
	c, err := parseRankConfig("dim=512,half=true,alpha=0.8,pool=50")
	require.NoError(t, err)
	assert.Equal(t, RankConfig{Dim: 512, Half: true, Alpha: 0.8, Pool: 50}, c)
	_, err = parseRankConfig("foo=1")
	assert.Error(t, err)
}

// syntheticArticles は「Embedding は記事ごとにランダム、文体はブログごとに固有」なデータを作る。
func syntheticArticles(blogs, perBlog, dim int) []Article {
	r := rand.New(rand.NewPCG(1, 2))
	endings := [][]string{{"です", "ました"}, {"だ", "である"}, {"だよね", "かな"}}
	firsts := []string{"私は", "僕は", "俺は", ""}
	tails := []string{"", "！", "笑", "…"}
	var arts []Article
	for b := 0; b < blogs; b++ {
		end := endings[b%len(endings)]
		first := firsts[(b/3)%len(firsts)]
		tail := tails[(b/12)%len(tails)]
		sentLen := 5 + (b%5)*8
		for i := 0; i < perBlog; i++ {
			var sb strings.Builder
			for s := 0; s < 6; s++ {
				sb.WriteString(first)
				sb.WriteString(strings.Repeat("あ漢カ", sentLen/3+r.IntN(2)))
				sb.WriteString(end[r.IntN(len(end))])
				sb.WriteString(tail)
				sb.WriteString("。")
			}
			emb := make([]float32, dim)
			for j := range emb {
				emb[j] = float32(r.NormFloat64())
			}
			arts = append(arts, Article{
				ID:      fmt.Sprintf("a%03d_%d", b, i),
				BlogID:  fmt.Sprintf("b%03d", b),
				URL:     fmt.Sprintf("https://b%03d.example/entry/%d", b, i),
				Title:   fmt.Sprintf("記事 %d-%d", b, i),
				Summary: sb.String(),
				Emb:     emb,
			})
		}
	}
	return arts
}

func TestDatasetRoundTrip(t *testing.T) {
	arts := syntheticArticles(3, 2, 8)
	path := filepath.Join(t.TempDir(), "d.bin")
	require.NoError(t, saveDataset(path, arts))
	got, err := loadDataset(path)
	require.NoError(t, err)
	assert.Equal(t, arts, got)
}

func TestEvalDims(t *testing.T) {
	d, err := newEvalData(syntheticArticles(40, 5, 64))
	require.NoError(t, err)
	rows, err := evalDims(d, []int{64, 8}, 10, 100, 1)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	// 同じ次元の半精度化ではほぼ結果が変わらない
	assert.Greater(t, rows[0].Recall, 0.95)
	assert.Greater(t, rows[0].Retention, 0.99)
	// 大きく削ると一致率が下がる
	assert.Less(t, rows[1].Recall, rows[0].Recall)

	_, err = evalDims(d, []int{128}, 10, 10, 1)
	assert.Error(t, err)
}

func TestEvalStyle(t *testing.T) {
	d, err := newEvalData(syntheticArticles(48, 5, 64))
	require.NoError(t, err)
	s, err := evalStyle(d, 64, 50, []float64{1, 0.5}, 10, 0, 1)
	require.NoError(t, err)
	require.Len(t, s.Rows, 4) // 現行 + alpha 2 種 + 文体のみ
	embOnly, styleOnly := s.Rows[0], s.Rows[3]
	// Embedding がランダムなので、文体のみの方が同ブログ記事を見つけられる
	assert.Greater(t, styleOnly.SameBlogHit, embOnly.SameBlogHit)
	assert.Greater(t, styleOnly.StyleSim, embOnly.StyleSim)
	assert.Greater(t, embOnly.TopicSim, styleOnly.TopicSim)
	assert.InDelta(t, 1.0, embOnly.OverlapNow, 1e-9)
}

func TestBlindRoundTrip(t *testing.T) {
	d, err := newEvalData(syntheticArticles(10, 3, 16))
	require.NoError(t, err)
	dir := t.TempDir()
	a := RankConfig{Dim: 16, Alpha: 1}
	b := RankConfig{Dim: 16, Half: true, Alpha: 0, Pool: 0}
	require.NoError(t, writeBlind(d, a, b, 3, 4, 1, dir))

	sheet, err := readCSV(filepath.Join(dir, sheetFile))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(sheet[0][0], utf8BOM))
	// 全候補に style=2, topic=1 を付ける
	for i := 1; i < len(sheet); i++ {
		sheet[i][8], sheet[i][9] = "2", "1"
	}
	require.NoError(t, writeCSV(filepath.Join(dir, sheetFile), sheet))

	rows, err := scoreBlind(filepath.Join(dir, sheetFile), filepath.Join(dir, keyFile))
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, r := range rows {
		assert.Equal(t, 4*3, r.Rated)
		assert.InDelta(t, 2.0, r.StyleMean, 1e-9)
		assert.InDelta(t, 1.0, r.TopicMean, 1e-9)
	}
}
