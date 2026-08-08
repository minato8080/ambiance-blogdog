# インフラ・実装設計

## 技術スタック

| 項目                       | 採用技術                                  |
| -------------------------- | ----------------------------------------- |
| 言語                       | Go 1.22+                                  |
| HTTPフレームワーク         | `net/http`（標準ライブラリ）              |
| DBドライバ                 | `github.com/jackc/pgx/v5`                 |
| マイグレーション           | `github.com/golang-migrate/migrate`       |
| RSSパーサー                | `github.com/mmcdole/gofeed`               |
| HTMLパーサー（クローラー） | `github.com/PuerkitoBio/goquery`          |
| OpenAI SDK                 | `github.com/sashabaranov/go-openai`       |
| ULID生成                   | `github.com/oklog/ulid/v2`                |
| テスト                     | `testing` + `github.com/stretchr/testify` |
| コンテナ                   | Docker / docker-compose                   |

---

## ディレクトリ構成

```
ambiance-blogdog/
├── cmd/
│   ├── api/
│   │   └── main.go                   # APIサーバー エントリポイント（Cloud Run）
│   └── crawler/
│       └── main.go                   # クローラー エントリポイント（Cloud Run Jobs）
├── internal/
│   ├── handler/
│   │   ├── similar.go                # GET /similar
│   │   ├── blogs.go                  # GET /blogs（管理用）
│   │   ├── stats.go                  # GET /stats（管理用）
│   │   └── keywords.go               # GET /keywords（管理用）
│   ├── crawler/
│   │   ├── discovery.go              # はてなブログ公開ページからブログURL収集
│   │   ├── historical.go             # 過去記事の時間断面サンプリング
│   │   ├── recent.go                 # ブックマーク数0の最新エントリー収集
│   │   ├── indexer.go                # 記事インデックス構築
│   │   ├── syncer.go                 # 差分更新
│   │   └── scheduler.go              # 全フェーズのインプロセス スケジューリング（開発用）
│   ├── rss/
│   │   └── fetcher.go                # RSSフィード取得・パース
│   ├── tfidf/
│   │   └── tfidf.go                  # TF-IDF によるキーワード抽出
│   ├── embedding/
│   │   └── openai.go                 # OpenAI Embeddings APIクライアント（並列制限付き）
│   ├── repository/
│   │   ├── blog.go                   # blogs テーブル CRUD
│   │   ├── article.go                # articles テーブル CRUD + ベクトル検索
│   │   └── keyword.go                # crawler_keywords テーブル CRUD
│   ├── middleware/
│   │   ├── apikey.go                 # APIキー認証ミドルウェア
│   │   └── logger.go                 # リクエストログ
│   └── model/
│       ├── blog.go
│       ├── article.go
│       └── platform.go
├── migrations/
│   ├── 001_create_platforms.{up,down}.sql
│   ├── 002_create_blogs.{up,down}.sql
│   ├── 003_create_articles.{up,down}.sql
│   ├── 004_articles_fk_cascade.{up,down}.sql
│   ├── 005_create_crawler_keywords.{up,down}.sql
│   └── embed.go
├── public/                           # Firebase Hosting デプロイ対象
│   ├── admin.html                    # 管理画面
│   ├── viewer.html                   # ビューワー
│   ├── config.js                     # API URL設定（ローカル: localhost:8080、本番: CI/CDで上書き）
│   └── style.css
├── .github/
│   └── workflows/
│       └── deploy.yml                # CI/CD（master push → Cloud Run / Firebase デプロイ）
├── open-api/
│   └── similar.yaml                  # OpenAPI仕様
├── config/
│   └── config.go
├── scripts/
├── docker-compose.yml
├── Dockerfile
├── Makefile
├── go.mod
├── go.sum
└── README.md
```

---

## 環境変数

### 共通

| 変数名                  | デフォルト              | 説明                                          |
| ----------------------- | ----------------------- | --------------------------------------------- |
| `PORT`                  | `8080`                  | サーバーポート                                |
| `DATABASE_URL`          | —（必須）               | PostgreSQL接続文字列                          |
| `OPENAI_API_KEY`        | —（必須）               | OpenAI APIキー                                |
| `OPENAI_EMBEDDING_MODEL`| `text-embedding-3-small`| 使用するEmbeddingモデル名                     |
| `API_KEY`               | —（必須）               | 管理系エンドポイントの認証キー                |
| `CRAWL_CONCURRENCY`     | `5`                     | OpenAI API 並列呼び出し数上限                 |
| `EMBED_MAX_CHARS`        | `1000`                  | Embedding に使用するテキストの最大文字数      |
| `CORS_ALLOWED_ORIGINS`  | `*`                     | 許可CORSオリジン（カンマ区切り）              |
| `LOG_LEVEL`             | `info`                  | ログレベル（debug/info/warn/error）           |

### クローラー固有

| 変数名                        | デフォルト        | 対象フェーズ    | 説明                                                        |
| ----------------------------- | ----------------- | --------------- | ----------------------------------------------------------- |
| `CRAWLER_PHASE`               | `indexer`         | 共通            | 実行フェーズ（discovery/indexer/syncer/historical/recent）  |
| `TFIDF_SAMPLE_SIZE`           | `500`             | discovery       | TF-IDF コーパスサイズ（記事数）                             |
| `TFIDF_KEYWORD_COUNT`         | `20`              | discovery       | TF-IDF 抽出キーワード数                                     |
| `INDEX_BATCH_SIZE`            | `50`              | indexer         | 1回あたりの処理ブログ数                                     |
| `INDEX_MAX_ERROR_COUNT`       | `3`               | indexer         | ブログ削除に移行するエラー連続回数                          |
| `MAX_ARTICLES_PER_BLOG`       | `5`               | indexer/syncer  | 1ブログあたりのインデックス上限記事数                       |
| `SYNC_STALENESS_DAYS`         | `30`              | syncer          | 差分チェック対象とする最終同期からの経過日数                |
| `SYNC_BATCH_SIZE`             | `50`              | syncer          | 1回あたりの処理ブログ数                                     |
| `SYNC_MAX_ERROR_COUNT`        | `3`               | syncer          | error 状態に移行するエラー連続回数                          |
| `CRAWL_DATE_FROM`             | `2010-01-01`      | historical      | 過去クロールの対象開始日                                    |
| `CRAWL_DATE_TO`               | `（1年前の日付）` | historical      | 過去クロールの対象終了日                                    |
| `HISTORICAL_BOOKMARK_MAX`     | `200`             | historical      | ブックマーク数検索の上限（0〜N のランダム）                 |
| `HISTORICAL_DATE_WINDOW_DAYS` | `7`               | historical      | 日付範囲検索のウィンドウ幅（日）                            |
| `HISTORICAL_DATE_USERS_MAX`   | `2`               | historical      | 日付範囲検索のブックマーク数上限（0〜N）                    |
| `CRAWL_INTERVAL_MIN`          | `360`             | Scheduler       | discovery フェーズの実行間隔（分）                          |
| `SYNC_INTERVAL_MIN`           | `60`              | Scheduler       | indexer フェーズの実行間隔（分）                            |
| `HISTORICAL_INTERVAL_MIN`     | `1440`            | Scheduler       | historical フェーズの実行間隔（分）                         |
| `RECENT_INTERVAL_MIN`         | `30`              | Scheduler       | recent フェーズの実行間隔（分）                             |

> Scheduler 系の変数はインプロセス スケジューリング（`scheduler.go`）用。本番の Cloud Run Jobs では Cloud Scheduler がトリガーを担うため不要。

---

## デプロイ・運用

### 環境構成

- **開発環境**: Docker Compose（Go + PostgreSQL + pgvector）
- **本番環境**:
  - Firebase Hosting — 静的HTML（無料枠）
  - Cloud Run — Go API（`blogdog-api`、asia-northeast1）
  - Cloud Run Jobs — Goクローラー フェーズ別5ジョブ（毎週日曜 JST、パイプライン順に実行）
    - `blogdog-crawler-discovery` — 毎週日曜 3:00 JST
    - `blogdog-crawler-indexer` — 毎週日曜 4:00 JST
    - `blogdog-crawler-syncer` — 毎週日曜 5:00 JST
    - `blogdog-crawler-historical` — 毎週日曜 5:30 JST
    - `blogdog-crawler-recent` — 毎週日曜 6:00 JST
  - Cloud Scheduler — 各ジョブのトリガー（`CRAWLER_PHASE` 環境変数でフェーズ選択）。運用コマンドは `docs/operations.md` を参照
  - Neon — PostgreSQL + pgvector（無料枠）

### 監視・ログ

- ログ: 構造化JSONログ（`log/slog`）を標準出力に出力。Cloud Run のログエクスプローラーで参照。
- アラート: Cloud Run の障害通知（Cloud Monitoring）

### CI/CD

- `master` push → GitHub Actions が自動デプロイ
  1. Docker イメージをビルドして Artifact Registry（asia-northeast1）に push
  2. Cloud Run（API）をデプロイ
  3. 全クローラー Jobs のイメージを更新
  4. Cloud Run の URL を取得して `public/config.js` を生成
  5. Firebase Hosting にデプロイ
- 必要な GitHub Secrets: `GCP_SA_KEY`（サービスアカウントJSONのbase64）
