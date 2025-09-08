CREATE INDEX IF NOT EXISTS fts_chinese_tsv_idx ON targets_fts USING GIN (fts_chinese_tsv);
CREATE INDEX IF NOT EXISTS fts_english_tsv_idx ON targets_fts USING GIN (fts_english_tsv);
CREATE INDEX IF NOT EXISTS fts_chinese_notes_tsv_idx ON targets_fts USING GIN (fts_chinese_notes_tsv);
CREATE INDEX IF NOT EXISTS fts_english_notes_tsv_idx ON targets_fts USING GIN (fts_english_notes_tsv);
