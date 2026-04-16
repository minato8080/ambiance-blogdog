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
blog-finder/
├── cmd/
│   └── server/
│       └── main.go                   # エントリポイント（APIサーバー + ワーカー起動）
├── internal/
│   ├── handler/
│   │   ├── similar.go                # GET /similar
│   │   ├── blogs.go                  # GET /blogs（管理用）
│   │   └── stats.go                  # GET /stats（管理用）
│   ├── crawler/
│   │   ├── discovery.go              # はてなブログ公開ページからブログURL収集
│   │   └── scheduler.go              # 3フェーズのスケジューリング管理
│   ├── rss/
│   │   └── fetcher.go                # RSSフィード取得・パース
│   ├── tfidf/
│   │   └── tfidf.go                  # TF-IDF によるキーワード抽出
│   ├── embedding/
│   │   └── openai.go                 # OpenAI Embeddings APIクライアント（並列制限付き）
│   ├── repository/
│   │   ├── blog.go                   # blogs テーブル CRUD
│   │   └── article.go                # articles テーブル CRUD + ベクトル検索
│   ├── middleware/
│   │   ├── apikey.go                 # APIキー認証ミドルウェア
│   │   └── logger.go                 # リクエストログ
│   └── model/
│       ├── blog.go
│       └── article.go
├── migrations/
│   ├── 001_create_platforms.up.sql
│   ├── 001_create_platforms.down.sql
│   ├── 002_create_blogs.up.sql
│   ├── 002_create_blogs.down.sql
│   ├── 003_create_articles.up.sql
│   └── 003_create_articles.down.sql
├── config/
│   └── config.go
├── docker-compose.yml
├── go.mod
├── go.sum
└── README.md
```

---

## 環境変数

| 変数名                  | デフォルト        | 説明                                          |
| ----------------------- | ----------------- | --------------------------------------------- |
| `PORT`                  | `8080`            | サーバーポート                                |
| `DATABASE_URL`          | —                 | PostgreSQL接続文字列                          |
| `OPENAI_API_KEY`        | —                 | OpenAI APIキー                                |
| `API_KEY`               | —                 | 管理系エンドポイントの認証キー                |
| `CRAWL_INTERVAL_MIN`    | `360`             | ブログ発見クロール間隔（分）                  |
| `SYNC_INTERVAL_MIN`     | `60`              | 記事インデックス更新間隔（分）                |
| `SYNC_STALENESS_DAYS`   | `30`              | ready状態のブログを差分チェックする間隔（日） |
| `CRAWL_CONCURRENCY`     | `5`               | OpenAI API並列呼び出し数上限                  |
| `CRAWL_DATE_FROM`       | `2010-01-01`      | 過去ランダムクロールの対象開始日              |
| `CRAWL_DATE_TO`         | `（1年前の日付）` | 過去ランダムクロールの対象終了日              |
| `MAX_ARTICLES_PER_BLOG` | `5`               | 1ブログあたりのインデックス上限記事数         |
| `CORS_ALLOWED_ORIGINS`  | `*`               | 許可CORSオリジン（カンマ区切り）              |
| `LOG_LEVEL`             | `info`            | ログレベル                                    |

---

## デプロイ・運用

### 環境構成

- **開発環境**: Docker Compose（Go + PostgreSQL + pgvector）
- **本番環境**: Kubernetes または AWS ECS
- **データベース**: PostgreSQL 15+ with pgvector 拡張

### 監視・ログ

- メトリクス: Prometheus + Grafana
- ログ: ELK Stack または CloudWatch
- アラート: API レスポンスタイム超過、クローラー失敗

### CI/CD

- GitHub Actions による自動テスト・デプロイ
- テストカバレッジ: 80% 以上
