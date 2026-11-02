package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
)

const (
	sheetFile = "blind_sheet.csv"
	keyFile   = "blind_key.csv"
	excerptN  = 120
)

// utf8BOM は Excel で CSV を開いたときの文字化けを防ぐための先頭マーク。
var utf8BOM = string(rune(0xFEFF))

var sheetHeader = []string{"query_no", "query_title", "query_url", "query_excerpt",
	"cand_no", "cand_title", "cand_url", "cand_excerpt", "style_score", "topic_score", "memo"}

// writeBlind は A/B 2 方式の上位 k 件を混ぜてシャッフルした採点シートと、
// どちらの方式の結果かを記録した答えファイルを outDir に書き出す。
func writeBlind(d *evalData, a, b RankConfig, k, nq int, seed uint64, outDir string) error {
	ra, err := d.ranker(a)
	if err != nil {
		return fmt.Errorf("A: %w", err)
	}
	rb, err := d.ranker(b)
	if err != nil {
		return fmt.Errorf("B: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	queries := sampleIndices(allIndices(len(d.arts)), nq, seed)
	rng := rand.New(rand.NewPCG(seed+1, seed^0x5bd1e995))

	var sheet, key [][]string
	sheet = append(sheet, sheetHeader)
	key = append(key, []string{"query_no", "cand_no", "rank_a", "rank_b", "config_a", "config_b"})
	for qi, q := range queries {
		rankA := map[int]int{}
		rankB := map[int]int{}
		var cands []int
		for i, h := range ra.Search(q, k, d.skipSameBlog(q)) {
			rankA[h.Idx] = i + 1
			cands = append(cands, h.Idx)
		}
		for i, h := range rb.Search(q, k, d.skipSameBlog(q)) {
			rankB[h.Idx] = i + 1
			if _, dup := rankA[h.Idx]; !dup {
				cands = append(cands, h.Idx)
			}
		}
		rng.Shuffle(len(cands), func(i, j int) { cands[i], cands[j] = cands[j], cands[i] })

		qa := d.arts[q]
		for ci, c := range cands {
			ca := d.arts[c]
			qno, cno := strconv.Itoa(qi+1), strconv.Itoa(ci+1)
			sheet = append(sheet, []string{qno, qa.Title, qa.URL, excerpt(qa.Summary),
				cno, ca.Title, ca.URL, excerpt(ca.Summary), "", "", ""})
			key = append(key, []string{qno, cno, strconv.Itoa(rankA[c]), strconv.Itoa(rankB[c]), a.String(), b.String()})
		}
	}
	if err := writeCSV(filepath.Join(outDir, sheetFile), sheet); err != nil {
		return err
	}
	return writeCSV(filepath.Join(outDir, keyFile), key)
}

type blindScore struct {
	Label                string
	Rated                int
	StyleMean, TopicMean float64
}

// scoreBlind は採点済みシートと答えファイルを突き合わせ、方式ごとの平均点を返す。
func scoreBlind(sheetPath, keyPath string) ([]blindScore, error) {
	sheet, err := readCSV(sheetPath)
	if err != nil {
		return nil, err
	}
	key, err := readCSV(keyPath)
	if err != nil {
		return nil, err
	}
	if len(sheet) < 2 || len(key) < 2 {
		return nil, fmt.Errorf("シートまたは答えファイルが空です")
	}
	col := func(header []string, name string) (int, error) {
		for i, h := range header {
			if strings.TrimPrefix(h, utf8BOM) == name {
				return i, nil
			}
		}
		return 0, fmt.Errorf("列 %q がありません", name)
	}
	type score struct {
		style, topic float64
		ok           bool
	}
	scores := map[string]score{}
	iq, _ := col(sheet[0], "query_no")
	ic, err := col(sheet[0], "cand_no")
	if err != nil {
		return nil, err
	}
	is, err := col(sheet[0], "style_score")
	if err != nil {
		return nil, err
	}
	it, err := col(sheet[0], "topic_score")
	if err != nil {
		return nil, err
	}
	need := max(iq, ic, is, it)
	for lineNo, row := range sheet[1:] {
		if len(row) <= need {
			continue
		}
		sStr, tStr := strings.TrimSpace(row[is]), strings.TrimSpace(row[it])
		if sStr == "" && tStr == "" {
			continue
		}
		sv, err1 := strconv.ParseFloat(sStr, 64)
		tv, err2 := strconv.ParseFloat(tStr, 64)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("シート %d 行目: style_score と topic_score は両方とも数値で入力してください", lineNo+2)
		}
		scores[row[iq]+"/"+row[ic]] = score{sv, tv, true}
	}

	kq, _ := col(key[0], "query_no")
	kc, _ := col(key[0], "cand_no")
	ka, _ := col(key[0], "rank_a")
	kb, _ := col(key[0], "rank_b")
	kca, _ := col(key[0], "config_a")
	kcb, _ := col(key[0], "config_b")
	res := []blindScore{{Label: "A: " + key[1][kca]}, {Label: "B: " + key[1][kcb]}}
	for _, row := range key[1:] {
		s, ok := scores[row[kq]+"/"+row[kc]]
		if !ok {
			continue
		}
		for i, rankCol := range []int{ka, kb} {
			if row[rankCol] != "0" {
				res[i].Rated++
				res[i].StyleMean += s.style
				res[i].TopicMean += s.topic
			}
		}
	}
	for i := range res {
		if res[i].Rated > 0 {
			res[i].StyleMean /= float64(res[i].Rated)
			res[i].TopicMean /= float64(res[i].Rated)
		}
	}
	return res, nil
}

func printBlindScore(w io.Writer, rows []blindScore) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "方式\t採点済み件数\t文体・雰囲気(平均)\t主題(平均)")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%d\t%.3f\t%.3f\n", r.Label, r.Rated, r.StyleMean, r.TopicMean)
	}
	tw.Flush()
}

func excerpt(s string) string {
	r := []rune(s)
	if len(r) > excerptN {
		return string(r[:excerptN]) + "…"
	}
	return s
}

// writeCSV は Excel で文字化けしないよう UTF-8 BOM 付きで書き出す。
func writeCSV(path string, rows [][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(utf8BOM); err != nil {
		return err
	}
	w := csv.NewWriter(f)
	if err := w.WriteAll(rows); err != nil {
		return err
	}
	return f.Close()
}

func readCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	return r.ReadAll()
}
