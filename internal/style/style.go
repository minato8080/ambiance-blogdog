// Package style は日本語ブログ本文から文体特徴量を抽出する。
//
// 形態素解析は使わず、文字種・記号・文末表現などの出現頻度だけで
// 「文体・雰囲気」を表す低次元ベクトルを作る。
package style

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Names は Extract が返す特徴量の並び順。
var Names = []string{
	"hiragana_ratio",
	"katakana_ratio",
	"kanji_ratio",
	"latin_ratio",
	"digit_ratio",
	"long_vowel_ratio",
	"touten_per_char",
	"kuten_per_char",
	"exclaim_per_char",
	"question_per_char",
	"ellipsis_per_char",
	"bracket_per_char",
	"emoji_per_char",
	"laugh_per_sentence",
	"log_sentence_len",
	"sentence_len_cv",
	"kanji_run_len",
	"desu_masu_ratio",
	"da_dearu_ratio",
	"particle_end_ratio",
	"first_watashi",
	"first_boku",
	"first_ore",
}

// Dim は特徴量の次元数。
var Dim = len(Names)

var (
	desuMasuEnds = []string{"です", "ます", "でした", "ました", "ません", "でしょう", "ましょう", "ください"}
	daDearuEnds  = []string{"だ", "である", "だった", "ではない", "じゃない", "だろう", "であった"}
	particleEnds = []string{"ね", "よ", "な", "よね", "かな", "わ", "ぞ"}
)

// Extract は本文テキストから文体特徴量を返す。テキストが空なら全て 0。
func Extract(text string) []float32 {
	f := make([]float32, Dim)
	text = strings.TrimSpace(text)
	total := utf8.RuneCountInString(text)
	if total == 0 {
		return f
	}

	var hira, kata, kanji, latin, digit, longV, touten, kuten, excl, ques, ellip, brack, emoji int
	var kanjiRuns, kanjiRunTotal, curKanjiRun int
	for _, r := range text {
		if isKanji(r) {
			curKanjiRun++
		} else if curKanjiRun > 0 {
			kanjiRuns++
			kanjiRunTotal += curKanjiRun
			curKanjiRun = 0
		}
		switch {
		case r == 'ー':
			longV++
			kata++
		case unicode.Is(unicode.Hiragana, r):
			hira++
		case unicode.Is(unicode.Katakana, r):
			kata++
		case isKanji(r):
			kanji++
		case r < 0x80 && unicode.IsLetter(r), r >= 'Ａ' && r <= 'ｚ' && unicode.IsLetter(r):
			latin++
		case unicode.IsDigit(r):
			digit++
		case r == '、' || r == '，' || r == ',':
			touten++
		case r == '。' || r == '．':
			kuten++
		case r == '!' || r == '！':
			excl++
		case r == '?' || r == '？':
			ques++
		case r == '…' || r == '‥':
			ellip++
		case strings.ContainsRune("「」『』（）()【】", r):
			brack++
		case isEmoji(r):
			emoji++
		}
	}
	if curKanjiRun > 0 {
		kanjiRuns++
		kanjiRunTotal += curKanjiRun
	}

	sentences := splitSentences(text)
	n := float64(total)
	f[0] = float32(float64(hira) / n)
	f[1] = float32(float64(kata) / n)
	f[2] = float32(float64(kanji) / n)
	f[3] = float32(float64(latin) / n)
	f[4] = float32(float64(digit) / n)
	f[5] = float32(float64(longV) / n)
	f[6] = float32(float64(touten) / n)
	f[7] = float32(float64(kuten) / n)
	f[8] = float32(float64(excl) / n)
	f[9] = float32(float64(ques) / n)
	f[10] = float32(float64(ellip) / n)
	f[11] = float32(float64(brack) / n)
	f[12] = float32(float64(emoji) / n)
	if kanjiRuns > 0 {
		f[16] = float32(float64(kanjiRunTotal) / float64(kanjiRuns))
	}

	if len(sentences) == 0 {
		return f
	}
	ns := float64(len(sentences))
	var lenSum, lenSq float64
	var laugh, desu, da, part, watashi, boku, ore int
	for _, s := range sentences {
		l := float64(utf8.RuneCountInString(s))
		lenSum += l
		lenSq += l * l
		body := strings.TrimRight(s, "。．!！?？…‥」』)）　 ")
		if hasLaugh(body) {
			laugh++
		}
		switch {
		case hasAnySuffix(body, desuMasuEnds):
			desu++
		case hasAnySuffix(body, daDearuEnds):
			da++
		}
		if hasAnySuffix(body, particleEnds) {
			part++
		}
		if strings.Contains(s, "私") || strings.Contains(s, "わたし") {
			watashi++
		}
		if strings.Contains(s, "僕") || strings.Contains(s, "ぼく") {
			boku++
		}
		if strings.Contains(s, "俺") || strings.Contains(s, "おれ") || strings.Contains(s, "オレ") {
			ore++
		}
	}
	mean := lenSum / ns
	variance := lenSq/ns - mean*mean
	if variance < 0 {
		variance = 0
	}
	f[13] = float32(float64(laugh) / ns)
	f[14] = float32(math.Log1p(mean))
	if mean > 0 {
		f[15] = float32(math.Sqrt(variance) / mean)
	}
	f[17] = float32(float64(desu) / ns)
	f[18] = float32(float64(da) / ns)
	f[19] = float32(float64(part) / ns)
	f[20] = float32(float64(watashi) / ns)
	f[21] = float32(float64(boku) / ns)
	f[22] = float32(float64(ore) / ns)
	return f
}

// splitSentences は句点・感嘆符・疑問符で文に分割する（空文は除く）。
func splitSentences(text string) []string {
	var out []string
	var b strings.Builder
	runes := []rune(text)
	for i, r := range runes {
		b.WriteRune(r)
		closesSentence := isClosingBracket(r) && i > 0 && isSentenceEnd(runes[i-1])
		if !isSentenceEnd(r) && !closesSentence {
			continue
		}
		// 「！？」「。」」のような連続終端・閉じ括弧はまとめて同じ文に含める
		if i+1 < len(runes) && (isSentenceEnd(runes[i+1]) || isClosingBracket(runes[i+1])) {
			continue
		}
		if s := strings.TrimSpace(b.String()); s != "" {
			out = append(out, s)
		}
		b.Reset()
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		out = append(out, s)
	}
	return out
}

func isSentenceEnd(r rune) bool {
	return r == '。' || r == '．' || r == '!' || r == '！' || r == '?' || r == '？'
}

func isClosingBracket(r rune) bool {
	return r == '」' || r == '』' || r == ')' || r == '）'
}

func hasAnySuffix(s string, suffixes []string) bool {
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	return false
}

// hasLaugh は文末の「笑」「w」「草」を検出する。
func hasLaugh(s string) bool {
	s = strings.TrimRight(s, "）) ")
	return strings.HasSuffix(s, "笑") || strings.HasSuffix(s, "w") ||
		strings.HasSuffix(s, "ｗ") || strings.HasSuffix(s, "草")
}

func isKanji(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

func isEmoji(r rune) bool {
	return (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF)
}

// Standardizer はコーパス全体の平均・標準偏差で特徴量を z-score 化する。
type Standardizer struct {
	Mean []float64
	Std  []float64
}

// Fit は特徴量の集合から平均・標準偏差を計算する。
func Fit(features [][]float32) *Standardizer {
	s := &Standardizer{Mean: make([]float64, Dim), Std: make([]float64, Dim)}
	if len(features) == 0 {
		return s
	}
	n := float64(len(features))
	for _, f := range features {
		for j, v := range f {
			s.Mean[j] += float64(v)
		}
	}
	for j := range s.Mean {
		s.Mean[j] /= n
	}
	for _, f := range features {
		for j, v := range f {
			d := float64(v) - s.Mean[j]
			s.Std[j] += d * d
		}
	}
	for j := range s.Std {
		s.Std[j] = math.Sqrt(s.Std[j] / n)
	}
	return s
}

// Transform は z-score 化したベクトルを返す（標準偏差 0 の次元は 0）。
func (s *Standardizer) Transform(f []float32) []float32 {
	out := make([]float32, len(f))
	for j, v := range f {
		if s.Std[j] > 0 {
			out[j] = float32((float64(v) - s.Mean[j]) / s.Std[j])
		}
	}
	return out
}
