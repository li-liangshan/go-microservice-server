# migrations

SQL files are versioned in numeric order (`0001_*.sql`, `0002_*.sql`, …).
Each file should be idempotent so that it can be re-applied safely.

The database name is **not** part of a migration. The caller selects it —
`scripts/migrate.sh` passes `--database="$DATABASE_NAME"` and the Docker
entrypoint passes `--database="$MYSQL_DATABASE"` — so a deployment can point
`DATABASE_NAME` anywhere without touching the schema.

## Apply with the script (recommended)

```bash
# Uses DATABASE_HOST / DATABASE_PORT / DATABASE_USER / DATABASE_PASSWORD /
# DATABASE_NAME. Creates the database when it is missing.
DATABASE_NAME=microservice make migrate
```

The script records every applied file in the `schema_migrations` table, skips
files that were applied before, and reports the version the database is at.

## Apply manually

```bash
mysql -h127.0.0.1 -P3306 -uroot -padmin --database=microservice \
  < migrations/0001_init.sql
```

## Catalog

| File | Description |
|---|---|
| `0001_init.sql` | Creates the `users` table. |

## Moving to a real migration tool

For anything beyond a one-table template, switch to a versioned migration
runner. Drop-in options that work with this layout:

- [golang-migrate](https://github.com/golang-migrate/migrate) — pure Go, CLI + library
- [goose](https://github.com/pressly/goose) — Go + SQL, embeds into the binary
- [atlas](https://atlasgo.io) — declarative schema diffing

Both expect files numbered exactly like this (`NNN_*.sql` with `.up.sql` /
`.down.sql` pairs). Renaming `0001_init.sql` → `0001_init.up.sql` and adding a
matching `.down.sql` is all that's needed to adopt them.
