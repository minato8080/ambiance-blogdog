package main

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/minato8080/ambiance-blogdog/internal/style"
)

// Matrix は L2 正規化済みベクトルを行優先で並べたもの。内積 = コサイン類似度。
type Matrix struct {
	Dim  int
	Data []float32
}

func (m *Matrix) Row(i int) []float32 { return m.Data[i*m.Dim : (i+1)*m.Dim] }

func (m *Matrix) Dot(i, j int) float32 { return dot(m.Row(i), m.Row(j)) }

func dot(a, b []float32) float32 {
	var s float32
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

func normalize(v []float32) {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	if s == 0 {
		return
	}
	inv := float32(1 / math.Sqrt(s))
	for i := range v {
		v[i] *= inv
	}
}

// buildEmbMatrix は先頭 dim 次元に切り詰め → 正規化 → (half なら) float16 丸め した行列を作る。
// pgvector の l2_normalize(subvector(embedding, 1, dim))::halfvec(dim) に相当する。
func buildEmbMatrix(arts []Article, dim int, half bool) *Matrix {
	m := &Matrix{Dim: dim, Data: make([]float32, len(arts)*dim)}
	for i, a := range arts {
		row := m.Row(i)
		copy(row, a.Emb[:dim])
		normalize(row)
		if half {
			for j := range row {
				row[j] = roundHalf(row[j])
			}
			normalize(row) // コサインはスケール不変なので、内積計算用に再正規化
		}
	}
	return m
}

// buildStyleMatrix は summary から文体特徴量を計算し、z-score 化 → 正規化した行列を作る。
func buildStyleMatrix(arts []Article) *Matrix {
	raw := make([][]float32, len(arts))
	for i, a := range arts {
		raw[i] = style.Extract(a.Summary)
	}
	st := style.Fit(raw)
	m := &Matrix{Dim: style.Dim, Data: make([]float32, len(arts)*style.Dim)}
	for i, f := range raw {
		row := m.Row(i)
		copy(row, st.Transform(f))
		normalize(row)
	}
	return m
}

// roundHalf は float32 を IEEE 754 半精度に丸めた値（round-to-nearest-even）を返す。
func roundHalf(f float32) float32 {
	a := math.Abs(float64(f))
	if a == 0 || math.IsNaN(a) {
		return f
	}
	if a >= 65520 {
		return float32(math.Copysign(math.Inf(1), float64(f)))
	}
	if a < 6.103515625e-05 { // 半精度の非正規化数域: 刻み 2^-24
		return float32(math.RoundToEven(float64(f)*(1<<24)) / (1 << 24))
	}
	const drop = 13 // float32 仮数 23bit → 半精度 10bit
	bits := math.Float32bits(f)
	lsb := (bits >> drop) & 1
	bits += (1 << (drop - 1)) - 1 + lsb
	bits &^= (1 << drop) - 1
	return math.Float32frombits(bits)
}

type hit struct {
	Idx   int
	Score float32
}

// topK は score(j) の上位 k 件を返す。skip(j) が true の行は除外する。
func topK(n, k int, score func(j int) float32, skip func(j int) bool) []hit {
	if k <= 0 {
		return nil
	}
	res := make([]hit, 0, k+1)
	for j := 0; j < n; j++ {
		if skip(j) {
			continue
		}
		s := score(j)
		if len(res) == k && s <= res[k-1].Score {
			continue
		}
		pos := sort.Search(len(res), func(x int) bool { return res[x].Score < s })
		res = append(res, hit{})
		copy(res[pos+1:], res[pos:])
		res[pos] = hit{Idx: j, Score: s}
		if len(res) > k {
			res = res[:k]
		}
	}
	return res
}

// RankConfig は検索方式。Pool 件を Embedding で取り出し、
// Alpha·Embedding類似度 + (1−Alpha)·文体類似度 で並べ替える。
type RankConfig struct {
	Name  string
	Dim   int
	Half  bool
	Alpha float64
	Pool  int // 0 なら全件を並べ替え対象にする
}

func (c RankConfig) String() string {
	if c.Name != "" {
		return c.Name
	}
	prec := "half"
	if !c.Half {
		prec = "float"
	}
	if c.Alpha >= 1 {
		return fmt.Sprintf("dim=%d(%s) Embeddingのみ", c.Dim, prec)
	}
	pool := strconv.Itoa(c.Pool)
	if c.Pool == 0 {
		pool = "all"
	}
	return fmt.Sprintf("dim=%d(%s) alpha=%.2f pool=%s", c.Dim, prec, c.Alpha, pool)
}

// parseRankConfig は "dim=512,alpha=0.8,pool=100,half=true" 形式を解釈する。
func parseRankConfig(s string) (RankConfig, error) {
	c := RankConfig{Dim: 1536, Half: false, Alpha: 1, Pool: 100}
	for _, kv := range strings.Split(s, ",") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return c, fmt.Errorf("不正な指定: %q", kv)
		}
		var err error
		switch k {
		case "dim":
			c.Dim, err = strconv.Atoi(v)
		case "alpha":
			c.Alpha, err = strconv.ParseFloat(v, 64)
		case "pool":
			c.Pool, err = strconv.Atoi(v)
		case "half":
			c.Half, err = strconv.ParseBool(v)
		default:
			return c, fmt.Errorf("不明なキー: %q", k)
		}
		if err != nil {
			return c, fmt.Errorf("%s: %w", kv, err)
		}
	}
	return c, nil
}

// Ranker は RankConfig に従って検索する。
type Ranker struct {
	Cfg   RankConfig
	Emb   *Matrix
	Style *Matrix // Alpha == 1 なら nil でよい
}

func (r *Ranker) Search(q, k int, skip func(j int) bool) []hit {
	n := len(r.Emb.Data) / r.Emb.Dim
	qe := r.Emb.Row(q)
	embScore := func(j int) float32 { return dot(qe, r.Emb.Row(j)) }
	if r.Cfg.Alpha >= 1 {
		return topK(n, k, embScore, skip)
	}
	alpha := float32(r.Cfg.Alpha)
	qs := r.Style.Row(q)
	mixed := func(j int) float32 {
		return alpha*dot(qe, r.Emb.Row(j)) + (1-alpha)*dot(qs, r.Style.Row(j))
	}
	if r.Cfg.Pool <= 0 {
		return topK(n, k, mixed, skip)
	}
	pool := topK(n, r.Cfg.Pool, embScore, skip)
	for i := range pool {
		pool[i].Score = mixed(pool[i].Idx)
	}
	sort.SliceStable(pool, func(a, b int) bool { return pool[a].Score > pool[b].Score })
	if len(pool) > k {
		pool = pool[:k]
	}
	return pool
}

// parallelFor は 0..n-1 を CPU 数の goroutine で処理する。
func parallelFor(n int, fn func(i int)) {
	workers := runtime.GOMAXPROCS(0)
	var wg sync.WaitGroup
	ch := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range ch {
				fn(i)
			}
		}()
	}
	for i := 0; i < n; i++ {
		ch <- i
	}
	close(ch)
	wg.Wait()
}
