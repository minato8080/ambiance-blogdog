package tfidf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTopKeywords_TooFewDocs(t *testing.T) {
	assert.Nil(t, TopKeywords(make([]string, 9), 5))
}

func TestTopKeywords_ZeroN(t *testing.T) {
	assert.Nil(t, TopKeywords(make([]string, 20), 0))
}

func TestTopKeywords_ReturnsTopN(t *testing.T) {
	docs := make([]string, 20)
	for i := range docs {
		docs[i] = "機械学習 解説 記事"
		if i < 10 {
			docs[i] += " Go言語 プログラミング"
		}
	}
	result := TopKeywords(docs, 2)
	assert.Len(t, result, 2)
}

func TestTopKeywords_ExcludesSingleDocKeyword(t *testing.T) {
	docs := make([]string, 10)
	for i := range docs {
		docs[i] = "共通ワード テスト記事"
	}
	docs[0] = "唯一無二の内容"

	result := TopKeywords(docs, 5)
	for _, kw := range result {
		assert.NotEqual(t, "唯一無二", kw)
	}
}

func TestTopKeywords_LimitsClamped(t *testing.T) {
	docs := make([]string, 10)
	for i := range docs {
		docs[i] = "共通単語 テスト"
	}
	result := TopKeywords(docs, 100)
	assert.LessOrEqual(t, len(result), len(docs))
}

func TestTokenize_ASCIILowercased(t *testing.T) {
	tokens := tokenize("Hello World Golang")
	assert.Equal(t, []string{"hello", "world", "golang"}, tokens)
}

func TestTokenize_KanjiAndKatakana(t *testing.T) {
	tokens := tokenize("機械学習のアルゴリズム")
	assert.Equal(t, []string{"機械学習", "アルゴリズム"}, tokens)
}

func TestTokenize_HiraganaExcluded(t *testing.T) {
	tokens := tokenize("これはテスト")
	for _, tok := range tokens {
		assert.NotContains(t, tok, "これ")
		assert.NotContains(t, tok, "は")
	}
}

func TestTokenize_ShortTokenExcluded(t *testing.T) {
	// 1文字トークンは除外される
	tokens := tokenize("A BB CCC")
	for _, tok := range tokens {
		assert.NotEqual(t, "a", tok)
	}
	assert.Contains(t, tokens, "bb")
	assert.Contains(t, tokens, "ccc")
}

func TestTokenize_DigitsOnlyExcluded(t *testing.T) {
	tokens := tokenize("2024年")
	for _, tok := range tokens {
		assert.NotEqual(t, "2024", tok)
	}
}

func TestIsAllDigits_True(t *testing.T) {
	assert.True(t, isAllDigits([]rune("1234")))
}

func TestIsAllDigits_False(t *testing.T) {
	assert.False(t, isAllDigits([]rune("12ab")))
	assert.False(t, isAllDigits([]rune("abcd")))
}
