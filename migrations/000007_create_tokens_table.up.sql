CREATE TABLE IF NOT EXISTS tokens (
    hash bytea PRIMARY KEY,
    user_uuid uuid NOT NULL REFERENCES users(uuid) ON DELETE CASCADE,
    session_uuid uuid NOT NULL,
    expiry timestamp(0) with time zone NOT NULL,
    scope text NOT NULL
);
