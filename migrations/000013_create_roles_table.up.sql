CREATE TABLE IF NOT EXISTS "roles" (
    "code" text PRIMARY KEY,
    "rank" smallint NOT NULL 
);

INSERT INTO "roles" ("code", "rank") VALUES
    ('owner', 0),
    ('editor', 100),
    ('viewer', 200);
