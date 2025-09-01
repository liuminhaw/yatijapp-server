CREATE TABLE IF NOT EXISTS "activities_fts" (
    "activity_uuid" uuid PRIMARY KEY REFERENCES activities(uuid) ON DELETE CASCADE,
    "fts_chinese_tsv" tsvector NOT NULL,
    "fts_english_tsv" tsvector NOT NULL
)
