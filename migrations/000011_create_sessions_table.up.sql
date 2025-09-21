CREATE TABLE IF NOT EXISTS "sessions" (
    "uuid" uuid PRIMARY KEY DEFAULT uuidv7 (),
    "starts_at" timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    "ends_at" timestamp(0) with time zone,
    "created_at" timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    "updated_at" timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    "notes" text NOT NULL DEFAULT '',
    "version" int NOT NULL DEFAULT 1,
    "action_uuid" uuid NOT NULL REFERENCES actions(uuid) ON DELETE CASCADE,
    CONSTRAINT ends_after_starts CHECK (ends_at IS NULL OR ends_at > starts_at)
);

CREATE INDEX "sessions_actions_uuid_idx" ON "sessions" ("action_uuid");

