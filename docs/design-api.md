# API詳細設計

## エンドポイント一覧

| メソッド | パス       | 認証    | 説明                         |
| -------- | ---------- | ------- | ---------------------------- |
| `GET`    | `/similar` | 不要    | 類似記事取得                 |
| `GET`    | `/blogs`   | APIキー | 収集済みブログ一覧（管理用） |
| `GET`    | `/stats`   | APIキー | クロール統計情報（管理用）   |

---

## `GET /similar` — 類似記事取得

### クエリパラメータ

| パラメータ | 型     | 必須 | 説明                                |
| ---------- | ------ | ---- | ----------------------------------- |
| `url`      | string | ✅   | 類似記事を探したい対象記事のURL     |
| `limit`    | int    | ❌   | 返却件数（デフォルト: 5、最大: 20） |

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
