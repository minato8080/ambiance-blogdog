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

## 10. Cloud Scheduler 3本化（discovery/historical/recent → gather統合）

クローラーを週1回稼働に変更した際、Cloud Scheduler の無料枠（3ジョブ/月）に収めるため、discovery/historical/recentを1つの`gather`フェーズに統合し、Cloud Run Jobs / Cloud Scheduler を5本→3本（gather/indexer/syncer）に削減する。

- [x] `cmd/crawler/main.go` に `CRAWLER_PHASE=gather`（discovery→historical→recentを順次実行、エラーは`errors.Join`でまとめて返す）を追加
- [x] `go build ./...` をWSLで検証（Windows側にgoコマンドが無く未検証）
- [x] `.github/workflows/deploy.yml` の crawler Jobs 更新ループを `discovery indexer syncer historical recent` → `gather indexer syncer` に変更
- [x] `docs/operations.md` の全停止・再開・個別操作コマンドのジョブ名一覧を `gather indexer syncer` に変更
- [x] `docs/design-infrastructure.md` / `docs/design-function-crawler.md` を3グループ構成（gather=discovery+historical+recent統合）に書き直す
- [x] `docs/cost-estimate.md` のCloud Scheduler試算を3ジョブ($0/月、無料枠内)に更新
- [ ] GCPインフラ移行（**破壊的操作あり・要事前確認**）
  - [x] 新コードをmasterにデプロイしてイメージ反映（2026-11-02、run 41429160334）
  - [x] `blogdog-crawler-gather` Cloud Run Job作成（2026-11-02 のデプロイでイメージ更新に成功したことで存在を確認。設定内容は未確認）
  - [ ] `blogdog-crawler-gather` 用 Cloud Scheduler（毎週日曜3:00 JST）作成
  - [ ] `blogdog-crawler-gather` を手動実行して動作確認
  - [ ] 問題なければ `blogdog-crawler-discovery` / `blogdog-crawler-historical` / `blogdog-crawler-recent` のCloud Run Jobs・Cloud Schedulerを削除
  - [ ] `gcloud scheduler jobs list` で最終的に3ジョブ(gather/indexer/syncer)になっていることを確認

## 11. Embedding の halfvec(512) 化（ストレージ削減）

`articles` の容量の約9割が `embedding vector(1536)` と IVFFlat 索引だったため、`halfvec(512)` に変換して索引を削除する。評価方法と結果は `docs/evaluation.md`、設計は `docs/design-database.md` の「Embedding の次元変更」を参照。

- [x] 評価ツール `cmd/evalsearch` を作成し、本番データで計測（DB 478MB、IVFFlat の recall@10 0.16、halfvec(512) の recall@10 0.739・類似度保持率 0.990）
- [x] マイグレーション `006_articles_embedding_halfvec` を作成（変換＋IVFFlat 削除）
- [x] Embedding 生成で `dimensions=512` を指定、リポジトリを `pgvector.HalfVector` に変更
- [x] ローカル DB（pgvector 0.8.0 / PostgreSQL 17、本番規模の合成データ 26,500件）でマイグレーションと検索を検証
  - `articles` 474MB → 39MB、006 の適用は約2秒
  - 検索は全件比較でサーバー側 約35ms（ローカル計測。Neon のコンピュートでは要再計測）、Go で計算した正解との一致率 1.000
  - 512次元の Upsert / FindByURL / FindRandom、embedding が NULL の Upsert、down → up を確認。1536次元の書き込みは `expected 512 dimensions` エラーになる
- [x] 本番適用前にバックアップを取得（`articles` の pg_dump、2026-11-02）
- [x] master へデプロイ（2026-11-02、PR #1 → develop、PR #2 → master、run 41429160334。API リビジョン `blogdog-api-00012-kcc`、indexer / syncer / gather のイメージ更新）
- [x] 適用後に `evalsearch dbcheck` で容量とレイテンシを確認（DB 478MB → 77MB、recall@10 1.000、サーバー側 約95ms）
- [ ] 文体特徴量の並べ替え（alpha=0.9 候補）を `evalsearch blind` で目視評価してから導入を判断

## 12. 類似検索の高速化と記事数上限の修正

006 適用後の本番 EXPLAIN で、全件比較のたびに TOAST 上の embedding を読んでいた（26,530 行で約10万バッファ参照）。設計は `docs/design-database.md` の「Embedding の行内保存と検索 SQL」を参照。

- [x] マイグレーション `007_articles_embedding_storage_main`（embedding を STORAGE MAIN にしてテーブルを作り直す）
- [x] `SearchSimilar` の SQL を、距離を1回だけ計算し、blogs との結合を上位件数に絞った後に行う形に変更
- [x] `FindRandom` を、id だけで並べ替えて選び、主キーで全列を取得する形に変更
- [x] ローカル（本番と同じ件数・行サイズの合成データ）で検証: 検索のサーバー側実行時間 約180〜220ms → 約26〜29ms、結果は変更前と同じ。007 の適用は数秒、down → up も確認
- [x] 記事数上限（FIFO）を indexer / syncer の両方でブログ処理の最後に適用（`ArticleRepository.TrimToNewest`）。syncer が上限を超えて記事を残していた問題を修正
- [x] `evalsearch dbcheck` が本番と同じ SQL（`repository.SearchSimilarSQL`）で計測するように変更
- [x] master へデプロイ（2026-11-02、PR #3 → develop、PR #4 → master、run 41434664308。API リビジョン `blogdog-api-00013-d86`）。`/random` の応答（日本からのクライアント計測）は約0.40〜0.46秒 → 約0.30〜0.34秒
- [ ] `evalsearch dbcheck` で本番の容量と実行時間を確認
- [x] Deploy の Firebase Hosting が 403（`serviceusage.services.get` 権限なし）で失敗する問題に対応: `firebase-tools@15.26.0`（最後に成功した版）に固定
- [ ] 固定後の master デプロイで Firebase Hosting の手順が成功することを確認
