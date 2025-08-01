# Sessions of Life

## APIs

### Application
- Pattern: `GET /v1/healthcheck` -> Handler: `healthcheckHandler` -> Action: `Show application information`

### Targets
- Pattern: `GET /v1/targets` -> Handler: `...` -> Action: `List targets`
- Pattern: `POST /v1/targets` -> Handler: `...` -> Action: `Create new target`
- Pattern: `GET /v1/targets/{UUIDv1}` -> Handler: `...` -> Action: `Get detail of specific target`
- Pattern: `PUT /v1/targets/{UUIDv1}` -> Handler: `...` -> Action: `Update detail of specific target`
- Pattern: `DELETE /v1/targets/{UUIDv1}` -> Handler: `...` -> Action: `Delete a specific target`

## DB
- Using PostgreSQL as the database.
    - DSN format: `postgres://username:password@host:port/database?sslmode=disable` 

### Migrations
Using [`migrate` tool](https://github.com/golang-migrate/migrate) for database migrations.

#### Show current migration version
```bash
migrate -path=./migrations -database=<DSN> version
```

#### Generate migration files
```bash
migrate create -seq -ext=.sql -dir=./migrations <name>
```

#### Run migrations
```bash
# Executes all migrations
migrate -path=./migrations -database=<DSN> up
# Migrate to specific version
migrate -path=./migrations -database=<DSN> goto [version]
# Roll back by a specific number of migrations
migrate -path=./migrations -database=<DSN> down [number]
# Rolling back all migrations
migrate -path=./migrations -database=<DSN> down
```

### APIs
#### Create new target
Body
```json
{
  "due_at": "03 Aug 2025 15:30 +0800",
  "title": "Example title",
  "description": "Example description",
  "notes": "Example notes",
  "status": "in progress"
}
```
Request
```bash
BODY='<json>'
curl -i -X POST -d "$BODY" http://localhost:8080/v1/targets
```
