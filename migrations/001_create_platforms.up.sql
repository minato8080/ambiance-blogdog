CREATE TABLE platforms (
    id               VARCHAR(26)  PRIMARY KEY,
    slug             VARCHAR(50)  NOT NULL UNIQUE,
    name             TEXT         NOT NULL,
    feed_url_pattern TEXT         NOT NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- 初期データ: はてなブログ
INSERT INTO platforms (id, slug, name, feed_url_pattern) VALUES
    ('01ARZ3NDEKTSV4RRFFQ69G5FAV', 'hatenablog', 'はてなブログ', '{blog_url}/feed');
