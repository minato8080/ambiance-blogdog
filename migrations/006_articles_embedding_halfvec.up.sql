-- Embedding を vector(1536) から halfvec(512) に縮小し、IVFFlat 索引を廃止する。
--
-- - text-embedding-3-small は先頭 N 次元に切り詰めて L2 正規化しても使える作り
--   （API の dimensions 指定と同等）なので、既存値から変換して OpenAI の再呼び出しは不要。
-- - 索引は空テーブルで作成されていて recall@10 が 0.16 程度しかなく、容量も約200MB あったため削除する。
--   検索は全件比較（シーケンシャルスキャン）になる。
-- - ALTER COLUMN TYPE はテーブルを作り直すため、古い行の領域（TOAST を含む）もこの時点で回収される。
-- 必要: pgvector 0.7.0 以上（halfvec / subvector / l2_normalize）
DROP INDEX IF EXISTS idx_articles_embedding;

ALTER TABLE articles
    ALTER COLUMN embedding TYPE halfvec(512)
    USING l2_normalize(subvector(embedding, 1, 512))::halfvec(512);
