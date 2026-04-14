# Ambiance Blogdog

はてなブログの記事URLを受け取り、本文の意味的類似度によって類似記事一覧を返すREST APIサービス。

## 概要

- はてなブログの公開ディレクトリ・フィードを自動クロールして記事を収集・ベクトル化
- OpenAI Embeddings API（text-embedding-3-small）でベクトルを生成し、pgvectorに保存
- 指定URLの記事に対してコサイン類似度検索を行い、類似記事を返却

## 技術スタック

| 項目               | 採用技術                      |
| ------------------ | ----------------------------- |
| 言語               | Go 1.22+                      |
| DB                 | PostgreSQL 15+ with pgvector  |
| HTTPフレームワーク | `net/http`（標準ライブラリ）  |
| Embeddings         | OpenAI text-embedding-3-small |

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
docker compose exec app migrate -path ./migrations -database $DATABASE_URL up
```

## 環境変数

| 変数名                  | デフォルト | 説明                                  |
| ----------------------- | ---------- | ------------------------------------- |
| `PORT`                  | `8080`     | サーバーポート                        |
| `DATABASE_URL`          | —          | PostgreSQL接続文字列                  |
| `OPENAI_API_KEY`        | —          | OpenAI APIキー                        |
| `API_KEY`               | —          | 管理系エンドポイントの認証キー        |
| `CRAWL_INTERVAL_MIN`    | `360`      | ブログ発見クロール間隔（分）          |
| `SYNC_INTERVAL_MIN`     | `60`       | 記事インデックス更新間隔（分）        |
| `SYNC_STALENESS_DAYS`   | `30`       | 差分チェック間隔（日）                |
| `CRAWL_CONCURRENCY`     | `5`        | OpenAI API並列呼び出し数上限          |
| `MAX_ARTICLES_PER_BLOG` | `5`        | 1ブログあたりのインデックス上限記事数 |
| `CORS_ALLOWED_ORIGINS`  | `*`        | 許可CORSオリジン（カンマ区切り）      |

## API

### `GET /similar` — 類似記事取得

```
GET /similar?url=https://example.hatenablog.com/entry/...&limit=5
```

| パラメータ | 必須 | 説明                                |
| ---------- | ---- | ----------------------------------- |
| `url`      | ✅   | 類似記事を探したい対象記事のURL     |
| `limit`    | ❌   | 返却件数（デフォルト: 5、最大: 20） |

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

管理系エンドポイントは `Authorization: Bearer <API_KEY>` ヘッダーが必要。

## クローラー動作

バックグラウンドで3フェーズのクローラーが動作する。

| フェーズ  | 処理                                            | 実行間隔 |
| --------- | ----------------------------------------------- | -------- |
| フェーズ1 | はてなブログ公開ページからブログURLを発見・登録 | 6時間    |
| フェーズ2 | `pending` ブログのRSSを取得し記事をベクトル化   | 1時間    |
| フェーズ3 | `ready` ブログの差分チェック・更新              | 24時間   |

## ライセンス

MIT
