// Command evalsearch は類似記事検索の改善案（次元削減・半精度化・文体特徴量）を
// 既存データで比較評価するための開発用ツール。本番 DB へは読み取り専用でのみ接続する。
//
//	go run ./cmd/evalsearch export   -out eval-data/articles.bin   # DB から記事を取り出す
//	go run ./cmd/evalsearch dbcheck                                # DB 容量と現行索引の精度・速度
//	go run ./cmd/evalsearch dims     -in eval-data/articles.bin    # 次元削減の影響
//	go run ./cmd/evalsearch style    -in eval-data/articles.bin    # 文体特徴量の効果
//	go run ./cmd/evalsearch blind    -in eval-data/articles.bin    # 目視評価用シート作成
//	go run ./cmd/evalsearch blind-score -dir eval-data/blind       # 目視評価の集計
//
// 詳しい手順は docs/evaluation.md を参照。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const defaultDataset = "eval-data/articles.bin"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: evalsearch <export|dbcheck|dims|style|blind|blind-score> [flags]")
	fmt.Fprintln(os.Stderr, "各サブコマンドのフラグは `evalsearch <cmd> -h` で確認できます。")
}

func run(cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	in := fs.String("in", defaultDataset, "評価用データセット")
	seed := fs.Uint64("seed", 42, "クエリ抽出の乱数シード")
	k := fs.Int("k", 10, "上位何件を比較するか")
	ctx := context.Background()

	switch cmd {
	case "export":
		out := fs.String("out", defaultDataset, "出力ファイル")
		dbURL := fs.String("db", "", "接続文字列（省略時は環境変数 DATABASE_URL）")
		fs.Parse(args)
		conn, err := connect(ctx, *dbURL)
		if err != nil {
			return err
		}
		defer conn.Close(ctx)
		start := time.Now()
		arts, err := exportArticles(ctx, conn)
		if err != nil {
			return fmt.Errorf("export: %w", err)
		}
		if err := saveDataset(*out, arts); err != nil {
			return fmt.Errorf("save: %w", err)
		}
		fmt.Printf("%d 件を %s に保存しました（%s）\n", len(arts), *out, time.Since(start).Round(time.Second))
		return nil

	case "dbcheck":
		dbURL := fs.String("db", "", "接続文字列（省略時は環境変数 DATABASE_URL）")
		nq := fs.Int("queries", 30, "精度・速度を測るクエリ数")
		fs.Parse(args)
		conn, err := connect(ctx, *dbURL)
		if err != nil {
			return err
		}
		defer conn.Close(ctx)
		return dbCheck(ctx, conn, os.Stdout, *nq, *k)

	case "dims":
		dims := fs.String("dims", "1536,1024,512,256", "評価する次元数（カンマ区切り、すべて halfvec として評価）")
		nq := fs.Int("queries", 500, "クエリ数（0 で全件）")
		fs.Parse(args)
		ds, err := parseInts(*dims)
		if err != nil {
			return err
		}
		d, err := loadEvalData(*in)
		if err != nil {
			return err
		}
		rows, err := evalDims(d, ds, *k, *nq, *seed)
		if err != nil {
			return err
		}
		printDims(os.Stdout, rows, *k, min(*nq, len(d.arts)), len(d.arts))
		return nil

	case "style":
		dim := fs.Int("dim", 512, "候補抽出に使う Embedding の次元数（halfvec）")
		pool := fs.Int("pool", 100, "Embedding で取り出す候補数")
		alphas := fs.String("alphas", "1,0.9,0.8,0.7,0.5", "Embedding 類似度の重み（カンマ区切り）")
		nq := fs.Int("queries", 1000, "クエリ数（0 で全件）")
		fs.Parse(args)
		as, err := parseFloats(*alphas)
		if err != nil {
			return err
		}
		d, err := loadEvalData(*in)
		if err != nil {
			return err
		}
		s, err := evalStyle(d, *dim, *pool, as, *k, *nq, *seed)
		if err != nil {
			return err
		}
		printStyle(os.Stdout, s, *k, len(d.arts))
		return nil

	case "blind":
		a := fs.String("a", "dim=1536,half=false,alpha=1", "方式A")
		b := fs.String("b", "dim=512,half=true,alpha=0.8,pool=100", "方式B")
		nq := fs.Int("queries", 30, "クエリ数")
		out := fs.String("dir", "eval-data/blind", "出力ディレクトリ")
		fs.Parse(args)
		if !flagSet(fs, "k") {
			*k = 5
		}
		ca, err := parseRankConfig(*a)
		if err != nil {
			return fmt.Errorf("-a: %w", err)
		}
		cb, err := parseRankConfig(*b)
		if err != nil {
			return fmt.Errorf("-b: %w", err)
		}
		d, err := loadEvalData(*in)
		if err != nil {
			return err
		}
		if err := writeBlind(d, ca, cb, *k, *nq, *seed, *out); err != nil {
			return err
		}
		fmt.Printf("採点シート: %s/%s\n答え: %s/%s（採点が終わるまで開かないこと）\n", *out, sheetFile, *out, keyFile)
		return nil

	case "blind-score":
		dir := fs.String("dir", "eval-data/blind", "blind の出力ディレクトリ")
		fs.Parse(args)
		rows, err := scoreBlind(*dir+"/"+sheetFile, *dir+"/"+keyFile)
		if err != nil {
			return err
		}
		printBlindScore(os.Stdout, rows)
		return nil
	}
	usage()
	return fmt.Errorf("不明なサブコマンド: %s", cmd)
}

func loadEvalData(path string) (*evalData, error) {
	arts, err := loadDataset(path)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w（先に export を実行してください）", path, err)
	}
	return newEvalData(arts)
}

// connect は読み取り専用で DB に接続する。
func connect(ctx context.Context, dbURL string) (*pgx.Conn, error) {
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		return nil, errors.New("-db か環境変数 DATABASE_URL で接続先を指定してください")
	}
	cfg, err := pgx.ParseConfig(dbURL)
	if err != nil {
		return nil, fmt.Errorf("接続文字列の解析に失敗しました: %w", err)
	}
	cfg.RuntimeParams["application_name"] = "blogdog-evalsearch"
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("DB 接続に失敗しました: %w", err)
	}
	return conn, nil
}

func flagSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func parseInts(s string) ([]int, error) {
	var out []int
	for _, p := range strings.Split(s, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, fmt.Errorf("数値ではありません: %q", p)
		}
		out = append(out, v)
	}
	return out, nil
}

func parseFloats(s string) ([]float64, error) {
	var out []float64
	for _, p := range strings.Split(s, ",") {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil || v < 0 || v > 1 {
			return nil, fmt.Errorf("0〜1 の数値で指定してください: %q", p)
		}
		out = append(out, v)
	}
	return out, nil
}
