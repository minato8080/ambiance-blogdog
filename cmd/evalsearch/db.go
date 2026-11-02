package main

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/minato8080/ambiance-blogdog/internal/repository"
)

// DB にはすべて BEGIN READ ONLY のトランザクション内でアクセスする。
// Neon のコネクションプーラー（トランザクションモード）経由でも SET LOCAL が効くようにするため。
func readOnly(ctx context.Context, conn *pgx.Conn, fn func(tx pgx.Tx) error) error {
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // 読み取り専用なので Rollback で終える
	return fn(tx)
}

// exportArticles は embedding を持つ全記事をデータセットとして取り出す。
func exportArticles(ctx context.Context, conn *pgx.Conn) ([]Article, error) {
	var arts []Article
	err := readOnly(ctx, conn, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, blog_id, url, title, summary, embedding::real[]
			FROM articles
			WHERE embedding IS NOT NULL
			ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a Article
			if err := rows.Scan(&a.ID, &a.BlogID, &a.URL, &a.Title, &a.Summary, &a.Emb); err != nil {
				return err
			}
			arts = append(arts, a)
		}
		return rows.Err()
	})
	return arts, err
}

// embeddingType は articles.embedding の型（例: "vector(1536)"）と、キャストに使う型名を返す。
func embeddingType(ctx context.Context, conn *pgx.Conn) (full, base string, err error) {
	err = readOnly(ctx, conn, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT format_type(atttypid, atttypmod), t.typname
			FROM pg_attribute a JOIN pg_type t ON t.oid = a.atttypid
			WHERE a.attrelid = 'articles'::regclass AND a.attname = 'embedding' AND NOT a.attisdropped`).
			Scan(&full, &base)
	})
	return full, base, err
}

// vectorLiteral は EXPLAIN 出力に埋め込まれたベクトル値を省略するための正規表現。
var vectorLiteral = regexp.MustCompile(`'\[[^\]]*\]'`)

// dbCheck は容量の内訳と、現行 IVFFlat 索引の取りこぼし率・レイテンシを表示する。
func dbCheck(ctx context.Context, conn *pgx.Conn, w io.Writer, nq, k int) error {
	if err := printSizes(ctx, conn, w); err != nil {
		return err
	}
	embFull, _, err := embeddingType(ctx, conn)
	if err != nil {
		return fmt.Errorf("embedding 列の型取得: %w", err)
	}
	// 本番の ArticleRepository.SearchSimilar と同じ SQL。$1 の型（vector / halfvec）は列から推論されるので、
	// 移行前後どちらの DB でもベクトルを文字列で渡せる。
	searchSQL := repository.SearchSimilarSQL
	fmt.Fprintf(w, "\narticles.embedding の型: %s\n", embFull)

	type sample struct{ url, blogID, emb string }
	var samples []sample
	err = readOnly(ctx, conn, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT url, blog_id, embedding::text FROM articles
			WHERE embedding IS NOT NULL ORDER BY random() LIMIT $1`, nq)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s sample
			if err := rows.Scan(&s.url, &s.blogID, &s.emb); err != nil {
				return err
			}
			samples = append(samples, s)
		}
		return rows.Err()
	})
	if err != nil {
		return fmt.Errorf("サンプル取得: %w", err)
	}
	if len(samples) == 0 {
		fmt.Fprintln(w, "embedding を持つ記事がありません")
		return nil
	}

	// 現行設定（索引を使う）と、索引を使わない全件比較を同じクエリで比べる
	modes := []struct {
		name string
		sets []string
	}{
		{"現行設定（索引があれば使う）", nil},
		{"全件比較（索引なし）", []string{"SET LOCAL enable_indexscan = off", "SET LOCAL enable_bitmapscan = off"}},
	}
	results := make([][][]string, len(modes))
	latencies := make([][]time.Duration, len(modes))
	for mi, m := range modes {
		for _, s := range samples {
			var urls []string
			start := time.Now()
			err := readOnly(ctx, conn, func(tx pgx.Tx) error {
				for _, q := range m.sets {
					if _, err := tx.Exec(ctx, q); err != nil {
						return err
					}
				}
				rows, err := tx.Query(ctx, searchSQL, s.emb, s.url, s.blogID, k)
				if err != nil {
					return err
				}
				defer rows.Close()
				for rows.Next() {
					vals, err := rows.Values()
					if err != nil {
						return err
					}
					urls = append(urls, fmt.Sprint(vals[0])) // 先頭列は記事URL
				}
				return rows.Err()
			})
			if err != nil {
				return fmt.Errorf("%s: %w", m.name, err)
			}
			latencies[mi] = append(latencies[mi], time.Since(start))
			results[mi] = append(results[mi], urls)
		}
	}

	var recallSum float64
	scored, short := 0, 0
	for i := range samples {
		exact := map[string]bool{}
		for _, u := range results[1][i] {
			exact[u] = true
		}
		hits := 0
		for _, u := range results[0][i] {
			if exact[u] {
				hits++
			}
		}
		if len(results[1][i]) > 0 {
			recallSum += float64(hits) / float64(len(results[1][i]))
			scored++
		}
		if len(results[0][i]) < k {
			short++
		}
	}

	var probes string
	_ = readOnly(ctx, conn, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT COALESCE(current_setting('ivfflat.probes', true), '(未設定=1)')`).Scan(&probes)
	})

	fmt.Fprintf(w, "\n== 現行検索の精度・速度（クエリ %d 件、k=%d、ivfflat.probes=%s）==\n", len(samples), k, probes)
	recall := "-（全件比較の結果が 0 件で比較できない）"
	if scored > 0 {
		recall = fmt.Sprintf("%.3f", recallSum/float64(scored))
	}
	fmt.Fprintf(w, "索引ありの recall@%d（全件比較を正解とする）: %s\n", k, recall)
	fmt.Fprintf(w, "索引ありで結果が %d 件未満だったクエリ: %d 件\n", k, short)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "方式\tp50\tp95\t最大\t（クライアント計測。ネットワーク往復を含む）")
	for mi, m := range modes {
		p50, p95, max := percentiles(latencies[mi])
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t\n", m.name, p50, p95, max)
	}
	tw.Flush()

	// 1 件目のクエリの実行計画（サーバー側の実行時間とバッファ読み込み量）
	for _, m := range modes {
		fmt.Fprintf(w, "\n-- EXPLAIN (ANALYZE, BUFFERS): %s\n", m.name)
		err := readOnly(ctx, conn, func(tx pgx.Tx) error {
			for _, q := range m.sets {
				if _, err := tx.Exec(ctx, q); err != nil {
					return err
				}
			}
			rows, err := tx.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+searchSQL, samples[0].emb, samples[0].url, samples[0].blogID, k)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var line string
				if err := rows.Scan(&line); err != nil {
					return err
				}
				fmt.Fprintln(w, vectorLiteral.ReplaceAllString(line, "'[…]'"))
			}
			return rows.Err()
		})
		if err != nil {
			return fmt.Errorf("EXPLAIN %s: %w", m.name, err)
		}
	}
	return nil
}

func printSizes(ctx context.Context, conn *pgx.Conn, w io.Writer) error {
	return readOnly(ctx, conn, func(tx pgx.Tx) error {
		var pgVer, vecVer string
		if err := tx.QueryRow(ctx, `
			SELECT current_setting('server_version'),
			       COALESCE((SELECT extversion FROM pg_extension WHERE extname = 'vector'), '(なし)')`).
			Scan(&pgVer, &vecVer); err != nil {
			return err
		}
		fmt.Fprintf(w, "PostgreSQL %s / pgvector %s\n", pgVer, vecVer)

		var dbSize string
		if err := tx.QueryRow(ctx, `SELECT pg_size_pretty(pg_database_size(current_database()))`).Scan(&dbSize); err != nil {
			return err
		}
		fmt.Fprintf(w, "データベース全体: %s\n\n== テーブル別 ==\n", dbSize)

		rows, err := tx.Query(ctx, `
			SELECT c.relname,
			       pg_size_pretty(pg_total_relation_size(c.oid)),
			       pg_size_pretty(pg_relation_size(c.oid)),
			       pg_size_pretty(COALESCE(pg_total_relation_size(NULLIF(c.reltoastrelid, 0)), 0)),
			       pg_size_pretty(pg_indexes_size(c.oid)),
			       c.reltuples::bigint
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE c.relkind = 'r' AND n.nspname = 'public'
			ORDER BY pg_total_relation_size(c.oid) DESC`)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "テーブル\t合計\t本体\tTOAST\t索引\t推定行数")
		for rows.Next() {
			var name, total, heap, toast, idx string
			var tuples int64
			if err := rows.Scan(&name, &total, &heap, &toast, &idx, &tuples); err != nil {
				rows.Close()
				return err
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\n", name, total, heap, toast, idx, tuples)
		}
		rows.Close()
		tw.Flush()

		fmt.Fprintln(w, "\n== 索引別 ==")
		rows, err = tx.Query(ctx, `
			SELECT indexrelname, relname, pg_size_pretty(pg_relation_size(indexrelid))
			FROM pg_stat_user_indexes
			ORDER BY pg_relation_size(indexrelid) DESC`)
		if err != nil {
			return err
		}
		tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "索引\tテーブル\tサイズ")
		for rows.Next() {
			var idx, tbl, size string
			if err := rows.Scan(&idx, &tbl, &size); err != nil {
				rows.Close()
				return err
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\n", idx, tbl, size)
		}
		rows.Close()
		tw.Flush()

		var n int64
		var embAvg, sumAvg, titleAvg float64
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*),
			       COALESCE(AVG(pg_column_size(embedding)), 0),
			       COALESCE(AVG(pg_column_size(summary)), 0),
			       COALESCE(AVG(pg_column_size(title)), 0)
			FROM articles`).Scan(&n, &embAvg, &sumAvg, &titleAvg); err != nil {
			return err
		}
		fmt.Fprintf(w, "\n== articles 列別（平均バイト × %d 行）==\n", n)
		tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "列\t平均\t合計(推定)")
		for _, c := range []struct {
			name string
			avg  float64
		}{{"embedding", embAvg}, {"summary", sumAvg}, {"title", titleAvg}} {
			fmt.Fprintf(tw, "%s\t%.0f B\t%.1f MB\n", c.name, c.avg, c.avg*float64(n)/1e6)
		}
		tw.Flush()

		var maxPerBlog, overLimit int64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(MAX(cnt), 0), COUNT(*) FILTER (WHERE cnt > 5)
			FROM (SELECT COUNT(*) AS cnt FROM articles GROUP BY blog_id) t`).Scan(&maxPerBlog, &overLimit); err != nil {
			return err
		}
		fmt.Fprintf(w, "\n1ブログあたり最大記事数: %d（5件を超えるブログ: %d 件）\n", maxPerBlog, overLimit)
		return nil
	})
}

func percentiles(d []time.Duration) (p50, p95, max time.Duration) {
	if len(d) == 0 {
		return
	}
	s := append([]time.Duration(nil), d...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	at := func(p float64) time.Duration { return s[int(p*float64(len(s)-1))].Round(time.Millisecond) }
	return at(0.5), at(0.95), s[len(s)-1].Round(time.Millisecond)
}
