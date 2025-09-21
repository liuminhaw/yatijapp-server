CREATE INDEX IF NOT EXISTS sessions_fts_chinese_notes_tsv_idx ON targets_fts USING GIN (fts_chinese_notes_tsv);
CREATE INDEX IF NOT EXISTS sessions_fts_english_notes_tsv_idx ON targets_fts USING GIN (fts_english_notes_tsv);
