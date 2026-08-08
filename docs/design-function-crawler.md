# クローラー機能設計

クローラーは `CRAWLER_PHASE` 環境変数で実行フェーズを切り替え、1回実行して終了する（Cloud Run Jobs モデル）。

| フェーズ | `CRAWLER_PHASE` | 役割               | Cloud Scheduler 間隔 |
| -------- | --------------- | ------------------ | -------------------- |
| 1        | `discovery`     | ブログURL発見・登録 | 毎週日曜 3:00 JST    |
| 2        | `indexer`       | 記事インデックス構築 | 毎週日曜 4:00 JST   |
| 3        | `syncer`        | 差分更新           | 毎週日曜 5:00 JST    |
| 4        | `recent`        | 新着ゼロブクマ収集 | 毎週日曜 6:00 JST    |
| 5        | `historical`    | 過去記事サンプリング | 毎週日曜 5:30 JST   |

---

## フェーズ1: ブログ発見（discovery）

収集元ページを巡回し、ブログURLを抽出して `blogs` テーブルに `status=pending` で UPSERT（重複はスキップ）。

### 処理フロー

```
1. インデックス済み記事（最大 TFIDF_SAMPLE_SIZE 件）から TF-IDF でキーワードを更新
   → 結果が空の場合はデフォルトキーワードリストを使用
   → 更新したキーワードを crawler_keywords テーブルに全件置き換え保存
2. キーワードをローテーションして収集元URLリストを生成
3. 各収集元を順次 GET（リクエスト間に1秒インターバル）
4. HTMLをパースしてブログURLを抽出
5. 抽出したブログURLごとに RSS フィード（{blog_url}/feed）の疎通確認
   → 取得成功したブログのみ blogs テーブルに UPSERT
6. 次回用にキーワードインデックスをインクリメント
```

### 収集元URL

| URL | 抽出方法 |
| --- | -------- |
| `https://hatena.blog/` | `<a href>` からブログURLを抽出 |
| `https://hatena.blog/topics/journal?sort=recent` | 同上 |
| `https://b.hatena.ne.jp/hotentry` | `data-entry-url` 属性から抽出 |
| `https://b.hatena.ne.jp/entrylist/all` | 同上 |
| `https://www.hatena.ne.jp/o/search/top?q={keyword}` | `<a href>` から抽出（キーワードはローテーション） |

### ブログURL判定ルール

以下のいずれかに該当するURLをブログURLとして認識する。

| 条件 | 例 |
| ---- | -- |
| ホストが `.hatena.blog` で終わる | `https://foo.hatena.blog` |
| ホストが `.hatenablog.jp` で終わる | `https://foo.hatenablog.jp` |
| ホストが `.hateblo.jp` で終わる | `https://foo.hateblo.jp` |
| ホストが `.hatenadiary.com` で終わる | `https://foo.hatenadiary.com` |
| ホストが `.hatenadiary.jp` で終わる | `https://foo.hatenadiary.jp` |
| パスに `/entry/` を含む（独自ドメイン） | `https://myblog.example.com/entry/...` |

### TF-IDF キーワード

- コーパス: `articles.indexed_at` 降順で最大 `TFIDF_SAMPLE_SIZE`（デフォルト: 500）件
- トークナイズ: ASCII英数字 / 漢字・カタカナの連続（ひらがなは除外）
- 抽出上限: `TFIDF_KEYWORD_COUNT`（デフォルト: 20）件
- TF-IDF 結果が空の場合のデフォルト: `実体験`, `ルポルタージュ`, `失敗談`, `雑記`, `後悔`, `旅行記`, `備忘録`

### エラー処理

- 収集元ページの取得失敗はスキップしてログ記録（次回実行時に再試行）
- RSS 疎通確認が失敗したブログは UPSERT しない

---

## フェーズ2: 記事インデックス構築（indexer）

`status=pending` のブログを `INDEX_BATCH_SIZE` 件ずつ取得し、並列で記事をベクトル化する。

### 処理フロー

```
1. status=pending のブログを最大 INDEX_BATCH_SIZE 件取得
2. CRAWL_CONCURRENCY 並列でブログを処理
   a. status を indexing に更新
   b. {blog_url}/feed を GET してRSS記事一覧を取得（最大 MAX_ARTICLES_PER_BLOG 件）
   c. 記事ごとに「タイトル + 本文サマリー」を結合してEmbedding生成
   d. articles テーブルに UPSERT（URLをユニークキー）
   e. status を ready に更新、error_count をリセット、last_synced_at を記録
```

### 記事数制限（FIFO）

```
現在の記事数 >= MAX_ARTICLES_PER_BLOG の場合:
  → published_at が最も古い記事を1件削除してから新規 UPSERT
```

### エラー処理

- RSS取得失敗時: `error_count` をインクリメントして `status=pending` に戻す
- `error_count >= INDEX_MAX_ERROR_COUNT`（デフォルト: 3）になったブログは **blogs テーブルから削除**（関連 articles も CASCADE 削除）

---

## フェーズ3: 差分更新（syncer）

`status=ready` かつ `last_synced_at` から `SYNC_STALENESS_DAYS` 日以上経過したブログを対象に差分チェックする。

### 処理フロー

```
1. 対象ブログを最大 SYNC_BATCH_SIZE 件取得（ブログ間に1秒インターバル）
2. 各ブログに対して:
   a. {blog_url}/feed を GET してRSS記事一覧を取得
   b. 各記事の published_at をDBと比較
   c. 変化あり（新規 or published_at が変わった）: Embedding 再生成して UPSERT（記事間に1秒インターバル）
   d. 変化なし: スキップ
   e. status=ready、error_count をリセット、last_synced_at を更新
```

### エラー処理

- RSS取得失敗時: `error_count` をインクリメント、`status=ready` を維持
- `error_count >= SYNC_MAX_ERROR_COUNT`（デフォルト: 3）になったブログは `status=error` に移行（検索対象から除外）

---

## フェーズ4: ブックマーク数0最新クロール（recent）

まだ注目されていない新着記事のブログを素早く収集する。

### 処理フロー

```
1. 以下の URL を GET して HTML をパース
   https://b.hatena.ne.jp/q/entry?target=all&sort=recent&users=0
2. data-entry-url 属性からブログURLを、data-blog-name 属性からブログ名を抽出
3. blogs テーブルに UPSERT（status=pending）
```

- 1回の実行でページ1件のみ取得（ページネーションなし）
- ブログ名は発見時に記録できる場合のみ保存

---

## フェーズ5: 時間断面サンプリング（historical）

新着バイアスを避け、幅広い時代・人気帯の記事からブログを収集する。1回の実行で2種類の検索を順に実行（間に1秒インターバル）。

### ブックマーク数ランダム検索

```
https://b.hatena.ne.jp/q/entry?target=all&sort=recent&users={N}
```

- `N` = `0` 〜 `HISTORICAL_BOOKMARK_MAX`（デフォルト: 200）のランダム値
- 人気・マイナー問わず幅広い記事を対象にする

### 日付範囲ランダム検索

```
https://b.hatena.ne.jp/q/entry?target=all&sort=recent&users={M}&safe=on&date_begin={begin}&date_end={end}
```

- `begin` = `CRAWL_DATE_FROM` 〜 `CRAWL_DATE_TO - HISTORICAL_DATE_WINDOW_DAYS` のランダム日付
- `end` = `begin + HISTORICAL_DATE_WINDOW_DAYS`（デフォルト: 7日）
- `M` = `0` 〜 `HISTORICAL_DATE_USERS_MAX`（デフォルト: 2）のランダム値（低ブクマ優先）

### 共通

- 両検索とも `data-entry-url` と `data-blog-name` 属性を使って blogs テーブルに UPSERT
- 取得失敗した検索はスキップしてログ記録
