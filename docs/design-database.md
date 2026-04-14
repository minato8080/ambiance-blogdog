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
| `blog_id`      | `VARCHAR(26)`  | ブログID（FK → blogs.id）                   |
| `url`          | `TEXT`         | 記事URL（ユニーク）                         |
| `title`        | `TEXT`         | 記事タイトル                                |
| `summary`      | `TEXT`         | 本文サマリー                                |
| `tags`         | `TEXT[]`       | タグ一覧                                    |
| `embedding`    | `vector(1536)` | Embeddingベクトル（text-embedding-3-small） |
| `published_at` | `TIMESTAMPTZ`  | 記事公開日時                                |
| `indexed_at`   | `TIMESTAMPTZ`  | インデックス更新日時                        |

---

## インデックス

| 対象                    | 種別               | 用途                     |
| ----------------------- | ------------------ | ------------------------ |
| `articles.embedding`    | IVFFlat (pgvector) | コサイン類似度検索       |
| `articles.blog_id`      | B-tree             | ブログ別記事絞り込み     |
| `articles.published_at` | B-tree             | 公開日時ソート・絞り込み |
| `blogs.blog_url`        | ユニーク           | 重複登録防止             |
