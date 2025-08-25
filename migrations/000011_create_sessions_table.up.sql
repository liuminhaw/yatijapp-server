CREATE TABLE IF NOT EXISTS "sessions" (
    "uuid" uuid PRIMARY KEY DEFAULT uuid_generate_v1 (),
    "start" timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    "end" timestamp(0) with time zone,
    "note" text NOT NULL DEFAULT '',
    "version" int NOT NULL DEFAULT 1,
    "activity_uuid" uuid NOT NULL REFERENCES activities(uuid) ON DELETE CASCADE
);

