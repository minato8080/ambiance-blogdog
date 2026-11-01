package style

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func idx(name string) int {
	for i, n := range Names {
		if n == name {
			return i
		}
	}
	panic("unknown feature: " + name)
}

func TestExtract_Empty(t *testing.T) {
	f := Extract("   ")
	require.Len(t, f, Dim)
	for _, v := range f {
		assert.Zero(t, v)
	}
}

func TestExtract_DesuMasuVsDaDearu(t *testing.T) {
	polite := Extract("今日は晴れです。散歩に行きました。とても楽しかったです。")
	plain := Extract("今日は晴れだ。散歩に行くのである。楽しいだろう。")

	assert.InDelta(t, 1.0, polite[idx("desu_masu_ratio")], 1e-6)
	assert.Zero(t, polite[idx("da_dearu_ratio")])
	assert.InDelta(t, 1.0, plain[idx("da_dearu_ratio")], 1e-6)
	assert.Zero(t, plain[idx("desu_masu_ratio")])
}

func TestExtract_CasualMarkers(t *testing.T) {
	f := Extract("俺はラーメンが好きだよね！最高すぎるｗ　マジで草。")
	assert.Greater(t, f[idx("first_ore")], float32(0))
	assert.Greater(t, f[idx("exclaim_per_char")], float32(0))
	// 文は「…よね！」「最高すぎるｗ　マジで草。」の2つ。文末の「草」だけを数える
	assert.InDelta(t, 0.5, f[idx("laugh_per_sentence")], 1e-6)
	assert.Greater(t, f[idx("long_vowel_ratio")], float32(0))
}

func TestExtract_CharRatiosSumToAtMostOne(t *testing.T) {
	f := Extract("Goで作るAPIサーバー入門。2024年版です。")
	sum := f[idx("hiragana_ratio")] + f[idx("katakana_ratio")] + f[idx("kanji_ratio")] +
		f[idx("latin_ratio")] + f[idx("digit_ratio")]
	assert.LessOrEqual(t, sum, float32(1.0001))
	assert.Greater(t, f[idx("latin_ratio")], float32(0))
	assert.Greater(t, f[idx("digit_ratio")], float32(0))
}

func TestSplitSentences(t *testing.T) {
	got := splitSentences("本当に？！すごい。「よし。」と言った")
	assert.Equal(t, []string{"本当に？！", "すごい。", "「よし。」", "と言った"}, got)
}

func TestStandardizer(t *testing.T) {
	a := make([]float32, Dim)
	b := make([]float32, Dim)
	a[0], b[0] = 1, 3
	s := Fit([][]float32{a, b})
	assert.InDelta(t, 2.0, s.Mean[0], 1e-9)
	assert.InDelta(t, 1.0, s.Std[0], 1e-9)
	z := s.Transform(b)
	assert.InDelta(t, 1.0, z[0], 1e-6)
	assert.Zero(t, z[1]) // 分散 0 の次元
	assert.False(t, math.IsNaN(float64(z[1])))
}
