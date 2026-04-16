CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE articles (
    id           VARCHAR(26)    PRIMARY KEY,
    blog_id      VARCHAR(26)    NOT NULL REFERENCES blogs(id),
    url          TEXT           NOT NULL UNIQUE,
    title        TEXT           NOT NULL DEFAULT '',
    summary      TEXT           NOT NULL DEFAULT '',
    tags         TEXT[]         NOT NULL DEFAULT '{}',
    embedding    vector(1536),
    published_at TIMESTAMPTZ,
    indexed_at   TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_articles_blog_id      ON articles (blog_id);
CREATE INDEX idx_articles_published_at ON articles (published_at);
-- IVFFlat インデックス（コサイン類似度検索用）
-- NOTE: lists は sqrt(行数) が推奨値。データ投入後に REINDEX を推奨。
CREATE INDEX idx_articles_embedding ON articles USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
