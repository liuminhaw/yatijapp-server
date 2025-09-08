CREATE TABLE IF NOT EXISTS "targets" (
    "uuid" uuid PRIMARY KEY DEFAULT uuidv7 (),
    "serial_id" bigserial NOT NULL UNIQUE,
    "title" text NOT NULL,
    "description" text NOT NULL,
    "notes" text NOT NULL,
    "created_at" timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    "due_date" date,
    "updated_at" timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    "last_active" timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    "version" int NOT NULL DEFAULT 1,
    "status" statuses NOT NULL DEFAULT 'queued'
);

