# Ambiance Blogdog

はてなブログの記事URLを受け取り、本文の意味的類似度によって類似記事一覧を返すREST APIサービス。

## 概要

- はてなブログの公開ディレクトリ・フィードを自動クロールして記事を収集・ベクトル化
- OpenAI Embeddings API（text-embedding-3-small）でベクトルを生成し、pgvectorに保存
- 指定URLの記事に対してコサイン類似度検索を行い、類似記事を返却
- 未インデックスの記事URLでもオンデマンドでベクトル化して応答

## 外部サービス

| サービス                  | 用途                                               | 備考           |
| ------------------------- | -------------------------------------------------- | -------------- |
| **OpenAI Embeddings API** | `text-embedding-3-small` でベクトル生成            | APIキー必須    |
| **Neon**                  | PostgreSQL + pgvector（ベクトルDB）                | 無料枠         |
| **Firebase Hosting**      | 管理画面・ビューワー（静的HTML）                   | 無料枠         |
| **GCP Cloud Run**         | Go API サーバー（`blogdog-api`、asia-northeast1）  | —              |
| **GCP Cloud Run Jobs**    | クローラー フェーズ別5ジョブ                       | —              |
| **GCP Cloud Scheduler**   | Cloud Run Jobs の定期実行トリガー                  | —              |
| **GCP Artifact Registry** | Dockerイメージ保存（asia-northeast1）              | —              |
| **GitHub Actions**        | CI/CD（master push → 自動デプロイ）                | —              |

## 技術スタック

| 項目               | 採用技術                                       |
| ------------------ | ---------------------------------------------- |
| 言語               | Go 1.22+                                       |
| DB                 | PostgreSQL 15+ with pgvector                   |
| HTTPフレームワーク | `net/http`（標準ライブラリ）                   |
| Embeddings         | OpenAI text-embedding-3-small                  |
| DBドライバ         | `github.com/jackc/pgx/v5`                      |
| RSSパーサー        | `github.com/mmcdole/gofeed`                    |
| HTMLパーサー       | `github.com/PuerkitoBio/goquery`               |
| OpenAI SDK         | `github.com/sashabaranov/go-openai`            |
| コンテナ           | Docker / docker-compose                        |

## セットアップ

### 前提条件

- Docker / Docker Compose
- OpenAI APIキー

### 起動

```bash
cp .env.example .env
# .env を編集して OPENAI_API_KEY, API_KEY などを設定

docker compose up -d
```

### マイグレーション

```bash
make db-migrate   # 未適用のマイグレーションを適用
make db-reset     # DBを初期化（全データ削除して再作成）
make db-rollback  # すべてのマイグレーションをロールバック
```

デフォルトの接続先は `localhost:5432`。別のDBを使う場合は `DB_URL` を上書きする。

```bash
make db-reset DB_URL=pgx5://user:pass@host:5432/dbname?sslmode=disable
```

## 環境変数

### 共通

| 変数名                  | デフォルト | 説明                                          |
| ----------------------- | ---------- | --------------------------------------------- |
| `PORT`                  | `8080`     | サーバーポート                                |
| `DATABASE_URL`          | —          | PostgreSQL接続文字列                          |
| `OPENAI_API_KEY`        | —          | OpenAI APIキー                                |
| `API_KEY`               | —          | 管理系エンドポイントの認証キー                |
| `CRAWL_CONCURRENCY`     | `5`        | OpenAI API並列呼び出し数上限                  |
| `EMBED_MAX_CHARS`       | `1000`     | Embeddingに使用するテキストの最大文字数       |
| `CORS_ALLOWED_ORIGINS`  | `*`        | 許可CORSオリジン（カンマ区切り）              |
| `LOG_LEVEL`             | `info`     | ログレベル                                    |

### クローラー固有

| 変数名                        | デフォルト        | 対象フェーズ   | 説明                                                        |
| ----------------------------- | ----------------- | -------------- | ----------------------------------------------------------- |
| `CRAWLER_PHASE`               | `indexer`         | 共通           | 実行フェーズ（discovery/indexer/syncer/historical/recent）  |
| `TFIDF_SAMPLE_SIZE`           | `500`             | discovery      | TF-IDF コーパスサイズ（記事数）                             |
| `TFIDF_KEYWORD_COUNT`         | `20`              | discovery      | TF-IDF 抽出キーワード数                                     |
| `INDEX_BATCH_SIZE`            | `50`              | indexer        | 1回あたりの処理ブログ数                                     |
| `INDEX_MAX_ERROR_COUNT`       | `3`               | indexer        | error 状態に移行するエラー連続回数                          |
| `MAX_ARTICLES_PER_BLOG`       | `5`               | indexer/syncer | 1ブログあたりのインデックス上限記事数                       |
| `SYNC_STALENESS_DAYS`         | `30`              | syncer         | 差分チェック対象とする最終同期からの経過日数                |
| `SYNC_BATCH_SIZE`             | `50`              | syncer         | 1回あたりの処理ブログ数                                     |
| `SYNC_MAX_ERROR_COUNT`        | `3`               | syncer         | error 状態に移行するエラー連続回数                          |
| `CRAWL_DATE_FROM`             | `2010-01-01`      | historical     | 過去クロールの対象開始日                                    |
| `CRAWL_DATE_TO`               | `（1年前の日付）` | historical     | 過去クロールの対象終了日                                    |
| `HISTORICAL_BOOKMARK_MAX`     | `200`             | historical     | ブックマーク数検索の上限（0〜N のランダム）                 |
| `HISTORICAL_DATE_WINDOW_DAYS` | `7`               | historical     | 日付範囲検索のウィンドウ幅（日）                            |
| `HISTORICAL_DATE_USERS_MAX`   | `2`               | historical     | 日付範囲検索のブックマーク数上限（0〜N）                    |

## API

### `GET /similar` — 類似記事取得

```
GET /similar?url=https://example.hatenablog.com/entry/...&limit=5
```

| パラメータ | 必須 | 説明                                 |
| ---------- | ---- | ------------------------------------ |
| `url`      | ✅   | 類似記事を探したい対象記事のURL      |
| `limit`    | ❌   | 返却件数（デフォルト: 10、最大: 20） |

**レスポンス例:**

```json
{
  "source": {
    "url": "https://example.hatenablog.com/entry/2024/01/01/post1",
    "title": "Goで始めるWebAPI開発"
  },
  "similar_articles": [
    {
      "url": "https://example.hatenablog.com/entry/2024/02/01/post2",
      "title": "GoのHTTPハンドラ設計パターン",
      "published_at": "2024-02-01T10:00:00+09:00",
      "tags": ["Go", "API"],
      "similarity": 0.91
    }
  ],
  "total": 4
}
```

### `GET /blogs` — 収集済みブログ一覧（APIキー認証必須）

### `GET /stats` — クロール統計情報（APIキー認証必須）

### `GET /keywords` — クローラーキーワード一覧（APIキー認証必須）

管理系エンドポイントは `Authorization: Bearer <API_KEY>` ヘッダーが必要。

## クローラー動作

`CRAWLER_PHASE` 環境変数でフェーズを切り替えて実行する。

| フェーズ      | `CRAWLER_PHASE` | 処理                                            | 実行間隔  |
| ------------- | --------------- | ----------------------------------------------- | --------- |
| ブログ発見    | `discovery`     | はてなブログ公開ページからブログURLを発見・登録 | 6時間     |
| インデックス  | `indexer`       | `pending` ブログのRSSを取得し記事をベクトル化   | 1時間     |
| 差分更新      | `syncer`        | `ready` ブログの差分チェック・更新              | 24時間    |
| 時間断面      | `historical`    | 過去記事のランダムサンプリング収集              | 24時間    |
| 最新記事      | `recent`        | ブックマーク数0の新着記事からブログを収集       | 30分      |

## デプロイ（本番）

### 構成

```
Firebase Hosting  ── 管理画面・ビューワー（静的HTML）
Cloud Run         ── Go API サーバー（blogdog-api）
Cloud Run Jobs    ── クローラー（フェーズ別5ジョブ）
Cloud Scheduler   ── 各ジョブの定期実行トリガー
Artifact Registry ── Dockerイメージ（asia-northeast1）
Neon              ── PostgreSQL + pgvector
```

### CI/CD

`master` ブランチへのpushでGitHub Actionsが自動デプロイする。

1. APIイメージ・クローラーイメージをビルドしてArtifact Registryにpush
2. Cloud Run（API）をデプロイ
3. 全クローラー Jobs のイメージを更新
4. Cloud RunのURLを取得して `public/config.js` を生成
5. Firebase Hostingにデプロイ

必要なGitHub Secrets:

| Secret名      | 説明                               |
| ------------- | ---------------------------------- |
| `GCP_SA_KEY`  | サービスアカウントJSONのbase64エンコード |

## ライセンス

MIT
