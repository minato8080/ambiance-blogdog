ALTER TABLE articles
    DROP CONSTRAINT articles_blog_id_fkey,
    ADD CONSTRAINT articles_blog_id_fkey
        FOREIGN KEY (blog_id) REFERENCES blogs(id) ON DELETE CASCADE;
