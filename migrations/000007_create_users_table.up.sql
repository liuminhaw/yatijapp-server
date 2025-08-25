CREATE TABLE IF NOT EXISTS users (
    "uuid" uuid PRIMARY KEY DEFAULT uuid_generate_v1 (),
    "created_at" timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    "updated_at" timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    "name" text NOT NULL,
    "email" citext UNIQUE NOT NULL,
    "password_hash" bytea NOT NULL,
    "activated" bool NOT NULL,
    "version" int NOT NULL DEFAULT 1
);
