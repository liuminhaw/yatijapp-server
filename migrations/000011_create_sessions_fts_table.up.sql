CREATE TABLE IF NOT EXISTS "sessions_fts" (
    "session_uuid" uuid PRIMARY KEY REFERENCES sessions(uuid) ON DELETE CASCADE,
    "fts_chinese_tsv" tsvector NOT NULL,
    "fts_english_tsv" tsvector NOT NULL
);
