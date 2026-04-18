CREATE TABLE crawler_keywords (
    keyword    TEXT        PRIMARY KEY,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
