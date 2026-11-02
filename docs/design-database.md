# データベース設計

## `platforms` テーブル

プラットフォーム（ブログサービス）のマスタ。初期データとして `hatenablog` を投入。

| カラム             | 型            | 説明                                              |
| ------------------ | ------------- | ------------------------------------------------- |
| `id`               | `VARCHAR(26)` | ULID（主キー）                                    |
| `slug`             | `VARCHAR(50)` | 識別子（例: `hatenablog`）（ユニーク）            |
| `name`             | `TEXT`        | 表示名（例: `はてなブログ`）                      |
| `feed_url_pattern` | `TEXT`        | RSSフィードURLのパターン（例: `{blog_url}/feed`） |
| `created_at`       | `TIMESTAMPTZ` | 登録日時                                          |

---

## `blogs` テーブル

| カラム           | 型            | 説明                                                    |
| ---------------- | ------------- | ------------------------------------------------------- |
| `id`             | `VARCHAR(26)` | ULID（主キー）                                          |
| `platform_id`    | `VARCHAR(26)` | プラットフォームID（FK → platforms.id）                 |
| `blog_url`       | `TEXT`        | ブログベースURL（ユニーク）                             |
| `name`           | `TEXT`        | ブログ名称                                              |
| `status`         | `VARCHAR(20)` | `pending` / `indexing` / `ready` / `error`（CHECK制約） |
| `error_count`    | `INT`         | 連続エラー回数（デフォルト: 0）                         |
| `last_synced_at` | `TIMESTAMPTZ` | 最終同期日時                                            |
| `discovered_at`  | `TIMESTAMPTZ` | クローラーによる発見日時                                |

```sql
CONSTRAINT chk_blog_status CHECK (status IN ('pending', 'indexing', 'ready', 'error'))
```

---

## `articles` テーブル

| カラム         | 型             | 説明                                        |
| -------------- | -------------- | ------------------------------------------- |
| `id`           | `VARCHAR(26)`  | ULID（主キー）                              |
| `blog_id`      | `VARCHAR(26)`  | ブログID（FK → blogs.id、`ON DELETE CASCADE`） |
| `url`          | `TEXT`         | 記事URL（ユニーク）                         |
| `title`        | `TEXT`         | 記事タイトル                                |
| `summary`      | `TEXT`         | 本文（HTML除去済み）の先頭500文字。API応答には含めず、TF-IDF キーワード抽出のコーパスに使用 |
| `tags`         | `TEXT[]`       | タグ一覧                                    |
| `embedding`    | `halfvec(512)` | Embeddingベクトル（text-embedding-3-small を `dimensions=512` で生成、半精度、1行あたり約1KB）。ストレージは `MAIN`（行内に保存）。入力テキストは `design-function-crawler.md` を参照 |
| `published_at` | `TIMESTAMPTZ`  | 記事公開日時                                |
| `indexed_at`   | `TIMESTAMPTZ`  | インデックス更新日時                        |

---

## `crawler_keywords` テーブル

クローラーが TF-IDF で算出した現在のニッチキーワード一覧。Discovery クローラーの `refreshKeywords` 実行のたびに全件置き換えされる。

| カラム       | 型            | 説明                         |
| ------------ | ------------- | ---------------------------- |
| `keyword`    | `TEXT`        | キーワード（主キー）         |
| `updated_at` | `TIMESTAMPTZ` | 最終更新日時                 |

---

## インデックス

| 対象                    | 種別               | 名前                         | 用途                     |
| ----------------------- | ------------------ | ---------------------------- | ------------------------ |
| `articles.url`          | ユニーク           | （制約による自動作成）       | UPSERT のキー            |
| `articles.blog_id`      | B-tree             | `idx_articles_blog_id`       | ブログ別記事絞り込み     |
| `articles.published_at` | B-tree             | `idx_articles_published_at`  | 公開日時ソート・絞り込み |
| `blogs.blog_url`        | ユニーク           | （制約による自動作成）       | 重複登録防止             |
| `blogs.blog_url`        | B-tree             | `idx_blogs_blog_url`         | ユニーク制約の索引と重複している |
| `blogs.status`          | B-tree             | `idx_blogs_status`           | ステータス別取得         |
| `blogs.platform_id`     | B-tree             | `idx_blogs_platform_id`      | プラットフォーム別取得   |

> `articles.embedding` にはベクトル索引を張らず、類似検索は全件比較（シーケンシャルスキャン）で行う。
> 以前の IVFFlat 索引 `idx_articles_embedding` は空のテーブルで作成されていたため精度が低く（全件比較に対する recall@10 が 0.16）、容量も約208MB あったため、`006_articles_embedding_halfvec` で削除した。
> 記事数が大きく増えて全件比較が遅くなった場合は、データ投入後に HNSW または IVFFlat（`lists` ≈ √行数、`probes` を調整）を作成して精度と速度を測ってから導入する。

---

## 容量と検索性能

`go run ./cmd/evalsearch dbcheck` で実測できる（`docs/evaluation.md` 参照）。いずれも `articles` 26,535件。

| 要素 | 006 適用前（2026-11-02 実測） | 006 適用後（2026-11-02 実測） |
| ---- | ----------------------------- | ----------------------------- |
| DB 全体 | 478MB | **77MB** |
| `articles` 合計（本体 / TOAST / 索引） | 466MB（39MB / 213MB / 214MB） | 66MB（38MB / 23MB / 4.7MB） |
| `articles.embedding` 平均 | 6,148B（`vector(1536)`） | 1,030B（`halfvec(512)`） |
| `articles.summary` 平均 | 1,155B | 858B |
| 類似検索のサーバー側実行時間 | 1.4ms（IVFFlat、recall@10 0.16） | 約95ms（全件比較、recall@10 1.000）※ |

※ dbcheck の計測 SQL は blogs との結合を含まない形だったため、本番の SQL ではこれより遅い可能性がある（ローカルの同規模データでは約180ms）。

## Embedding の行内保存と検索 SQL（`007_articles_embedding_storage_main`）

- halfvec の既定ストレージは `EXTERNAL`。`summary` などと合わせて行が約2KBを超えると `embedding` が TOAST に追い出され、全件比較で1行ごとに TOAST を読みに行っていた（本番の EXPLAIN で 26,530 行に対し約10万バッファ参照）。
- 007 で `embedding` を `MAIN` にしてテーブルを作り直す。行が大きい場合は先に `summary`（`EXTENDED`）が圧縮・TOAST 化され、`embedding` は行内に残る。
- あわせて `SearchSimilar` の SQL を、距離を副問い合わせで1回だけ計算し、blogs との結合を上位件数に絞った後に行う形に変更した。
- ローカル（PostgreSQL 17 + pgvector 0.8.0、本番と同じ件数・行サイズの合成データ）でのサーバー側実行時間:

| | 変更前の SQL | 変更後の SQL |
| - | - | - |
| `EXTERNAL`（006 適用後） | 約180〜220ms | 約95〜140ms |
| `MAIN`（007 適用後） | 約45〜70ms | **約26〜29ms** |

- 007 の down はストレージ指定を `EXTERNAL` に戻すだけで、既存行は移動しない。

## Embedding の次元変更（`006_articles_embedding_halfvec`）

- 既存の `vector(1536)` を `l2_normalize(subvector(embedding, 1, 512))::halfvec(512)` で変換する。text-embedding-3 系は先頭を切り詰めて正規化した値が API の `dimensions` 指定と同等のため、OpenAI の再呼び出しは不要。
- 既存データでの評価（`evalsearch dims`、クエリ500件）では、1536次元 float32 の全件比較に対して recall@10 0.739、類似度保持率 0.990。移行前の本番（IVFFlat）の recall@10 0.16 よりも正確な結果を返す。
- pgvector 0.7.0 以上が必要。
- down マイグレーションでは 1536 次元へ戻せないため `embedding` は NULL になり、全記事の Embedding 再生成が必要。
