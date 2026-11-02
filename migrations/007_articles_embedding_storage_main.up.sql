-- embedding（halfvec(512)、約1KB）を行内に保存する。
--
-- halfvec の既定ストレージは EXTERNAL のため、summary などと合わせて行が約2KBを超えると
-- embedding が TOAST（別テーブル）に追い出され、全件比較で1行ごとに TOAST を読みに行っていた
-- （本番の EXPLAIN で 26,530 行に対し約10万バッファ参照）。
-- MAIN にすると、行が大きい場合は先に summary（EXTENDED）が圧縮・TOAST 化され、embedding は行内に残る。
--
-- SET STORAGE だけでは既存行は移動しないため、同じ型への USING 付き変換でテーブルを作り直す。
-- 同じ ALTER TABLE 文の中で SET STORAGE も指定する（ALTER TYPE は列のストレージを型の既定値に戻すため）。
ALTER TABLE articles
    ALTER COLUMN embedding TYPE halfvec(512) USING embedding::text::halfvec(512),
    ALTER COLUMN embedding SET STORAGE MAIN;
