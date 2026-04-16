package tfidf

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// TopKeywords は docs のコーパスから TF-IDF スコアが高い上位 n 件のキーワードを返す。
// docs が少ない（10件未満）場合は nil を返す。
func TopKeywords(docs []string, n int) []string {
	if len(docs) < 10 || n == 0 {
		return nil
	}

	tokenized := make([][]string, len(docs))
	for i, doc := range docs {
		tokenized[i] = tokenize(doc)
	}

	// DF: 各トークンが何件のドキュメントに登場するか
	df := map[string]int{}
	for _, tokens := range tokenized {
		seen := map[string]bool{}
		for _, t := range tokens {
			if !seen[t] {
				df[t]++
				seen[t] = true
			}
		}
	}

	numDocs := float64(len(docs))

	// 各トークンの平均 TF-IDF を計算
	scores := map[string]float64{}
	counts := map[string]int{}
	for _, tokens := range tokenized {
		if len(tokens) == 0 {
			continue
		}
		tf := map[string]float64{}
		for _, t := range tokens {
			tf[t]++
		}
		for t, freq := range tf {
			tfScore := freq / float64(len(tokens))
			idf := math.Log(numDocs / float64(df[t]))
			scores[t] += tfScore * idf
			counts[t]++
		}
	}

	// 複数ドキュメントに出現するトークンのみを対象にスコア降順ソート
	type kv struct {
		key   string
		score float64
	}
	ranked := make([]kv, 0, len(scores))
	for t, score := range scores {
		if df[t] < 2 {
			continue
		}
		ranked = append(ranked, kv{t, score / float64(counts[t])})
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})

	if n > len(ranked) {
		n = len(ranked)
	}
	result := make([]string, n)
	for i := range result {
		result[i] = ranked[i].key
	}
	return result
}

// tokenize はテキストをトークンに分割する。
// ASCII英数字と漢字・カタカナの連続をそれぞれトークンとして扱う。
// ひらがなは助詞・助動詞が主体のため除外する。
func tokenize(text string) []string {
	var tokens []string
	var buf []rune
	currentType := 0 // 1=ASCII英数字, 2=漢字・カタカナ

	flush := func() {
		if len(buf) >= 2 && !isAllDigits(buf) {
			tokens = append(tokens, strings.ToLower(string(buf)))
		}
		buf = buf[:0]
	}

	for _, r := range text {
		var t int
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			t = 1
		case unicode.In(r, unicode.Han, unicode.Katakana):
			t = 2
		default:
			t = 0
		}

		if t == 0 {
			flush()
			currentType = 0
			continue
		}
		if t != currentType && currentType != 0 {
			flush()
		}
		buf = append(buf, r)
		currentType = t
	}
	flush()
	return tokens
}

func isAllDigits(runes []rune) bool {
	for _, r := range runes {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
