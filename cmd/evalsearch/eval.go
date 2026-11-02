package main

import (
	"fmt"
	"io"
	"math/rand/v2"
	"sort"
	"text/tabwriter"
)

// evalData は評価で共通に使う前処理済みデータ。
type evalData struct {
	arts    []Article
	blog    []int   // 記事 index → ブログ番号
	groups  [][]int // ブログ番号 → 記事 index 一覧
	base    *Matrix // 1536 次元 float32（現行の本番と同じ精度）
	style   *Matrix // 必要になるまで nil
	embByID map[[2]int]*Matrix
}

func newEvalData(arts []Article) (*evalData, error) {
	if len(arts) < 2 {
		return nil, fmt.Errorf("記事が少なすぎます（%d 件）", len(arts))
	}
	d := &evalData{arts: arts, blog: make([]int, len(arts)), embByID: map[[2]int]*Matrix{}}
	ids := map[string]int{}
	for i, a := range arts {
		b, ok := ids[a.BlogID]
		if !ok {
			b = len(d.groups)
			ids[a.BlogID] = b
			d.groups = append(d.groups, nil)
		}
		d.blog[i] = b
		d.groups[b] = append(d.groups[b], i)
	}
	d.base = d.emb(len(arts[0].Emb), false)
	return d, nil
}

func (d *evalData) fullDim() int { return len(d.arts[0].Emb) }

func (d *evalData) emb(dim int, half bool) *Matrix {
	key := [2]int{dim, 0}
	if half {
		key[1] = 1
	}
	if m, ok := d.embByID[key]; ok {
		return m
	}
	m := buildEmbMatrix(d.arts, dim, half)
	d.embByID[key] = m
	return m
}

func (d *evalData) styleMatrix() *Matrix {
	if d.style == nil {
		d.style = buildStyleMatrix(d.arts)
	}
	return d.style
}

func (d *evalData) ranker(c RankConfig) (*Ranker, error) {
	if c.Dim <= 0 || c.Dim > d.fullDim() {
		return nil, fmt.Errorf("dim=%d は 1〜%d で指定してください", c.Dim, d.fullDim())
	}
	r := &Ranker{Cfg: c, Emb: d.emb(c.Dim, c.Half)}
	if c.Alpha < 1 {
		r.Style = d.styleMatrix()
	}
	return r, nil
}

func (d *evalData) skipSameBlog(q int) func(int) bool {
	return func(j int) bool { return d.blog[j] == d.blog[q] }
}

func (d *evalData) skipSelf(q int) func(int) bool {
	return func(j int) bool { return j == q }
}

func sampleIndices(pool []int, n int, seed uint64) []int {
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	out := append([]int(nil), pool...)
	r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	if n > 0 && n < len(out) {
		out = out[:n]
	}
	sort.Ints(out)
	return out
}

func allIndices(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// ---------- dims: 次元削減・半精度化で本番の検索結果がどれだけ保たれるか ----------

type dimsRow struct {
	Label     string
	Recall    float64 // 基準の上位 k 件のうち、同じ記事が上位 k 件に入った割合
	Top1      float64 // 1 位が一致した割合
	Retention float64 // 結果の「1536次元での類似度」合計 / 基準結果の合計
}

func evalDims(d *evalData, dims []int, k, nq int, seed uint64) ([]dimsRow, error) {
	queries := sampleIndices(allIndices(len(d.arts)), nq, seed)
	baseRanker := &Ranker{Cfg: RankConfig{Alpha: 1}, Emb: d.base}
	baseRes := make([][]hit, len(queries))
	parallelFor(len(queries), func(i int) {
		q := queries[i]
		baseRes[i] = baseRanker.Search(q, k, d.skipSameBlog(q))
	})

	var rows []dimsRow
	for _, dim := range dims {
		r, err := d.ranker(RankConfig{Dim: dim, Half: true, Alpha: 1})
		if err != nil {
			return nil, err
		}
		recall := make([]float64, len(queries))
		top1 := make([]float64, len(queries))
		ret := make([]float64, len(queries))
		parallelFor(len(queries), func(i int) {
			q := queries[i]
			res := r.Search(q, k, d.skipSameBlog(q))
			base := baseRes[i]
			if len(base) == 0 {
				return
			}
			inBase := map[int]bool{}
			var baseSum float64
			for _, h := range base {
				inBase[h.Idx] = true
				baseSum += float64(h.Score)
			}
			var hits int
			var sum float64
			for _, h := range res {
				if inBase[h.Idx] {
					hits++
				}
				sum += float64(d.base.Dot(q, h.Idx))
			}
			recall[i] = float64(hits) / float64(len(base))
			if len(res) > 0 && res[0].Idx == base[0].Idx {
				top1[i] = 1
			}
			if baseSum != 0 {
				ret[i] = sum / baseSum
			}
		})
		rows = append(rows, dimsRow{
			Label:     fmt.Sprintf("halfvec(%d)", dim),
			Recall:    mean(recall),
			Top1:      mean(top1),
			Retention: mean(ret),
		})
	}
	return rows, nil
}

func printDims(w io.Writer, rows []dimsRow, k, nq, n int) {
	fmt.Fprintf(w, "基準: vector(1536) float32 の全件比較（同一ブログ除外）。記事 %d 件、クエリ %d 件、k=%d\n\n", n, nq, k)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "方式\trecall@%d\t1位一致率\t類似度保持率\n", k)
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%.3f\t%.3f\t%.4f\n", r.Label, r.Recall, r.Top1, r.Retention)
	}
	tw.Flush()
}

// ---------- style: 文体特徴量を混ぜたときの効果 ----------

type styleRow struct {
	Label string
	// 同一ブログを除外しない条件（自分自身のみ除外）での著者当て
	SameBlogHit float64 // 上位 k 件に同じブログの記事が 1 件以上入った割合
	SameBlogMRR float64 // 最初の同じブログ記事の順位の逆数（k 件以内）
	HardHit     float64 // 主題が遠い同ブログ記事が上位 k 件に入った割合
	// 本番条件（同一ブログ除外）での結果の傾向
	TopicSim   float64 // 結果の 1536 次元コサイン類似度の平均（主題の近さ）
	StyleSim   float64 // 結果の文体類似度の平均
	OverlapNow float64 // 現行方式（1536 float, Embedding のみ）の上位 k 件との重複率
}

type styleSummary struct {
	Queries     int
	HardQueries int
	HardThresh  float64
	Rows        []styleRow
}

func evalStyle(d *evalData, dim, pool int, alphas []float64, k, nq int, seed uint64) (*styleSummary, error) {
	var eligible []int
	for _, g := range d.groups {
		if len(g) >= 2 {
			eligible = append(eligible, g...)
		}
	}
	if len(eligible) == 0 {
		return nil, fmt.Errorf("同じブログの記事が 2 件以上あるブログがありません")
	}
	queries := sampleIndices(eligible, nq, seed)
	styleM := d.styleMatrix()

	// 主題が遠い同ブログ記事のしきい値: 同ブログ記事ペアの 1536 次元コサイン類似度の中央値
	var pairSims []float64
	for _, g := range d.groups {
		for a := 0; a < len(g); a++ {
			for b := a + 1; b < len(g); b++ {
				pairSims = append(pairSims, float64(d.base.Dot(g[a], g[b])))
			}
		}
	}
	sort.Float64s(pairSims)
	thresh := pairSims[len(pairSims)/2]

	hard := make([]map[int]bool, len(queries))
	hardQueries := 0
	for i, q := range queries {
		for _, j := range d.groups[d.blog[q]] {
			if j != q && float64(d.base.Dot(q, j)) < thresh {
				if hard[i] == nil {
					hard[i] = map[int]bool{}
				}
				hard[i][j] = true
			}
		}
		if hard[i] != nil {
			hardQueries++
		}
	}

	current := RankConfig{Name: "現行: vector(1536) Embeddingのみ", Dim: d.fullDim(), Half: false, Alpha: 1}
	configs := []RankConfig{current}
	for _, a := range alphas {
		configs = append(configs, RankConfig{Dim: dim, Half: true, Alpha: a, Pool: pool})
	}
	configs = append(configs, RankConfig{Name: "参考: 文体のみ（全件）", Dim: dim, Half: true, Alpha: 0, Pool: 0})

	currentRanker, _ := d.ranker(current)
	nowRes := make([][]hit, len(queries))
	parallelFor(len(queries), func(i int) {
		q := queries[i]
		nowRes[i] = currentRanker.Search(q, k, d.skipSameBlog(q))
	})

	sum := &styleSummary{Queries: len(queries), HardQueries: hardQueries, HardThresh: thresh}
	for _, c := range configs {
		r, err := d.ranker(c)
		if err != nil {
			return nil, err
		}
		hitV := make([]float64, len(queries))
		mrrV := make([]float64, len(queries))
		hardV := make([]float64, len(queries))
		topicV := make([]float64, len(queries))
		styleV := make([]float64, len(queries))
		overV := make([]float64, len(queries))
		parallelFor(len(queries), func(i int) {
			q := queries[i]
			res := r.Search(q, k, d.skipSelf(q))
			for rank, h := range res {
				if d.blog[h.Idx] == d.blog[q] {
					if hitV[i] == 0 {
						hitV[i] = 1
						mrrV[i] = 1 / float64(rank+1)
					}
					if hard[i][h.Idx] {
						hardV[i] = 1
					}
				}
			}

			prod := r.Search(q, k, d.skipSameBlog(q))
			now := map[int]bool{}
			for _, h := range nowRes[i] {
				now[h.Idx] = true
			}
			var overlap int
			for _, h := range prod {
				topicV[i] += float64(d.base.Dot(q, h.Idx))
				styleV[i] += float64(styleM.Dot(q, h.Idx))
				if now[h.Idx] {
					overlap++
				}
			}
			if len(prod) > 0 {
				topicV[i] /= float64(len(prod))
				styleV[i] /= float64(len(prod))
			}
			if len(nowRes[i]) > 0 {
				overV[i] = float64(overlap) / float64(len(nowRes[i]))
			}
		})

		var hardSum float64
		for i := range queries {
			if hard[i] != nil {
				hardSum += hardV[i]
			}
		}
		row := styleRow{
			Label:       c.String(),
			SameBlogHit: mean(hitV),
			SameBlogMRR: mean(mrrV),
			TopicSim:    mean(topicV),
			StyleSim:    mean(styleV),
			OverlapNow:  mean(overV),
		}
		if hardQueries > 0 {
			row.HardHit = hardSum / float64(hardQueries)
		}
		sum.Rows = append(sum.Rows, row)
	}
	return sum, nil
}

func printStyle(w io.Writer, s *styleSummary, k, n int) {
	fmt.Fprintf(w, "記事 %d 件、クエリ %d 件（同じブログに他の記事があるもの）、k=%d\n", n, s.Queries, k)
	fmt.Fprintf(w, "主題が遠い同ブログ記事: 1536次元コサイン類似度 < %.4f（同ブログ記事ペアの中央値）、対象クエリ %d 件\n\n", s.HardThresh, s.HardQueries)

	fmt.Fprintln(w, "[A] 著者当て（自分自身のみ除外）: 文体を捉えているほど高くなる")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "方式\t同ブログHit@%d\t同ブログMRR@%d\t主題が遠い同ブログHit@%d\n", k, k, k)
	for _, r := range s.Rows {
		fmt.Fprintf(tw, "%s\t%.3f\t%.3f\t%.3f\n", r.Label, r.SameBlogHit, r.SameBlogMRR, r.HardHit)
	}
	tw.Flush()

	fmt.Fprintln(w, "\n[B] 本番条件（同一ブログ除外）での結果の傾向")
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "方式\t主題の近さ(平均)\t文体の近さ(平均)\t現行結果との重複率\n")
	for _, r := range s.Rows {
		fmt.Fprintf(tw, "%s\t%.4f\t%.4f\t%.3f\n", r.Label, r.TopicSim, r.StyleSim, r.OverlapNow)
	}
	tw.Flush()
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var s float64
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}
