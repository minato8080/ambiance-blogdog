CREATE TABLE blogs (
    id             VARCHAR(26)  PRIMARY KEY,
    platform_id    VARCHAR(26)  NOT NULL REFERENCES platforms(id),
    blog_url       TEXT         NOT NULL UNIQUE,
    name           TEXT         NOT NULL DEFAULT '',
    status         VARCHAR(20)  NOT NULL DEFAULT 'pending',
    error_count    INT          NOT NULL DEFAULT 0,
    last_synced_at TIMESTAMPTZ,
    discovered_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_blog_status CHECK (status IN ('pending', 'indexing', 'ready', 'error'))
);

CREATE INDEX idx_blogs_blog_url    ON blogs (blog_url);
CREATE INDEX idx_blogs_status      ON blogs (status);
CREATE INDEX idx_blogs_platform_id ON blogs (platform_id);
