# 実装タスク

## 1. インフラ基盤

- [ ] `docker-compose.yml` を作成する（PostgreSQL 15 + pgvector）
- [ ] `Dockerfile` を作成する（マルチステージビルド）
- [ ] `go.mod` / `go.sum` を初期化する（Go 1.22+）

## 2. DB・マイグレーション

- [ ] `migrations/001_create_platforms.up.sql` を作成する
- [ ] `migrations/001_create_platforms.down.sql` を作成する
- [ ] `migrations/002_create_blogs.up.sql` を作成する（CHECK制約含む）
- [ ] `migrations/002_create_blogs.down.sql` を作成する
- [ ] `migrations/003_create_articles.up.sql` を作成する（vector型・IVFFlatインデックス含む）
- [ ] `migrations/003_create_articles.down.sql` を作成する

## 3. 設定・共通

- [ ] `config/config.go` を作成する（環境変数の読み込みと検証）
- [ ] `internal/middleware/logger.go` を作成する（メソッド・パス・ステータス・レイテンシのログ出力）
- [ ] `internal/middleware/apikey.go` を作成する（`X-API-Key` ヘッダー検証）

## 4. データ層

- [ ] `internal/model/platform.go` を作成する
- [ ] `internal/model/blog.go` を作成する
- [ ] `internal/model/article.go` を作成する
- [ ] `internal/repository/blog.go` を作成する
  - [ ] `Upsert`（重複スキップ）
  - [ ] `FindPending`（status=pending のブログ一覧取得）
  - [ ] `FindStale`（last_synced_at が staleness_days 以上経過した ready ブログ取得）
  - [ ] `UpdateStatus`（status・error_count・last_synced_at 更新）
  - [ ] `List`（管理用一覧取得）
- [ ] `internal/repository/article.go` を作成する
  - [ ] `Upsert`（記事URLをキーとして UPSERT）
  - [ ] `FindByURL`（URL で記事取得）
  - [ ] `SearchSimilar`（pgvector コサイン類似度検索・自身を除外）
  - [ ] `CountByBlogID`（ブログ別記事数取得）
  - [ ] `DeleteOldest`（上限超過時に最古記事削除）

## 5. RSS・Embedding

- [ ] `internal/rss/fetcher.go` を作成する
  - [ ] フィード URL 取得・パース（gofeed）
  - [ ] ページネーション対応
  - [ ] `MAX_ARTICLES_PER_BLOG` 件数制限の適用
- [ ] `internal/embedding/openai.go` を作成する
  - [ ] `text-embedding-3-small` でベクトル生成
  - [ ] セマフォによる並列数制限（`CRAWL_CONCURRENCY`）
  - [ ] エラー時のリトライ（指数バックオフ）

## 6. クローラー

- [ ] `internal/crawler/discovery.go` を作成する（フェーズ1: ブログ発見）
  - [ ] 各収集元 URL の HTML 取得・パース（goquery）
  - [ ] ブログ URL 抽出ロジック（収集元別）
  - [ ] ニッチキーワードのローテーション
  - [ ] 1秒以上のレート制限
- [ ] `internal/crawler/indexer.go` を作成する（フェーズ2: 記事インデックス構築）
  - [ ] pending ブログを最大50件/回処理
  - [ ] RSS 取得失敗3回で `status=error` に更新
  - [ ] タイトル＋サマリーを結合してベクトル化
  - [ ] `indexing` → `ready` のステータス遷移
- [ ] `internal/crawler/syncer.go` を作成する（フェーズ3: 差分更新）
  - [ ] `published_at` の変化で diff 判定
  - [ ] 変化なし時は `last_synced_at` のみ更新
- [ ] `internal/crawler/historical.go` を作成する（時間断面サンプリング）
  - [ ] `CRAWL_DATE_FROM` ～ `CRAWL_DATE_TO` 内からランダム日付選択
  - [ ] `b.hatena.ne.jp/entrylist?date=` を取得
- [ ] `internal/crawler/scheduler.go` を作成する
  - [ ] フェーズ1: 6時間ごと
  - [ ] フェーズ2: 1時間ごと
  - [ ] フェーズ3・時間断面: 24時間ごと

## 7. APIハンドラー

- [ ] `internal/handler/similar.go` を作成する（`GET /similar`）
  - [ ] `url` パラメータ検証
  - [ ] インデックス済み確認 → オンデマンドフェッチ分岐
  - [ ] pgvector 類似度検索・自身除外
  - [ ] エラーレスポンス（400 / 422 / 503）
- [ ] `internal/handler/blogs.go` を作成する（`GET /blogs`）
  - [ ] APIキー認証（middleware 経由）
  - [ ] ブログ一覧返却
- [ ] `internal/handler/stats.go` を作成する（`GET /stats`）
  - [ ] APIキー認証（middleware 経由）
  - [ ] 統計情報（ブログ数・記事数・status別カウント）返却

## 8. エントリポイント

- [ ] `cmd/server/main.go` を作成する
  - [ ] DB 接続・マイグレーション実行
  - [ ] ルーティング設定（`net/http`）
  - [ ] CORS 設定（`CORS_ALLOWED_ORIGINS`）
  - [ ] クローラースケジューラ起動（goroutine）
  - [ ] グレースフルシャットダウン

## 9. テスト

- [ ] `repository` のユニットテストを作成する（ベクトル検索・エラーハンドリング）
- [ ] `handler/similar` の統合テストを作成する
- [ ] `handler/blogs`, `handler/stats` の統合テストを作成する
- [ ] クローラーの E2E テストを作成する（実際のはてなブログデータ使用）
