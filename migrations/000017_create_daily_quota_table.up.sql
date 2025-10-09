CREATE TABLE IF NOT EXISTS "daily_quota" (
    "usage_date" date NOT NULL,
    "resource" text NOT NULL CHECK (resource IN ('target', 'action', 'session')),
    "quota_used" int NOT NULL DEFAULT 0,
    -- "quota_limit" int NOT NULL,
    "user_id" uuid NOT NULL REFERENCES users (uuid) ON DELETE CASCADE,
    PRIMARY KEY ("usage_date", "resource", "user_id")
);

