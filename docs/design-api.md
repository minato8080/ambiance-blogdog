# API詳細設計

## エンドポイント一覧

| メソッド | パス        | 認証    | 説明                         |
| -------- | ----------- | ------- | ---------------------------- |
| `GET`    | `/similar`  | 不要    | 類似記事取得                 |
| `GET`    | `/blogs`    | APIキー | 収集済みブログ一覧（管理用） |
| `GET`    | `/stats`    | APIキー | クロール統計情報（管理用）   |
| `GET`    | `/keywords` | APIキー | クローラーキーワード一覧（管理用） |

---

## `GET /similar` — 類似記事取得

### クエリパラメータ

| パラメータ | 型     | 必須 | 説明                                |
| ---------- | ------ | ---- | ----------------------------------- |
| `url`      | string | ✅   | 類似記事を探したい対象記事のURL     |
| `limit`    | int    | ❌   | 返却件数（デフォルト: 10、最大: 20） |

### 処理フロー

```
1. url パラメータを検証
2. DBにインデックス済みか確認
   a. インデックス済み: 既存のEmbeddingを使用
   b. 未登録: 記事URLをフェッチ → HTMLパース → Embedding生成 → DBにUPSERT
3. pgvectorでコサイン類似度検索（対象記事自身を除外）
4. limit件数でレスポンスを返却
```

> オンデマンドで解析するため、未登録記事でも類似記事を返せる。
> UPSERTにより、次回以降は既存Embeddingを再利用して高速に応答する。

### レスポンス（200 OK）

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

### エラーレスポンス

| HTTPステータス | コード                  | 説明                                |
| -------------- | ----------------------- | ----------------------------------- |
| 400            | `INVALID_PARAMS`        | パラメータ不正                      |
| 422            | `ARTICLE_FETCH_FAILED`  | 対象記事URLのフェッチ・パースに失敗 |
| 503            | `EMBEDDING_UNAVAILABLE` | Embedding API呼び出し失敗           |

---

## `GET /blogs` — 収集済みブログ一覧

> APIキー認証必須（`Authorization: Bearer <API_KEY>`）

### クエリパラメータ

なし

### レスポンス（200 OK）

```json
{
  "blogs": [
    {
      "id": "01J...",
      "blog_url": "https://example.hatenablog.com",
      "name": "Example Blog",
      "status": "ready",
      "error_count": 0,
      "last_synced_at": "2024-03-01T12:00:00+09:00",
      "discovered_at": "2024-01-01T00:00:00+09:00"
    }
  ],
  "total": 1
}
```

#### `status` の値

| 値         | 説明                               |
| ---------- | ---------------------------------- |
| `pending`  | クロール待ち                       |
| `indexing` | インデックス構築中                 |
| `ready`    | インデックス済み（検索対象）       |
| `error`    | エラー状態（連続失敗によりスキップ）|

### エラーレスポンス

| HTTPステータス | コード           | 説明             |
| -------------- | ---------------- | ---------------- |
| 401            | `UNAUTHORIZED`   | APIキー不正      |
| 500            | `INTERNAL_ERROR` | DB取得失敗       |

---

## `GET /stats` — クロール統計情報

> APIキー認証必須（`Authorization: Bearer <API_KEY>`）

### クエリパラメータ

なし

### レスポンス（200 OK）

```json
{
  "blogs": {
    "total": 120,
    "by_status": {
      "pending": 10,
      "indexing": 3,
      "ready": 105,
      "error": 2
    }
  },
  "articles": {
    "total": 498
  }
}
```

### エラーレスポンス

| HTTPステータス | コード           | 説明             |
| -------------- | ---------------- | ---------------- |
| 401            | `UNAUTHORIZED`   | APIキー不正      |
| 500            | `INTERNAL_ERROR` | DB取得失敗       |

---

## `GET /keywords` — クローラーキーワード一覧

> APIキー認証必須（`Authorization: Bearer <API_KEY>`）

### クエリパラメータ

なし

### レスポンス（200 OK）

```json
{
  "keywords": ["実体験", "Go言語", "機械学習"],
  "total": 3
}
```

> クローラーが未実行の場合は `keywords` が空配列になる。

### エラーレスポンス

| HTTPステータス | コード           | 説明             |
| -------------- | ---------------- | ---------------- |
| 401            | `UNAUTHORIZED`   | APIキー不正      |
| 500            | `INTERNAL_ERROR` | DB取得失敗       |
