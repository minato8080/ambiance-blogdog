-- 512 次元から 1536 次元は復元できないため、embedding は NULL に戻す。
-- 元に戻した後は全記事の Embedding を再生成する必要がある（旧コードは 1536 次元で書き込む）。
ALTER TABLE articles
    ALTER COLUMN embedding TYPE vector(1536)
    USING NULL;

CREATE INDEX idx_articles_embedding ON articles USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
