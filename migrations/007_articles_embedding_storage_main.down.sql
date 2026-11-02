-- halfvec の既定ストレージ（EXTERNAL）に戻す。既存行は移動しない（以降に書き込まれる行から適用される）。
ALTER TABLE articles ALTER COLUMN embedding SET STORAGE EXTERNAL;
