CREATE TABLE IF NOT EXISTS "actions_fts" (
    "action_uuid" uuid PRIMARY KEY REFERENCES actions(uuid) ON DELETE CASCADE,
    "fts_chinese_tsv" tsvector NOT NULL,
    "fts_english_tsv" tsvector NOT NULL,
    "fts_chinese_notes_tsv" tsvector NOT NULL,
    "fts_english_notes_tsv" tsvector NOT NULL
)
