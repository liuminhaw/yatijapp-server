CREATE TABLE IF NOT EXISTS "targets_fts" (
    "target_uuid" UUID PRIMARY KEY REFERENCES targets(uuid) ON DELETE CASCADE,
    "fts_chinese_tsv" TSVECTOR NOT NULL,
    "fts_english_tsv" TSVECTOR NOT NULL,
    "fts_chinese_notes_tsv" TSVECTOR NOT NULL,
    "fts_english_notes_tsv" TSVECTOR NOT NULL
);

