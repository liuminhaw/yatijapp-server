CREATE TABLE IF NOT EXISTS "activities" (
    "uuid" uuid PRIMARY KEY DEFAULT uuidv7 (),
    "serial_id" bigserial NOT NULL UNIQUE,
    "title" text NOT NULL,
    "description" text NOT NULL,
    "notes" text NOT NULL DEFAULT '',
    "created_at" timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    "updated_at" timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    "last_active" timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    "due_date" date,
    "status" statuses NOT NULL DEFAULT 'queued',
    "version" int NOT NULL DEFAULT 1,
    "target_uuid" uuid NOT NULL REFERENCES targets (uuid) ON DELETE CASCADE
);

