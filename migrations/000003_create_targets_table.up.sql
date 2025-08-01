CREATE TABLE IF NOT EXISTS "targets" (
    "uuid" uuid PRIMARY KEY DEFAULT uuid_generate_v1 (),
    "serial_id" bigserial NOT NULL UNIQUE,
    "title" text NOT NULL,
    "description" text NOT NULL,
    "notes" text NOT NULL,
    "created_at" timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    "due_at" timestamp(0) with time zone,
    "updated_at" timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    "version" int NOT NULL DEFAULT 1,
    "status" statuses NOT NULL DEFAULT 'queued'
);

