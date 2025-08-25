-- Partitioned parent
CREATE TABLE "acls" (
    "user_uuid" uuid NOT NULL REFERENCES users(uuid) ON DELETE CASCADE,
    "resource_type" resource_types NOT NULL,
    "resource_uuid" uuid NOT NULL,
    "role_code" text NOT NULL REFERENCES roles(code),

    PRIMARY KEY ("user_uuid", "resource_type", "resource_uuid")
) PARTITION BY LIST ("resource_type");

-- Partition for targets
CREATE TABLE "acls_targets" PARTITION OF "acls"
    FOR VALUES IN ('target');

ALTER TABLE "acls_targets"
    ADD CONSTRAINT "acls_targets_uuid_fk" 
    FOREIGN KEY ("resource_uuid") REFERENCES targets("uuid") ON DELETE CASCADE;

CREATE INDEX "acls_targets_user_uuid_idx" 
    ON "acls_targets" ("user_uuid");

CREATE INDEX "acls_targets_resource_uuid_idx" 
    ON "acls_targets" ("resource_uuid");

-- Partition for activities
CREATE TABLE "acls_activities" PARTITION OF "acls"
    FOR VALUES IN ('activity');

ALTER TABLE "acls_activities"
    ADD CONSTRAINT "acls_activities_uuid_fk" 
    FOREIGN KEY ("resource_uuid") REFERENCES activities("uuid") ON DELETE CASCADE;

CREATE INDEX "acls_activities_user_uuid_idx"
    ON "acls_activities" ("user_uuid");

CREATE INDEX "acls_activities_resource_uuid_idx"
    ON "acls_activities" ("resource_uuid");

-- Partition for activities
CREATE TABLE "acls_sessions" PARTITION OF "acls"
    FOR VALUES IN ('session');

ALTER TABLE "acls_sessions"
    ADD CONSTRAINT "acls_sessions_uuid_fk" 
    FOREIGN KEY ("resource_uuid") REFERENCES sessions("uuid") ON DELETE CASCADE;

CREATE INDEX "acls_sessions_user_uuid_idx"
    ON "acls_sessions" ("user_uuid");

CREATE INDEX "acls_sessions_resource_uuid_idx"
    ON "acls_sessions" ("resource_uuid");
