# 実装タスク

## 1. インフラ基盤

- [x] `docker-compose.yml` を作成する（PostgreSQL 15 + pgvector）
- [x] `Dockerfile` を作成する（マルチステージビルド）
- [x] `go.mod` / `go.sum` を初期化する（Go 1.22+）

## 2. DB・マイグレーション

- [x] `migrations/001_create_platforms.up.sql` を作成する
- [x] `migrations/001_create_platforms.down.sql` を作成する
- [x] `migrations/002_create_blogs.up.sql` を作成する（CHECK制約含む）
- [x] `migrations/002_create_blogs.down.sql` を作成する
- [x] `migrations/003_create_articles.up.sql` を作成する（vector型・IVFFlatインデックス含む）
- [x] `migrations/003_create_articles.down.sql` を作成する

## 3. 設定・共通

- [x] `config/config.go` を作成する（環境変数の読み込みと検証）
- [x] `internal/middleware/logger.go` を作成する（メソッド・パス・ステータス・レイテンシのログ出力）
- [x] `internal/middleware/apikey.go` を作成する（`Authorization: Bearer` ヘッダー検証）

## 4. データ層

- [x] `internal/model/platform.go` を作成する
- [x] `internal/model/blog.go` を作成する
- [x] `internal/model/article.go` を作成する
- [x] `internal/repository/blog.go` を作成する
  - [x] `Upsert`（重複スキップ）
  - [x] `FindPending`（status=pending のブログ一覧取得）
  - [x] `FindStale`（last_synced_at が staleness_days 以上経過した ready ブログ取得）
  - [x] `UpdateStatus`（status・error_count・last_synced_at 更新）
  - [x] `List`（管理用一覧取得）
- [x] `internal/repository/article.go` を作成する
  - [x] `Upsert`（記事URLをキーとして UPSERT）
  - [x] `FindByURL`（URL で記事取得）
  - [x] `SearchSimilar`（pgvector コサイン類似度検索・自身を除外）
  - [x] `CountByBlogID`（ブログ別記事数取得）
  - [x] `DeleteOldest`（上限超過時に最古記事削除）

## 5. RSS・Embedding

- [x] `internal/rss/fetcher.go` を作成する
  - [x] フィード URL 取得・パース（gofeed）
  - [x] ページネーション対応
  - [x] `MAX_ARTICLES_PER_BLOG` 件数制限の適用
- [x] `internal/embedding/openai.go` を作成する
  - [x] `text-embedding-3-small` でベクトル生成
  - [x] セマフォによる並列数制限（`CRAWL_CONCURRENCY`）
  - [x] エラー時のリトライ（指数バックオフ）

## 6. クローラー

- [x] `internal/crawler/discovery.go` を作成する（フェーズ1: ブログ発見）
  - [x] 各収集元 URL の HTML 取得・パース（goquery）
  - [x] ブログ URL 抽出ロジック（収集元別）
  - [x] ニッチキーワードのローテーション
  - [x] 1秒以上のレート制限
- [x] `internal/crawler/indexer.go` を作成する（フェーズ2: 記事インデックス構築）
  - [x] pending ブログを最大50件/回処理
  - [x] RSS 取得失敗3回で `status=error` に更新
  - [x] タイトル＋サマリーを結合してベクトル化
  - [x] `indexing` → `ready` のステータス遷移
- [x] `internal/crawler/syncer.go` を作成する（フェーズ3: 差分更新）
  - [x] `published_at` の変化で diff 判定
  - [x] 変化なし時は `last_synced_at` のみ更新
- [x] `internal/crawler/historical.go` を作成する（時間断面サンプリング）
  - [x] `CRAWL_DATE_FROM` ～ `CRAWL_DATE_TO` 内からランダム日付選択
  - [x] `b.hatena.ne.jp/entrylist?date=` を取得
- [x] `internal/crawler/scheduler.go` を作成する
  - [x] フェーズ1: 6時間ごと
  - [x] フェーズ2: 1時間ごと
  - [x] フェーズ3・時間断面: 24時間ごと

## 7. APIハンドラー

- [x] `internal/handler/similar.go` を作成する（`GET /similar`）
  - [x] `url` パラメータ検証
  - [x] インデックス済み確認 → オンデマンドフェッチ分岐
  - [x] pgvector 類似度検索・自身除外
  - [x] エラーレスポンス（400 / 422 / 503）
- [x] `internal/handler/blogs.go` を作成する（`GET /blogs`）
  - [x] APIキー認証（middleware 経由）
  - [x] ブログ一覧返却
- [x] `internal/handler/stats.go` を作成する（`GET /stats`）
  - [x] APIキー認証（middleware 経由）
  - [x] 統計情報（ブログ数・記事数・status別カウント）返却

## 8. エントリポイント

- [x] `cmd/server/main.go` を作成する
  - [x] DB 接続・マイグレーション実行
  - [x] ルーティング設定（`net/http`）
  - [x] CORS 設定（`CORS_ALLOWED_ORIGINS`）
  - [x] クローラースケジューラ起動（goroutine）
  - [x] グレースフルシャットダウン

## 9. テスト

- [x] `repository` のユニットテストを作成する（ベクトル検索・エラーハンドリング）
- [x] `handler/similar` の統合テストを作成する
- [x] `handler/blogs`, `handler/stats` の統合テストを作成する
- [x] クローラーの E2E テストを作成する（実際のはてなブログデータ使用）
