CREATE TABLE IF NOT EXISTS "preferences" (
    "user_uuid" uuid PRIMARY KEY REFERENCES users (uuid) ON DELETE CASCADE,
    "preference" jsonb NOT NULL
);
