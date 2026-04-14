# クローラー機能設計

## フェーズ1: ブログ発見

収集元ページを巡回し、ブログURLを抽出して `blogs` テーブルに `status=pending` でINSERT（重複はスキップ）。

- 実行間隔: **6時間ごと**

### 処理フロー

```
1. 収集元URLリストを順次取得
2. HTMLをパースし、各ページからブログURLを抽出
3. blogs テーブルに UPSERT（重複はスキップ、status=pending で登録）
4. ニッチキーワード検索の場合、次回用にキーワードをローテーション
```

### 収集元ごとの抽出方法

| 収集元                        | 抽出対象                                  |
| ----------------------------- | ----------------------------------------- |
| `hatenablog.com/`             | 記事リンクのホスト部分からブログURLを抽出 |
| `hatenablog.com/genre/...`    | 同上                                      |
| `b.hatena.ne.jp/hotentry`     | `data-entry-url` 属性からブログURLを抽出  |
| `b.hatena.ne.jp/entrylist`    | 同上                                      |
| `hatenablog.com/search?q=...` | 記事リンクのホスト部分からブログURLを抽出 |

### レート制限・エラー処理

- 各HTTPリクエスト間に1秒以上のインターバルを設ける（`robots.txt` 遵守）
- ページ取得失敗はスキップしてログ記録（次回実行時に再試行）

---

## フェーズ2: 記事インデックス構築

`status=pending` または `status=indexing` のブログを順次処理する。

```
1. {blog_url}/feed を取得（ページネーション対応）
2. 記事ごとに「タイトル + 本文サマリー」を結合
3. OpenAI Embeddings API（text-embedding-3-small）でベクトル化
4. pgvector に UPSERT（記事URLをユニークキー）
5. blogs.status を ready に更新、last_synced_at を記録
```

- 実行間隔: **1時間ごと**（pending ブログを最大50件/回処理）
- OpenAI API呼び出しは **並列数を制限**（最大5並列）してレート制限を回避
- RSS取得失敗が3回続いたブログは `status=error` にして一時停止

---

## フェーズ3: 差分更新

`status=ready` のブログのうち、`last_synced_at` から `SYNC_STALENESS_DAYS` 日以上経過したものを対象に差分チェックを行う。

- 実行間隔: **24時間ごと**（対象ブログを少量ずつ処理）
- 公開日時（`published_at`）の変化でdiff判定
- 新規・更新記事があればEmbeddingを再生成してUPSERT、`last_synced_at` を更新
- 記事に変化がなければ `status=ready` のまま、`last_synced_at` のみ更新してスキップ
- blogステータスは `ready` / `pending` / `indexing` / `error` の4値

---

## 時間断面サンプリング（過去記事カバー）

新着クロールとは別に、過去の任意日付を指定して記事を収集するクロールを並走させる。

```go
// 実行ごとに設定済み期間内からランダムに日付を選択
date := randomDateBetween(CRAWL_DATE_FROM, CRAWL_DATE_TO)
url := fmt.Sprintf("https://b.hatena.ne.jp/entrylist?date=%s", date)
```

- 実行間隔: **24時間ごと**
- 対象期間は環境変数 `CRAWL_DATE_FROM` / `CRAWL_DATE_TO` で設定
